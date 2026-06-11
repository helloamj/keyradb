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
	"time"

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

type immutableMemtable struct {
	mem     *memtable.Memtable
	walPath string
}

type DB struct {
	dir  string
	opts Options

	mu           sync.RWMutex
	active       *memtable.Memtable
	immutables   []*immutableMemtable
	sstableFiles []string
	readers      []*sstable.TableReader
	wal          *wal.WAL
	nextSST      int
	closed       bool

	flushChan chan *immutableMemtable
	flushDone chan struct{}
}

func Open(dir string, opts Options) (*DB, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("db: create dir: %w", err)
	}

	db := &DB{
		dir:       dir,
		opts:      opts,
		active:    memtable.New(),
		flushChan: make(chan *immutableMemtable, 10),
		flushDone: make(chan struct{}),
	}

	if err := db.loadSSTables(); err != nil {
		return nil, fmt.Errorf("db: load sstables: %w", err)
	}

	if err := db.recoverWALs(); err != nil {
		return nil, fmt.Errorf("db: wal recovery: %w", err)
	}

	go db.flushLoop()
	go db.compactionLoop()

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
		if err := db.rollActiveMemtable(); err != nil {
			return fmt.Errorf("db: roll memtable: %w", err)
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
		if err := db.rollActiveMemtable(); err != nil {
			return fmt.Errorf("db: roll memtable: %w", err)
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
		if v, ok := db.immutables[i].mem.Get(key); ok {
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
	if db.closed {
		db.mu.Unlock()
		return ErrClosed
	}
	if db.active.Size() > 0 {
		if err := db.rollActiveMemtable(); err != nil {
			db.mu.Unlock()
			return err
		}
	}
	db.mu.Unlock()
	return nil
}

func (db *DB) Close() error {
	db.mu.Lock()
	if db.closed {
		db.mu.Unlock()
		return nil
	}
	db.closed = true

	if db.active.Size() > 0 {
		_ = db.rollActiveMemtable()
	}
	db.mu.Unlock()

	close(db.flushChan)
	<-db.flushDone

	db.mu.Lock()
	defer db.mu.Unlock()

	for _, r := range db.readers {
		r.Close()
	}

	if db.wal != nil {
		return db.wal.Close()
	}
	return nil
}

func (db *DB) rollActiveMemtable() error {
	if db.wal != nil {
		if err := db.wal.Close(); err != nil {
			return err
		}
	}

	ts := time.Now().UnixNano()
	oldWalPath := filepath.Join(db.dir, fmt.Sprintf("wal-%d.log", ts))

	activeWalPath := filepath.Join(db.dir, walFileName)
	if _, err := os.Stat(activeWalPath); err == nil {
		if err := os.Rename(activeWalPath, oldWalPath); err != nil {
			return err
		}
	} else {
		oldWalPath = filepath.Join(db.dir, fmt.Sprintf("wal-%d.log", ts))
	}

	w, err := wal.NewWAL(activeWalPath)
	if err != nil {
		return err
	}
	db.wal = w

	imm := &immutableMemtable{
		mem:     db.active,
		walPath: oldWalPath,
	}

	db.immutables = append([]*immutableMemtable{imm}, db.immutables...)
	db.active = memtable.New()

	db.flushChan <- imm
	return nil
}

func (db *DB) flushLoop() {
	defer close(db.flushDone)
	for imm := range db.flushChan {
		if err := db.flushImmutable(imm); err != nil {
			fmt.Printf("db: flush failed: %v\n", err)
		}
	}
}

func (db *DB) flushImmutable(imm *immutableMemtable) error {
	db.mu.Lock()
	db.nextSST++
	path := filepath.Join(db.dir, fmt.Sprintf("%08d%s", db.nextSST, sstableExt))
	db.mu.Unlock()

	records := imm.mem.Iterator()
	builder, err := sstable.NewTableBuilder(path, uint64(len(records)))
	if err != nil {
		return err
	}

	for _, rec := range records {
		if err := builder.Add(rec.Key, rec.Value, rec.Deleted); err != nil {
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

	db.mu.Lock()
	db.readers = append([]*sstable.TableReader{reader}, db.readers...)
	db.sstableFiles = append([]string{path}, db.sstableFiles...)

	for i, im := range db.immutables {
		if im == imm {
			db.immutables = append(db.immutables[:i], db.immutables[i+1:]...)
			break
		}
	}
	db.mu.Unlock()

	os.Remove(imm.walPath)
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

func (db *DB) recoverWALs() error {
	entries, err := os.ReadDir(db.dir)
	if err != nil {
		return err
	}

	var walFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "wal") && strings.HasSuffix(e.Name(), ".log") {
			walFiles = append(walFiles, e.Name())
		}
	}

	sort.Strings(walFiles)

	for _, name := range walFiles {
		path := filepath.Join(db.dir, name)
		w, err := wal.NewWAL(path)
		if err != nil {
			return err
		}
		records, err := w.Recover()
		w.Close()

		if err != nil {
			return err
		}

		if name == walFileName {
			db.wal, _ = wal.NewWAL(path)
			for _, rec := range records {
				if rec.Deleted {
					db.active.Delete(rec.Key)
				} else {
					db.active.Put(rec.Key, rec.Value)
				}
			}
		} else {
			immMem := memtable.New()
			for _, rec := range records {
				if rec.Deleted {
					immMem.Delete(rec.Key)
				} else {
					immMem.Put(rec.Key, rec.Value)
				}
			}
			imm := &immutableMemtable{
				mem:     immMem,
				walPath: path,
			}
			db.immutables = append([]*immutableMemtable{imm}, db.immutables...)
			db.flushChan <- imm
		}
	}

	if db.wal == nil {
		activeWalPath := filepath.Join(db.dir, walFileName)
		db.wal, err = wal.NewWAL(activeWalPath)
		if err != nil {
			return err
		}
	}

	return nil
}
