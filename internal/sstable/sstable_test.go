package sstable

import (
	"bytes"
	"fmt"
	"path/filepath"
	"testing"
)

func TestTableBuilderAndReader(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "test.sst")

	builder, err := NewTableBuilder(path, 1000)
	if err != nil {
		t.Fatalf("failed to create builder: %v", err)
	}

	entries := []struct {
		key []byte
		val []byte
	}{
		{[]byte("key1"), []byte("val1")},
		{[]byte("key2"), []byte("val2")},
		{[]byte("key3"), []byte("val3")},
		{[]byte("key4"), []byte("val4")},
	}

	for _, e := range entries {
		err := builder.Add(e.key, e.val)
		if err != nil {
			t.Fatalf("failed to add %s: %v", string(e.key), err)
		}
	}

	err = builder.Finish()
	if err != nil {
		t.Fatalf("failed to finish builder: %v", err)
	}

	reader, err := NewTableReader(path)
	if err != nil {
		t.Fatalf("failed to open reader: %v", err)
	}
	defer reader.Close()

	for _, e := range entries {
		val, err := reader.Get(e.key)
		if err != nil {
			t.Errorf("failed to get %s: %v", string(e.key), err)
		}
		if !bytes.Equal(val, e.val) {
			t.Errorf("expected val %s, got %s", string(e.val), string(val))
		}
	}

	_, err = reader.Get([]byte("non-existent"))
	if err != ErrKeyNotFound {
		t.Errorf("expected ErrKeyNotFound, got %v", err)
	}
}

func TestUnsortedKeys(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "test_unsorted.sst")

	builder, err := NewTableBuilder(path, 10)
	if err != nil {
		t.Fatalf("failed to create builder: %v", err)
	}
	defer func() { _ = builder.Finish() }()

	err = builder.Add([]byte("key2"), []byte("val2"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = builder.Add([]byte("key1"), []byte("val1"))
	if err != ErrKeysNotSorted {
		t.Errorf("expected ErrKeysNotSorted, got %v", err)
	}
}

func TestLargeBlocks(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "test_large.sst")

	builder, err := NewTableBuilder(path, 1000)
	if err != nil {
		t.Fatalf("failed to create builder: %v", err)
	}

	for i := 0; i < 2000; i++ {
		key := []byte(fmt.Sprintf("key%05d", i))
		val := make([]byte, 100)
		err := builder.Add(key, val)
		if err != nil {
			t.Fatalf("failed to add %s: %v", string(key), err)
		}
	}

	err = builder.Finish()
	if err != nil {
		t.Fatalf("failed to finish builder: %v", err)
	}

	reader, err := NewTableReader(path)
	if err != nil {
		t.Fatalf("failed to open reader: %v", err)
	}
	defer reader.Close()

	for i := 0; i < 2000; i++ {
		key := []byte(fmt.Sprintf("key%05d", i))
		_, err := reader.Get(key)
		if err != nil {
			t.Errorf("failed to get %s: %v", string(key), err)
		}
	}
}
