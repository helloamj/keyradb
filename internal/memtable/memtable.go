package memtable

import (
	"bytes"
	"math/rand"
	"sync"
)

const (
	maxLevel    = 12
	probability = 0.25
)

type Record struct {
	Key     []byte
	Value   []byte
	Deleted bool
}

type skipListNode struct {
	record  Record
	forward []*skipListNode
}

func newSkipListNode(level int, key, value []byte, deleted bool) *skipListNode {
	return &skipListNode{
		record: Record{
			Key:     key,
			Value:   value,
			Deleted: deleted,
		},
		forward: make([]*skipListNode, level),
	}
}

type skipList struct {
	head  *skipListNode
	level int
	size  int64
}

func newSkipList() *skipList {
	return &skipList{
		head:  newSkipListNode(maxLevel, nil, nil, false),
		level: 1,
		size:  0,
	}
}

func (sl *skipList) randomLevel() int {
	lvl := 1
	for rand.Float32() < probability && lvl < maxLevel {
		lvl++
	}
	return lvl
}

func (sl *skipList) put(key, value []byte, deleted bool) {
	update := make([]*skipListNode, maxLevel)
	current := sl.head

	for i := sl.level - 1; i >= 0; i-- {
		for current.forward[i] != nil && bytes.Compare(current.forward[i].record.Key, key) < 0 {
			current = current.forward[i]
		}
		update[i] = current
	}

	current = current.forward[0]

	if current != nil && bytes.Equal(current.record.Key, key) {
		sl.size -= int64(len(current.record.Value))
		sl.size += int64(len(value))

		current.record.Value = value
		current.record.Deleted = deleted
		return
	}

	lvl := sl.randomLevel()
	if lvl > sl.level {
		for i := sl.level; i < lvl; i++ {
			update[i] = sl.head
		}
		sl.level = lvl
	}

	newNode := newSkipListNode(lvl, key, value, deleted)
	for i := 0; i < lvl; i++ {
		newNode.forward[i] = update[i].forward[i]
		update[i].forward[i] = newNode
	}

	sl.size += int64(len(key) + len(value))
}

func (sl *skipList) get(key []byte) (Record, bool) {
	current := sl.head

	for i := sl.level - 1; i >= 0; i-- {
		for current.forward[i] != nil && bytes.Compare(current.forward[i].record.Key, key) < 0 {
			current = current.forward[i]
		}
	}

	current = current.forward[0]

	if current != nil && bytes.Equal(current.record.Key, key) {
		return current.record, true
	}

	return Record{}, false
}

func (sl *skipList) iterator() []Record {
	var records []Record
	current := sl.head.forward[0]
	for current != nil {
		records = append(records, current.record)
		current = current.forward[0]
	}
	return records
}

type Memtable struct {
	mu sync.RWMutex
	sl *skipList
}

func New() *Memtable {
	return &Memtable{
		sl: newSkipList(),
	}
}

func (m *Memtable) Put(key, value []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()

	k := make([]byte, len(key))
	copy(k, key)
	v := make([]byte, len(value))
	copy(v, value)
	m.sl.put(k, v, false)
}

func (m *Memtable) Get(key []byte) ([]byte, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	record, found := m.sl.get(key)
	if !found || record.Deleted {
		return nil, false
	}
	v := make([]byte, len(record.Value))
	copy(v, record.Value)
	return v, true
}

func (m *Memtable) Delete(key []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()

	k := make([]byte, len(key))
	copy(k, key)
	m.sl.put(k, nil, true)
}

func (m *Memtable) Size() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.sl.size
}

func (m *Memtable) Iterator() []Record {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.sl.iterator()
}
