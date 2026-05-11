package sparsetable

import "testing"

func TestSparseTable_Basic(t *testing.T) {
	st := NewSparseTable()

	if st == nil {
		t.Fatal("expected non-nil SparseTable")
	}

	st.Add([]byte("apple"), []byte("banana"), 10)
	st.Add([]byte("carrot"), []byte("grape"), 20)
	st.Add([]byte("kiwi"), []byte("orange"), 30)

	tests := []struct {
		key      []byte
		expected uint64
		found    bool
	}{
		{[]byte("apple"), 10, true},
		{[]byte("banana"), 10, true},

		{[]byte("carrot"), 20, true},
		{[]byte("grape"), 20, true},
		{[]byte("date"), 20, true},

		{[]byte("kiwi"), 30, true},
		{[]byte("orange"), 30, true},

		{[]byte("aardvark"), 0, false},
		{[]byte("zzz"), 0, false},
		{[]byte("horse"), 0, false},
	}

	for _, tt := range tests {
		offset, found := st.Get(tt.key)

		if found != tt.found {
			t.Errorf(
				"Get(%q): expected found=%v got=%v",
				tt.key,
				tt.found,
				found,
			)
		}

		if found && offset != tt.expected {
			t.Errorf(
				"Get(%q): expected offset=%d got=%d",
				tt.key,
				tt.expected,
				offset,
			)
		}
	}
}

func TestSparseTable_Empty(t *testing.T) {
	st := NewSparseTable()

	offset, found := st.Get([]byte("missing"))

	if found {
		t.Errorf(
			"expected not found in empty table, got offset=%d",
			offset,
		)
	}
}

func TestSparseTable_KeyCopy(t *testing.T) {
	st := NewSparseTable()

	minKey := []byte("apple")
	maxKey := []byte("banana")

	st.Add(minKey, maxKey, 100)

	minKey[0] = 'z'
	maxKey[0] = 'z'

	offset, found := st.Get([]byte("apple"))

	if !found || offset != 100 {
		t.Fatalf("expected copied keys to remain intact")
	}
}

func TestSparseTable_SingleEntry(t *testing.T) {
	st := NewSparseTable()

	st.Add([]byte("a"), []byte("z"), 999)

	offset, found := st.Get([]byte("m"))

	if !found {
		t.Fatal("expected key to be found")
	}

	if offset != 999 {
		t.Fatalf("expected offset=999 got=%d", offset)
	}
}
