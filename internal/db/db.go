package db

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/helloamj/keyradb/internal/memtable"
	"github.com/helloamj/keyradb/internal/sstable"
	"github.com/helloamj/keyradb/internal/wal"
)

const (
	defaultMemtableMaxBytes int64 = 4 * 1024 * 1024
	walFileName                   = "wal.log"
	sstableExt                    = ".sst"
)

var (
	ErrKeyNotFound = errors.New("key not found")
	ErrClosed      = errors.New("db is closed")
)

type Options struct {
	MemtableMaxBytes int64
}

func DefaultOptions() Options {
	return Options{
		MemtableMaxBytes: defaultMemtableMaxBytes,
	}
}

type DB struct {
	dir  string
	opts Options

	mu           sync.RWMutex
	active       *memtable.Memtable
	immutables   []*memtable.Memtable
	sstableFiles []string
	readers      []*sstable.TableReader
	wal          *wal.WAL
	nextSST      int
	closed       bool
}

func Open(dir string, opts Options) (*DB, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("db: create dir: %w", err)
	}

	walPath := filepath.Join(dir, walFileName)
	w, err := wal.NewWAL(walPath)
	if err != nil {
		return nil, fmt.Errorf("db: open wal: %w", err)
	}

	db := &DB{
		dir:    dir,
		opts:   opts,
		active: memtable.New(),
		wal:    w,
	}

	if err := db.loadSSTables(); err != nil {
		w.Close()
		return nil, fmt.Errorf("db: load sstables: %w", err)
	}

	if err := db.recover(); err != nil {
		w.Close()
		return nil, fmt.Errorf("db: wal recovery: %w", err)
	}

	return db, nil
}

func (db *DB) Put(key, value []byte) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if db.closed {
		return ErrClosed
	}

	if err := db.wal.Append(key, value, false); err != nil {
		return fmt.Errorf("db: wal append: %w", err)
	}

	db.active.Put(key, value)

	if db.active.Size() >= db.opts.MemtableMaxBytes {
		if err := db.flushLocked(); err != nil {
			return fmt.Errorf("db: flush: %w", err)
		}
	}

	return nil
}

func (db *DB) Delete(key []byte) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if db.closed {
		return ErrClosed
	}

	if err := db.wal.Append(key, nil, true); err != nil {
		return fmt.Errorf("db: wal append: %w", err)
	}

	db.active.Delete(key)

	if db.active.Size() >= db.opts.MemtableMaxBytes {
		if err := db.flushLocked(); err != nil {
			return fmt.Errorf("db: flush: %w", err)
		}
	}

	return nil
}

func (db *DB) Get(key []byte) ([]byte, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	if db.closed {
		return nil, ErrClosed
	}

	if v, ok := db.active.Get(key); ok {
		return v, nil
	}

	for i := len(db.immutables) - 1; i >= 0; i-- {
		if v, ok := db.immutables[i].Get(key); ok {
			return v, nil
		}
	}

	for _, r := range db.readers {
		v, err := r.Get(key)
		if err == nil {
			return v, nil
		}
		if !errors.Is(err, sstable.ErrKeyNotFound) {
			return nil, err
		}
	}

	return nil, ErrKeyNotFound
}

func (db *DB) Flush() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if db.closed {
		return ErrClosed
	}

	return db.flushLocked()
}

func (db *DB) Close() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if db.closed {
		return nil
	}
	db.closed = true

	if err := db.flushLocked(); err != nil {
		return err
	}

	for _, r := range db.readers {
		r.Close()
	}

	return db.wal.Close()
}

func (db *DB) flushLocked() error {
	if db.active.Size() == 0 {
		return nil
	}

	records := db.active.Iterator()

	db.nextSST++
	path := filepath.Join(db.dir, fmt.Sprintf("%08d%s", db.nextSST, sstableExt))

	builder, err := sstable.NewTableBuilder(path, uint64(len(records)))
	if err != nil {
		return err
	}

	for _, rec := range records {
		if rec.Deleted {
			continue
		}
		if err := builder.Add(rec.Key, rec.Value); err != nil {
			return err
		}
	}

	if err := builder.Finish(); err != nil {
		return err
	}

	reader, err := sstable.NewTableReader(path)
	if err != nil {
		return err
	}

	db.readers = append([]*sstable.TableReader{reader}, db.readers...)
	db.sstableFiles = append([]string{path}, db.sstableFiles...)

	if err := db.wal.Clear(); err != nil {
		return err
	}

	db.active = memtable.New()

	return nil
}

func (db *DB) loadSSTables() error {
	entries, err := os.ReadDir(db.dir)
	if err != nil {
		return err
	}

	type sstEntry struct {
		seq  int
		path string
	}

	var found []sstEntry

	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, sstableExt) {
			continue
		}

		seqStr := strings.TrimSuffix(name, sstableExt)
		seq, err := strconv.Atoi(seqStr)
		if err != nil {
			continue
		}

		found = append(found, sstEntry{seq: seq, path: filepath.Join(db.dir, name)})
		if seq > db.nextSST {
			db.nextSST = seq
		}
	}

	sort.Slice(found, func(i, j int) bool { return found[i].seq < found[j].seq })

	for i := len(found) - 1; i >= 0; i-- {
		r, err := sstable.NewTableReader(found[i].path)
		if err != nil {
			return fmt.Errorf("open sstable %s: %w", found[i].path, err)
		}
		db.readers = append(db.readers, r)
		db.sstableFiles = append(db.sstableFiles, found[i].path)
	}

	return nil
}

func (db *DB) recover() error {
	records, err := db.wal.Recover()
	if err != nil {
		return err
	}

	for _, rec := range records {
		if rec.Deleted {
			db.active.Delete(rec.Key)
		} else {
			db.active.Put(rec.Key, rec.Value)
		}
	}

	return nil
}
