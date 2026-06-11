package wal

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
	"log"
	"os"
	"sync"
)

const (
	opPut    byte = 0
	opDelete byte = 1
)

var ErrChecksumMismatch = errors.New("wal record checksum mismatch - data corruption detected")

type Record struct {
	Key     []byte
	Value   []byte
	Deleted bool
}

type WAL struct {
	file *os.File
	mu   sync.Mutex
}

func NewWAL(path string) (*WAL, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	log.Print("NewWAL: created file: ", path)
	if err != nil {
		return nil, err
	}
	return &WAL{file: file}, nil
}

func (w *WAL) Append(key []byte, value []byte, deleted bool) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	var opType byte = opPut
	if deleted {
		opType = opDelete
	}

	keyLen := uint32(len(key))
	valLen := uint32(len(value))

	// Layout: opType(1) | keyLen(4) | key | valLen(4) | value | checksum(4)
	size := 1 + 4 + keyLen + 4 + valLen + 4
	buf := make([]byte, size)

	buf[0] = opType
	binary.LittleEndian.PutUint32(buf[1:5], keyLen)
	copy(buf[5:5+keyLen], key)
	binary.LittleEndian.PutUint32(buf[5+keyLen:9+keyLen], valLen)
	copy(buf[9+keyLen:size-4], value)

	checksum := crc32.ChecksumIEEE(buf[:size-4])
	binary.LittleEndian.PutUint32(buf[size-4:], checksum)

	_, err := w.file.Write(buf)
	if err != nil {
		return err
	}

	return w.file.Sync()
}

func (w *WAL) Recover() ([]Record, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	_, err := w.file.Seek(0, io.SeekStart)
	if err != nil {
		return nil, err
	}
	defer func() {
		_, _ = w.file.Seek(0, io.SeekEnd)
	}()

	var records []Record

	for {
		var opTypeBuf [1]byte
		_, err := io.ReadFull(w.file, opTypeBuf[:])
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		opType := opTypeBuf[0]
		deleted := (opType == opDelete)

		var keyLenBuf [4]byte
		_, err = io.ReadFull(w.file, keyLenBuf[:])
		if err != nil {
			return nil, err
		}
		keyLen := binary.LittleEndian.Uint32(keyLenBuf[:])

		key := make([]byte, keyLen)
		_, err = io.ReadFull(w.file, key)
		if err != nil {
			return nil, err
		}

		var valLenBuf [4]byte
		_, err = io.ReadFull(w.file, valLenBuf[:])
		if err != nil {
			return nil, err
		}
		valLen := binary.LittleEndian.Uint32(valLenBuf[:])

		value := make([]byte, valLen)
		if valLen > 0 {
			_, err = io.ReadFull(w.file, value)
			if err != nil {
				return nil, err
			}
		}

		var checksumBuf [4]byte
		_, err = io.ReadFull(w.file, checksumBuf[:])
		if err != nil {
			return nil, err
		}
		storedChecksum := binary.LittleEndian.Uint32(checksumBuf[:])

		dataSize := 1 + 4 + keyLen + 4 + valLen
		dataBuf := make([]byte, dataSize)
		dataBuf[0] = opType
		copy(dataBuf[1:5], keyLenBuf[:])
		copy(dataBuf[5:5+keyLen], key)
		copy(dataBuf[5+keyLen:9+keyLen], valLenBuf[:])
		copy(dataBuf[9+keyLen:], value)

		calculatedChecksum := crc32.ChecksumIEEE(dataBuf)
		if calculatedChecksum != storedChecksum {
			return nil, ErrChecksumMismatch
		}

		records = append(records, Record{
			Key:     key,
			Value:   value,
			Deleted: deleted,
		})
	}

	return records, nil
}

func (w *WAL) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.file.Close()
}

func (w *WAL) Clear() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	path := w.file.Name()
	if err := w.file.Close(); err != nil {
		return err
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_TRUNC|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	w.file = file
	return nil
}

func (w *WAL) Size() int64 {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file == nil {
		return 0
	}
	stat, err := w.file.Stat()
	if err != nil {
		return 0
	}
	return stat.Size()
}
