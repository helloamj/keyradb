package memtable

import (
	"bytes"
	"testing"
)

func TestMemtable_PutGet(t *testing.T) {
	m := New()

	key := []byte("hello")
	value := []byte("world")

	m.Put(key, value)

	got, found := m.Get(key)
	if !found {
		t.Fatalf("expected key %q to be found", key)
	}
	if !bytes.Equal(got, value) {
		t.Fatalf("expected value %q, got %q", value, got)
	}

	newValue := []byte("updated")
	m.Put(key, newValue)

	got, found = m.Get(key)
	if !found {
		t.Fatalf("expected key %q to be found", key)
	}
	if !bytes.Equal(got, newValue) {
		t.Fatalf("expected value %q, got %q", newValue, got)
	}
}

func TestMemtable_Delete(t *testing.T) {
	m := New()

	key := []byte("hello")
	value := []byte("world")

	m.Put(key, value)
	m.Delete(key)

	_, found := m.Get(key)
	if found {
		t.Fatalf("expected key %q to be deleted", key)
	}

	records := m.Iterator()
	if len(records) != 1 {
		t.Fatalf("expected 1 record in iterator, got %d", len(records))
	}
	if !records[0].Deleted {
		t.Fatalf("expected record to be marked as deleted")
	}
}

func TestMemtable_IteratorOrder(t *testing.T) {
	m := New()

	m.Put([]byte("z"), []byte("26"))
	m.Put([]byte("a"), []byte("1"))
	m.Put([]byte("m"), []byte("13"))

	records := m.Iterator()
	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(records))
	}

	if !bytes.Equal(records[0].Key, []byte("a")) ||
		!bytes.Equal(records[1].Key, []byte("m")) ||
		!bytes.Equal(records[2].Key, []byte("z")) {
		t.Fatalf("records are not in sorted order")
	}
}

func TestMemtable_Size(t *testing.T) {
	m := New()

	if m.Size() != 0 {
		t.Fatalf("expected size 0, got %d", m.Size())
	}

	m.Put([]byte("key1"), []byte("value1"))
	if m.Size() != 10 {
		t.Fatalf("expected size 10, got %d", m.Size())
	}

	m.Put([]byte("key1"), []byte("val2"))
	if m.Size() != 8 {
		t.Fatalf("expected size 8, got %d", m.Size())
	}

	m.Delete([]byte("key1"))
	if m.Size() != 4 {
		t.Fatalf("expected size 4, got %d", m.Size())
	}
}

func TestMemtable_PutGetMissing(t *testing.T) {
	m := New()

	_, found := m.Get([]byte("missing"))
	if found {
		t.Fatalf("expected not found")
	}
}
