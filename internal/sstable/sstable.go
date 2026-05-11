package sstable

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"sync"

	bloomfilter "github.com/helloamj/keyradb/internal/bloomfilter"
	sparsetable "github.com/helloamj/keyradb/internal/sparseTable"
)

var (
	ErrKeyNotFound   = errors.New("key not found in sstable")
	ErrClosed        = errors.New("sstable is closed")
	ErrKeysNotSorted = errors.New("keys must be added in strictly increasing order")
)

const (
	BlockSize   = 4096
	MagicNumber = 0x4B45595241444231
	FooterSize  = 40
)

type TableReader struct {
	file        *os.File
	size        int64
	sparseTable *sparsetable.SparseTable
	bloomFilter *bloomfilter.BloomFilter

	mu     sync.RWMutex
	closed bool
}

func NewTableReader(path string) (*TableReader, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	stat, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}

	if stat.Size() < int64(FooterSize) {
		file.Close()
		return nil, errors.New("file too small to be a valid sstable")
	}

	footerData := make([]byte, FooterSize)
	if _, err := file.ReadAt(footerData, stat.Size()-int64(FooterSize)); err != nil {
		file.Close()
		return nil, err
	}

	magic := binary.LittleEndian.Uint64(footerData[32:40])
	if magic != MagicNumber {
		file.Close()
		return nil, errors.New("invalid sstable magic number")
	}

	bfOffset := binary.LittleEndian.Uint64(footerData[0:8])
	bfSize := binary.LittleEndian.Uint64(footerData[8:16])
	stOffset := binary.LittleEndian.Uint64(footerData[16:24])
	stSize := binary.LittleEndian.Uint64(footerData[24:32])

	bfData := make([]byte, bfSize)
	if _, err := file.ReadAt(bfData, int64(bfOffset)); err != nil {
		file.Close()
		return nil, err
	}
	filter, err := bloomfilter.Deserialize(bfData)
	if err != nil {
		file.Close()
		return nil, err
	}

	stData := make([]byte, stSize)
	if _, err := file.ReadAt(stData, int64(stOffset)); err != nil {
		file.Close()
		return nil, err
	}
	index, err := sparsetable.Deserialize(stData)
	if err != nil {
		file.Close()
		return nil, err
	}

	return &TableReader{
		file:        file,
		size:        stat.Size(),
		sparseTable: index,
		bloomFilter: filter,
	}, nil
}

func (tr *TableReader) Get(key []byte) ([]byte, error) {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	if tr.closed {
		return nil, ErrClosed
	}

	if !tr.bloomFilter.Contains(key) {
		return nil, ErrKeyNotFound
	}

	offset, ok := tr.sparseTable.Get(key)
	if !ok {
		return nil, ErrKeyNotFound
	}

	var lenBuf [4]byte
	if _, err := tr.file.ReadAt(lenBuf[:], int64(offset)); err != nil {
		return nil, err
	}
	blockLen := binary.LittleEndian.Uint32(lenBuf[:])

	blockData := make([]byte, blockLen)
	if _, err := tr.file.ReadAt(blockData, int64(offset)+4); err != nil {
		return nil, err
	}

	idx := 0
	for idx < int(blockLen) {
		if idx+6 > int(blockLen) {
			break
		}
		kLen := int(binary.LittleEndian.Uint16(blockData[idx : idx+2]))
		vLen := int(binary.LittleEndian.Uint32(blockData[idx+2 : idx+6]))
		idx += 6

		if idx+kLen+vLen > int(blockLen) {
			break
		}

		currKey := blockData[idx : idx+kLen]
		currVal := blockData[idx+kLen : idx+kLen+vLen]
		idx += kLen + vLen

		cmp := bytes.Compare(currKey, key)
		if cmp == 0 {
			ret := make([]byte, len(currVal))
			copy(ret, currVal)
			return ret, nil
		} else if cmp > 0 {
			break
		}
	}

	return nil, ErrKeyNotFound
}

func (tr *TableReader) Close() error {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	if tr.closed {
		return nil
	}
	tr.closed = true
	return tr.file.Close()
}

type TableBuilder struct {
	file        *os.File
	sparseTable *sparsetable.SparseTable
	bloomFilter *bloomfilter.BloomFilter

	currentBlock bytes.Buffer
	blockOffset  uint64
	minKey       []byte
	maxKey       []byte
	lastKey      []byte

	closed bool
}

func NewTableBuilder(path string, expectedKeyCount uint64) (*TableBuilder, error) {
	file, err := os.Create(path)
	if err != nil {
		return nil, err
	}

	return &TableBuilder{
		file:        file,
		sparseTable: sparsetable.NewSparseTable(),
		bloomFilter: bloomfilter.NewBloomFilter(expectedKeyCount*10, 4),
	}, nil
}

func (tb *TableBuilder) flushBlock() error {
	if tb.currentBlock.Len() == 0 {
		return nil
	}

	var lenBuf [4]byte
	binary.LittleEndian.PutUint32(lenBuf[:], uint32(tb.currentBlock.Len()))
	if _, err := tb.file.Write(lenBuf[:]); err != nil {
		return err
	}

	if _, err := tb.file.Write(tb.currentBlock.Bytes()); err != nil {
		return err
	}

	tb.sparseTable.Add(tb.minKey, tb.maxKey, tb.blockOffset)

	tb.blockOffset += uint64(4 + tb.currentBlock.Len())
	tb.currentBlock.Reset()
	tb.minKey = nil
	tb.maxKey = nil

	return nil
}

func (tb *TableBuilder) Add(key []byte, value []byte) error {
	if tb.closed {
		return ErrClosed
	}

	if tb.lastKey != nil && bytes.Compare(key, tb.lastKey) <= 0 {
		return ErrKeysNotSorted
	}
	tb.lastKey = append([]byte(nil), key...)

	if tb.currentBlock.Len() == 0 {
		tb.minKey = append([]byte(nil), key...)
	}
	tb.maxKey = append([]byte(nil), key...)

	var hdr [6]byte
	binary.LittleEndian.PutUint16(hdr[0:2], uint16(len(key)))
	binary.LittleEndian.PutUint32(hdr[2:6], uint32(len(value)))

	tb.currentBlock.Write(hdr[:])
	tb.currentBlock.Write(key)
	tb.currentBlock.Write(value)

	tb.bloomFilter.Add(key)

	if tb.currentBlock.Len() >= BlockSize {
		return tb.flushBlock()
	}

	return nil
}

func (tb *TableBuilder) Finish() error {
	if tb.closed {
		return ErrClosed
	}
	tb.closed = true

	if err := tb.flushBlock(); err != nil {
		tb.file.Close()
		return err
	}

	bfData := tb.bloomFilter.Serialize()
	bfOffset := tb.blockOffset
	bfSize := uint64(len(bfData))
	if _, err := tb.file.Write(bfData); err != nil {
		tb.file.Close()
		return err
	}

	stData := tb.sparseTable.Serialize()
	stOffset := bfOffset + bfSize
	stSize := uint64(len(stData))
	if _, err := tb.file.Write(stData); err != nil {
		tb.file.Close()
		return err
	}

	var footer [FooterSize]byte
	binary.LittleEndian.PutUint64(footer[0:8], bfOffset)
	binary.LittleEndian.PutUint64(footer[8:16], bfSize)
	binary.LittleEndian.PutUint64(footer[16:24], stOffset)
	binary.LittleEndian.PutUint64(footer[24:32], stSize)
	binary.LittleEndian.PutUint64(footer[32:40], MagicNumber)

	if _, err := tb.file.Write(footer[:]); err != nil {
		tb.file.Close()
		return err
	}

	if err := tb.file.Sync(); err != nil {
		tb.file.Close()
		return err
	}

	return tb.file.Close()
}
