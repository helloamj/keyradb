package bloomfilter

import (
	"github.com/cespare/xxhash/v2"
)

type BloomFilter struct {
	bits      []uint64
	numBits   uint64
	numHashes uint32
}

func NewBloomFilter(numBits uint64, numHashes uint32) *BloomFilter {
	return &BloomFilter{
		bits:      make([]uint64, (numBits+63)/64),
		numBits:   numBits,
		numHashes: numHashes,
	}
}

func getHashes(key []byte) (uint64, uint64) {
	sum := xxhash.Sum64(key)

	h1 := uint64(uint32(sum))
	h2 := uint64(uint32(sum >> 32))
	if h2 == 0 {
		h2 = 0x27d4eb2d
	}

	return h1, h2
}

func (bf *BloomFilter) Add(key []byte) {
	if bf.numBits == 0 {
		return
	}

	h1, h2 := getHashes(key)

	combined := h1

	for i := uint32(0); i < bf.numHashes; i++ {
		bitPos := combined % bf.numBits

		wordIdx := bitPos >> 6
		bitIdx := bitPos & 63

		bf.bits[wordIdx] |= 1 << bitIdx

		combined += h2
	}
}

func (bf *BloomFilter) Contains(key []byte) bool {
	if bf.numBits == 0 {
		return false
	}

	h1, h2 := getHashes(key)

	combined := h1

	for i := uint32(0); i < bf.numHashes; i++ {
		bitPos := combined % bf.numBits

		wordIdx := bitPos >> 6
		bitIdx := bitPos & 63

		if (bf.bits[wordIdx] & (1 << bitIdx)) == 0 {
			return false
		}

		combined += h2
	}

	return true
}
