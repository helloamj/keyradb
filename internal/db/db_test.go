package db

import (
	"errors"
	"fmt"
	"os"
	"testing"
)

func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "keyradb-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func TestPutAndGet(t *testing.T) {
	db, err := Open(tempDir(t), DefaultOptions())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	if err := db.Put([]byte("hello"), []byte("world")); err != nil {
		t.Fatalf("Put: %v", err)
	}

	val, err := db.Get([]byte("hello"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(val) != "world" {
		t.Fatalf("expected 'world', got %q", val)
	}
}

func TestGetMissing(t *testing.T) {
	db, err := Open(tempDir(t), DefaultOptions())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	_, err = db.Get([]byte("missing"))
	if !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("expected ErrKeyNotFound, got %v", err)
	}
}

func TestDelete(t *testing.T) {
	db, err := Open(tempDir(t), DefaultOptions())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	if err := db.Put([]byte("k"), []byte("v")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := db.Delete([]byte("k")); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = db.Get([]byte("k"))
	if !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("expected ErrKeyNotFound after delete, got %v", err)
	}
}

func TestOverwrite(t *testing.T) {
	db, err := Open(tempDir(t), DefaultOptions())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	if err := db.Put([]byte("k"), []byte("v1")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := db.Put([]byte("k"), []byte("v2")); err != nil {
		t.Fatalf("Put: %v", err)
	}

	val, err := db.Get([]byte("k"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(val) != "v2" {
		t.Fatalf("expected 'v2', got %q", val)
	}
}

func TestFlushAndReadFromSSTable(t *testing.T) {
	dir := tempDir(t)
	db, err := Open(dir, DefaultOptions())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	const n = 100
	for i := 0; i < n; i++ {
		key := []byte(fmt.Sprintf("key-%04d", i))
		val := []byte(fmt.Sprintf("val-%04d", i))
		if err := db.Put(key, val); err != nil {
			t.Fatalf("Put %d: %v", i, err)
		}
	}

	if err := db.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	for i := 0; i < n; i++ {
		key := []byte(fmt.Sprintf("key-%04d", i))
		expected := fmt.Sprintf("val-%04d", i)
		val, err := db.Get(key)
		if err != nil {
			t.Fatalf("Get key-%04d: %v", i, err)
		}
		if string(val) != expected {
			t.Fatalf("key-%04d: expected %q, got %q", i, expected, val)
		}
	}

	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestWALRecovery(t *testing.T) {
	dir := tempDir(t)

	db, err := Open(dir, DefaultOptions())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if err := db.Put([]byte("persistent"), []byte("value")); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Close without flushing — WAL holds the data.
	if err := db.wal.Close(); err != nil {
		t.Fatalf("wal Close: %v", err)
	}
	db.closed = true

	// Re-open; recovery should replay the WAL.
	db2, err := Open(dir, DefaultOptions())
	if err != nil {
		t.Fatalf("Re-open: %v", err)
	}
	defer db2.Close()

	val, err := db2.Get([]byte("persistent"))
	if err != nil {
		t.Fatalf("Get after recovery: %v", err)
	}
	if string(val) != "value" {
		t.Fatalf("expected 'value', got %q", val)
	}
}

func TestReopenPersistence(t *testing.T) {
	dir := tempDir(t)

	db, err := Open(dir, DefaultOptions())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := db.Put([]byte("a"), []byte("1")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	db2, err := Open(dir, DefaultOptions())
	if err != nil {
		t.Fatalf("Re-open: %v", err)
	}
	defer db2.Close()

	val, err := db2.Get([]byte("a"))
	if err != nil {
		t.Fatalf("Get after reopen: %v", err)
	}
	if string(val) != "1" {
		t.Fatalf("expected '1', got %q", val)
	}
}

func TestClosedDBReturnsError(t *testing.T) {
	db, err := Open(tempDir(t), DefaultOptions())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	db.Close()

	if err := db.Put([]byte("k"), []byte("v")); !errors.Is(err, ErrClosed) {
		t.Fatalf("expected ErrClosed on Put, got %v", err)
	}
	if _, err := db.Get([]byte("k")); !errors.Is(err, ErrClosed) {
		t.Fatalf("expected ErrClosed on Get, got %v", err)
	}
}
