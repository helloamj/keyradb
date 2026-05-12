package wal

import (
	"bytes"
	"os"
	"testing"
)

func TestWAL(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "wal_test")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	w, err := NewWAL(tmpPath)
	if err != nil {
		t.Fatalf("NewWAL failed: %v", err)
	}
	defer w.Close()

	err = w.Append([]byte("key1"), []byte("val1"), false)
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	err = w.Append([]byte("key2"), nil, true)
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	records, err := w.Recover()
	if err != nil {
		t.Fatalf("Recover failed: %v", err)
	}

	if len(records) != 2 {
		t.Fatalf("Expected 2 records, got %d", len(records))
	}

	if !bytes.Equal(records[0].Key, []byte("key1")) || !bytes.Equal(records[0].Value, []byte("val1")) || records[0].Deleted {
		t.Errorf("Record 0 mismatch: %+v", records[0])
	}
	if !bytes.Equal(records[1].Key, []byte("key2")) || len(records[1].Value) != 0 || !records[1].Deleted {
		t.Errorf("Record 1 mismatch: %+v", records[1])
	}

	err = w.Clear()
	if err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	records, err = w.Recover()
	if err != nil {
		t.Fatalf("Recover after Clear failed: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("Expected 0 records after Clear, got %d", len(records))
	}
}

func TestWAL_Reopen(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "wal_test_reopen")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	w, err := NewWAL(tmpPath)
	if err != nil {
		t.Fatalf("NewWAL failed: %v", err)
	}

	err = w.Append([]byte("key1"), []byte("val1"), false)
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}
	w.Close()

	w2, err := NewWAL(tmpPath)
	if err != nil {
		t.Fatalf("NewWAL reopen failed: %v", err)
	}
	defer w2.Close()

	records, err := w2.Recover()
	if err != nil {
		t.Fatalf("Recover failed: %v", err)
	}

	if len(records) != 1 {
		t.Fatalf("Expected 1 record, got %d", len(records))
	}
	if !bytes.Equal(records[0].Key, []byte("key1")) || !bytes.Equal(records[0].Value, []byte("val1")) || records[0].Deleted {
		t.Errorf("Record mismatch: %+v", records[0])
	}

	err = w2.Append([]byte("key2"), []byte("val2"), false)
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	records, err = w2.Recover()
	if err != nil {
		t.Fatalf("Recover failed: %v", err)
	}

	if len(records) != 2 {
		t.Fatalf("Expected 2 records, got %d", len(records))
	}
}
