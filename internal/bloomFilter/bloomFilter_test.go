package bloomfilter

import (
	"testing"
)

func TestBloomFilter_AddAndContains(t *testing.T) {
	bf := NewBloomFilter(1024, 4)

	key := []byte("user:123")

	bf.Add(key)

	if !bf.Contains(key) {
		t.Fatalf("expected bloom filter to contain key")
	}
}

func TestBloomFilter_Contains_MissingKey(t *testing.T) {
	bf := NewBloomFilter(1024, 4)

	bf.Add([]byte("user:123"))

	if bf.Contains([]byte("user:999")) {
		t.Fatalf("unexpected false positive for small test set")
	}
}

func TestBloomFilter_MultipleKeys(t *testing.T) {
	bf := NewBloomFilter(4096, 6)

	keys := [][]byte{
		[]byte("apple"),
		[]byte("banana"),
		[]byte("orange"),
		[]byte("grape"),
		[]byte("mango"),
	}

	for _, key := range keys {
		bf.Add(key)
	}

	for _, key := range keys {
		if !bf.Contains(key) {
			t.Fatalf("expected bloom filter to contain key: %s", key)
		}
	}
}

func TestBloomFilter_EmptyFilter(t *testing.T) {
	bf := NewBloomFilter(0, 4)

	if bf.Contains([]byte("anything")) {
		t.Fatalf("empty bloom filter should never contain keys")
	}
}

func TestBloomFilter_NoFalseNegatives(t *testing.T) {
	bf := NewBloomFilter(1<<20, 7)

	for i := 0; i < 10000; i++ {
		key := []byte(string(rune(i)))
		bf.Add(key)
	}

	for i := 0; i < 10000; i++ {
		key := []byte(string(rune(i)))

		if !bf.Contains(key) {
			t.Fatalf("false negative detected for key: %v", key)
		}
	}
}

func BenchmarkBloomFilter_Add(b *testing.B) {
	bf := NewBloomFilter(1<<20, 7)

	key := []byte("benchmark-key")

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		bf.Add(key)
	}
}

func BenchmarkBloomFilter_Contains(b *testing.B) {
	bf := NewBloomFilter(1<<20, 7)

	key := []byte("benchmark-key")

	bf.Add(key)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		bf.Contains(key)
	}
}
