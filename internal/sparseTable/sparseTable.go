package sparsetable

import "bytes"

type SparseIndex struct {
	minKey []byte
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

func (st *SparseTable) Add(key []byte, offset uint64) {
	keyCopy := make([]byte, len(key))
	copy(keyCopy, key)

	st.index = append(st.index, SparseIndex{
		minKey: keyCopy,
		offset: offset,
	})
}

func (st *SparseTable) Get(key []byte) (uint64, bool) {
	if len(st.index) == 0 {
		return 0, false
	}

	low := 0
	high := len(st.index) - 1

	result := -1

	for low <= high {
		mid := low + (high-low)/2

		cmp := bytes.Compare(st.index[mid].minKey, key)

		if cmp <= 0 {
			result = mid
			low = mid + 1
		} else {
			high = mid - 1
		}
	}

	if result == -1 {
		return 0, false
	}

	return st.index[result].offset, true
}
