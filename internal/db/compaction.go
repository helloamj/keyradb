package db

import (
	"bytes"
	"container/heap"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/helloamj/keyradb/internal/sstable"
)

type compactionItem struct {
	rec      sstable.IteratorRecord
	iterIdx  int
	iterator *sstable.TableIterator
}

type compactionHeap []compactionItem

func (h compactionHeap) Len() int { return len(h) }
func (h compactionHeap) Less(i, j int) bool {
	cmp := bytes.Compare(h[i].rec.Key, h[j].rec.Key)
	if cmp == 0 {
		return h[i].iterIdx < h[j].iterIdx
	}
	return cmp < 0
}
func (h compactionHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *compactionHeap) Push(x interface{}) {
	*h = append(*h, x.(compactionItem))
}
func (h *compactionHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[0 : n-1]
	return item
}

func (db *DB) compactionLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-db.flushDone:
			return
		case <-ticker.C:
			db.maybeCompact()
		}
	}
}

func (db *DB) maybeCompact() {
	db.mu.RLock()
	readerCount := len(db.readers)
	db.mu.RUnlock()

	if readerCount < 4 {
		return
	}

	db.mu.Lock()
	readersToCompact := make([]*sstable.TableReader, len(db.readers))
	copy(readersToCompact, db.readers)

	pathsToRemove := make([]string, len(db.sstableFiles))
	copy(pathsToRemove, db.sstableFiles)

	db.nextSST++
	newSSTPath := filepath.Join(db.dir, fmt.Sprintf("%08d%s", db.nextSST, sstableExt))
	db.mu.Unlock()

	h := &compactionHeap{}
	heap.Init(h)

	var totalRecords uint64
	for i, r := range readersToCompact {
		st := r.SparseTable()
		if st != nil {
			totalRecords += uint64(len(st.Entries())) * 100
		}
		it := r.Iterator()
		rec, ok, err := it.Next()
		if err != nil {
			fmt.Printf("compaction error reading sstable: %v\n", err)
			return
		}
		if ok {
			heap.Push(h, compactionItem{
				rec:      rec,
				iterIdx:  i,
				iterator: it,
			})
		}
	}

	builder, err := sstable.NewTableBuilder(newSSTPath, totalRecords+100)
	if err != nil {
		fmt.Printf("compaction error creating builder: %v\n", err)
		return
	}

	var lastKey []byte

	for h.Len() > 0 {
		item := heap.Pop(h).(compactionItem)

		if lastKey == nil || !bytes.Equal(item.rec.Key, lastKey) {
			// Drop tombstones because we are merging all levels to bottom level
			if !item.rec.Deleted {
				if err := builder.Add(item.rec.Key, item.rec.Value, false); err != nil {
					fmt.Printf("compaction error adding to builder: %v\n", err)
					return
				}
			}
			lastKey = append([]byte(nil), item.rec.Key...)
		}

		rec, ok, err := item.iterator.Next()
		if err != nil {
			fmt.Printf("compaction error reading next: %v\n", err)
			return
		}
		if ok {
			heap.Push(h, compactionItem{
				rec:      rec,
				iterIdx:  item.iterIdx,
				iterator: item.iterator,
			})
		}
	}

	if err := builder.Finish(); err != nil {
		fmt.Printf("compaction error finishing builder: %v\n", err)
		return
	}

	newReader, err := sstable.NewTableReader(newSSTPath)
	if err != nil {
		fmt.Printf("compaction error opening new reader: %v\n", err)
		return
	}

	db.mu.Lock()
	var newReaders []*sstable.TableReader
	var newFiles []string

	for i := 0; i < len(db.readers); i++ {
		keep := true
		for _, rc := range readersToCompact {
			if db.readers[i] == rc {
				keep = false
				break
			}
		}
		if keep {
			newReaders = append(newReaders, db.readers[i])
			newFiles = append(newFiles, db.sstableFiles[i])
		}
	}

	newReaders = append(newReaders, newReader)
	newFiles = append(newFiles, newSSTPath)

	db.readers = newReaders
	db.sstableFiles = newFiles
	db.mu.Unlock()

	for _, rc := range readersToCompact {
		rc.Close()
	}
	for _, p := range pathsToRemove {
		os.Remove(p)
	}
}
