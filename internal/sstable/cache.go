package sstable

import "sync"

type cacheNode struct {
	offset uint64
	data   []byte
	prev   *cacheNode
	next   *cacheNode
}

type blockCache struct {
	mu       sync.Mutex
	capacity int
	items    map[uint64]*cacheNode
	head     *cacheNode
	tail     *cacheNode
}

func newBlockCache(capacity int) *blockCache {
	return &blockCache{
		capacity: capacity,
		items:    make(map[uint64]*cacheNode),
	}
}

func (c *blockCache) get(offset uint64) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if node, ok := c.items[offset]; ok {
		c.moveToFront(node)
		return node.data, true
	}
	return nil, false
}

func (c *blockCache) put(offset uint64, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if node, ok := c.items[offset]; ok {
		node.data = data
		c.moveToFront(node)
		return
	}

	node := &cacheNode{
		offset: offset,
		data:   data,
	}
	c.items[offset] = node
	c.addToFront(node)

	if len(c.items) > c.capacity {
		c.removeOldest()
	}
}

func (c *blockCache) moveToFront(node *cacheNode) {
	if c.head == node {
		return
	}
	c.removeNode(node)
	c.addToFront(node)
}

func (c *blockCache) addToFront(node *cacheNode) {
	node.prev = nil
	node.next = c.head
	if c.head != nil {
		c.head.prev = node
	}
	c.head = node
	if c.tail == nil {
		c.tail = node
	}
}

func (c *blockCache) removeNode(node *cacheNode) {
	if node.prev != nil {
		node.prev.next = node.next
	} else {
		c.head = node.next
	}
	if node.next != nil {
		node.next.prev = node.prev
	} else {
		c.tail = node.prev
	}
}

func (c *blockCache) removeOldest() {
	if c.tail == nil {
		return
	}
	delete(c.items, c.tail.offset)
	c.removeNode(c.tail)
}
