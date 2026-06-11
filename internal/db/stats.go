package db

import (
	"path/filepath"
)

type MemtableRecord struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Deleted bool   `json:"deleted"`
}

type BlockStats struct {
	MinKey string `json:"min_key"`
	MaxKey string `json:"max_key"`
	Offset uint64 `json:"offset"`
}

type SSTableStats struct {
	Path   string       `json:"path"`
	Name   string       `json:"name"`
	Size   int64        `json:"size"`
	Blocks []BlockStats `json:"blocks"`
}

type DBStats struct {
	ActiveMemSize int64              `json:"active_mem_size"`
	ActiveMemMax  int64              `json:"active_mem_max"`
	ActiveKeys    []MemtableRecord   `json:"active_keys"`
	Immutables    [][]MemtableRecord `json:"immutables"`
	SSTables      []SSTableStats     `json:"sstables"`
	WalSize       int64              `json:"wal_size"`
}

func (db *DB) Stats() (DBStats, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	if db.closed {
		return DBStats{}, ErrClosed
	}

	// 1. Active memtable records
	activeRecords := db.active.Iterator()
	activeKeys := make([]MemtableRecord, len(activeRecords))
	for i, r := range activeRecords {
		activeKeys[i] = MemtableRecord{
			Key:     string(r.Key),
			Value:   string(r.Value),
			Deleted: r.Deleted,
		}
	}

	// 2. Immutable memtable records
	immutables := make([][]MemtableRecord, len(db.immutables))
	for i, imm := range db.immutables {
		immRecords := imm.mem.Iterator()
		immKeys := make([]MemtableRecord, len(immRecords))
		for j, r := range immRecords {
			immKeys[j] = MemtableRecord{
				Key:     string(r.Key),
				Value:   string(r.Value),
				Deleted: r.Deleted,
			}
		}
		immutables[i] = immKeys
	}

	// 3. SSTable stats
	sstStats := make([]SSTableStats, len(db.readers))
	for i, reader := range db.readers {
		path := reader.Path()
		size := reader.Size()
		st := reader.SparseTable()

		var blocks []BlockStats
		if st != nil {
			entries := st.Entries()
			blocks = make([]BlockStats, len(entries))
			for j, entry := range entries {
				blocks[j] = BlockStats{
					MinKey: string(entry.MinKey),
					MaxKey: string(entry.MaxKey),
					Offset: entry.Offset,
				}
			}
		}

		sstStats[i] = SSTableStats{
			Path:   path,
			Name:   filepath.Base(path),
			Size:   size,
			Blocks: blocks,
		}
	}

	// 4. WAL size
	walSize := int64(0)
	if db.wal != nil {
		walSize = db.wal.Size()
	}

	return DBStats{
		ActiveMemSize: db.active.Size(),
		ActiveMemMax:  db.opts.MemtableMaxBytes,
		ActiveKeys:    activeKeys,
		Immutables:    immutables,
		SSTables:      sstStats,
		WalSize:       walSize,
	}, nil
}
