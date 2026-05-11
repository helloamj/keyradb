package sparsetable

import (
	"bytes"
	"encoding/binary"
	"errors"
)

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

// Get returns the offset of the block whose key range contains the key.
//
// Example:
//
//	[a-c] -> 0
//	[d-f] -> 100
//	[g-z] -> 200
//
// Lookup:
//
//	key="e" => 100
func (st *SparseTable) Get(key []byte) (uint64, bool) {
	if len(st.index) == 0 {
		return 0, false
	}

	low := 0
	high := len(st.index) - 1

	for low <= high {
		mid := low + (high-low)/2

		entry := st.index[mid]

		// key < minKey
		if bytes.Compare(key, entry.minKey) < 0 {
			high = mid - 1
			continue
		}

		// key > maxKey
		if bytes.Compare(key, entry.maxKey) > 0 {
			low = mid + 1
			continue
		}

		// minKey <= key <= maxKey
		return entry.offset, true
	}

	return 0, false
}

// Serialize converts the sparse table into a binary format.
func (st *SparseTable) Serialize() []byte {
	var buf bytes.Buffer

	numEntries := uint32(len(st.index))
	_ = binary.Write(&buf, binary.LittleEndian, numEntries)

	for _, entry := range st.index {
		_ = binary.Write(&buf, binary.LittleEndian, uint16(len(entry.minKey)))
		buf.Write(entry.minKey)

		_ = binary.Write(&buf, binary.LittleEndian, uint16(len(entry.maxKey)))
		buf.Write(entry.maxKey)

		_ = binary.Write(&buf, binary.LittleEndian, entry.offset)
	}

	return buf.Bytes()
}

// Deserialize reads a sparse table from a binary format.
func Deserialize(data []byte) (*SparseTable, error) {
	if len(data) < 4 {
		return nil, errors.New("invalid sparse table data")
	}

	buf := bytes.NewReader(data)

	var numEntries uint32
	if err := binary.Read(buf, binary.LittleEndian, &numEntries); err != nil {
		return nil, err
	}

	index := make([]SparseIndex, numEntries)
	for i := uint32(0); i < numEntries; i++ {
		var minKeyLen uint16
		if err := binary.Read(buf, binary.LittleEndian, &minKeyLen); err != nil {
			return nil, err
		}

		minKey := make([]byte, minKeyLen)
		if _, err := buf.Read(minKey); err != nil {
			return nil, err
		}

		var maxKeyLen uint16
		if err := binary.Read(buf, binary.LittleEndian, &maxKeyLen); err != nil {
			return nil, err
		}

		maxKey := make([]byte, maxKeyLen)
		if _, err := buf.Read(maxKey); err != nil {
			return nil, err
		}

		var offset uint64
		if err := binary.Read(buf, binary.LittleEndian, &offset); err != nil {
			return nil, err
		}

		index[i] = SparseIndex{
			minKey: minKey,
			maxKey: maxKey,
			offset: offset,
		}
	}

	return &SparseTable{index: index}, nil
}
