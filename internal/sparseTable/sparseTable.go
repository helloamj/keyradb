package sparsetable

import "bytes"

type SparseIndex struct {
	minKey []byte
	maxKey []byte
	offset uint64
}

type SparseTable struct {
	index []SparseIndex
}

func NewSparseTable() *SparseTable {
	return &SparseTable{
		index: make([]SparseIndex, 0),
	}
}

func (st *SparseTable) Add(minKey, maxKey []byte, offset uint64) {
	minKeyCopy := make([]byte, len(minKey))
	copy(minKeyCopy, minKey)

	maxKeyCopy := make([]byte, len(maxKey))
	copy(maxKeyCopy, maxKey)

	st.index = append(st.index, SparseIndex{
		minKey: minKeyCopy,
		maxKey: maxKeyCopy,
		offset: offset,
	})
}

func (st *SparseTable) Get(key []byte) (uint64, bool) {
	if len(st.index) == 0 {
		return 0, false
	}

	low := 0
	high := len(st.index) - 1

	for low <= high {
		mid := low + (high-low)/2

		entry := st.index[mid]

		if bytes.Compare(key, entry.minKey) < 0 {
			high = mid - 1
			continue
		}

		if bytes.Compare(key, entry.maxKey) > 0 {
			low = mid + 1
			continue
		}

		return entry.offset, true
	}

	return 0, false
}
