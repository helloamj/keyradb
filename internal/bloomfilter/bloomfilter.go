package bloomfilter

import (
	"encoding/binary"
	"errors"

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

func (bf *BloomFilter) Serialize() []byte {
	// Format:
	// numBits (8 bytes)
	// numHashes (4 bytes)
	// numWords (4 bytes) - len(bf.bits)
	// bits (numWords * 8 bytes)

	numWords := len(bf.bits)
	buf := make([]byte, 8+4+4+(numWords*8))

	binary.LittleEndian.PutUint64(buf[0:8], bf.numBits)
	binary.LittleEndian.PutUint32(buf[8:12], bf.numHashes)
	binary.LittleEndian.PutUint32(buf[12:16], uint32(numWords))

	offset := 16
	for _, word := range bf.bits {
		binary.LittleEndian.PutUint64(buf[offset:offset+8], word)
		offset += 8
	}

	return buf
}

func Deserialize(data []byte) (*BloomFilter, error) {
	if len(data) < 16 {
		return nil, errors.New("invalid bloom filter data")
	}

	numBits := binary.LittleEndian.Uint64(data[0:8])
	numHashes := binary.LittleEndian.Uint32(data[8:12])
	numWords := binary.LittleEndian.Uint32(data[12:16])

	if len(data) < 16+int(numWords)*8 {
		return nil, errors.New("corrupted bloom filter bits")
	}

	bits := make([]uint64, numWords)
	offset := 16
	for i := uint32(0); i < numWords; i++ {
		bits[i] = binary.LittleEndian.Uint64(data[offset : offset+8])
		offset += 8
	}

	return &BloomFilter{
		bits:      bits,
		numBits:   numBits,
		numHashes: numHashes,
	}, nil
}
