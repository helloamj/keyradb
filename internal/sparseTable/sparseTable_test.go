package sparsetable

import (
	"testing"
)

func TestSparseTable_Basic(t *testing.T) {
	st := NewSparseTable()
	if st == nil {
		t.Fatal("Expected NewSparseTable to return a non-nil pointer")
	}

	st.Add([]byte("apple"), 10)
	st.Add([]byte("banana"), 20)
	st.Add([]byte("cherry"), 30)

	tests := []struct {
		key      []byte
		expected uint64
		found    bool
	}{
		{[]byte("apple"), 10, true},
		{[]byte("banana"), 20, true},
		{[]byte("cherry"), 30, true},
		{[]byte("date"), 30, true},
		{[]byte("aardvark"), 0, false},
		{[]byte("blueberry"), 20, true},
	}

	for _, tt := range tests {
		offset, found := st.Get(tt.key)
		if found != tt.found {
			t.Errorf("Get(%q): expected found=%v, got found=%v", tt.key, tt.found, found)
		}
		if found && offset != tt.expected {
			t.Errorf("Get(%q): expected offset=%d, got offset=%d", tt.key, tt.expected, offset)
		}
	}
}

func TestSparseTable_Empty(t *testing.T) {
	st := NewSparseTable()

	offset, found := st.Get([]byte("missing"))
	if found {
		t.Errorf("Expected not found on empty table, got offset %d", offset)
	}
}

func TestSparseTable_DataCopy(t *testing.T) {
	st := NewSparseTable()

	key := []byte("hello")
	st.Add(key, 100)
	key[0] = 'j'
	offset, found := st.Get([]byte("hello"))
	if !found || offset != 100 {
		t.Errorf("Expected to find original key 'hello', Add might not have copied the key properly")
	}
}
