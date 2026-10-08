package local

import (
	"bytes"
	"container/list"
	"sync"
)

type cacheEntry struct {
	key  string
	data []byte
	size int64
}

type MemoryCache struct {
	mu       sync.RWMutex
	capacity int64 // in bytes
	used     int64
	items    map[string]*list.Element
	evict    *list.List
}

func NewMemoryCache(maxMB int64) *MemoryCache {
	return &MemoryCache{
		capacity: maxMB * 1024 * 1024,
		items:    make(map[string]*list.Element),
		evict:    list.New(),
	}
}

func (c *MemoryCache) Get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		c.evict.MoveToFront(elem)
		entry := elem.Value.(*cacheEntry)
		return entry.data, true
	}
	return nil, false
}

func (c *MemoryCache) Put(key string, data []byte) {
	dataSize := int64(len(data))
	// Do not cache files larger than 2MB individually
	if dataSize > 2*1024*1024 || dataSize > c.capacity {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// If already exists, update
	if elem, ok := c.items[key]; ok {
		c.evict.MoveToFront(elem)
		entry := elem.Value.(*cacheEntry)
		c.used += dataSize - entry.size
		entry.data = data
		entry.size = dataSize
		return
	}

	// Evict oldest until space is available
	for c.used+dataSize > c.capacity && c.evict.Len() > 0 {
		oldest := c.evict.Back()
		if oldest != nil {
			c.evict.Remove(oldest)
			entry := oldest.Value.(*cacheEntry)
			delete(c.items, entry.key)
			c.used -= entry.size
		}
	}

	entry := &cacheEntry{key: key, data: data, size: dataSize}
	elem := c.evict.PushFront(entry)
	c.items[key] = elem
	c.used += dataSize
}

func (c *MemoryCache) Invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		c.evict.Remove(elem)
		entry := elem.Value.(*cacheEntry)
		delete(c.items, key)
		c.used -= entry.size
	}
}

// MemoryReadSeekCloser adapts in-memory byte slices to ReadSeekCloser
type MemoryReadSeekCloser struct {
	*bytes.Reader
}

func NewMemoryReadSeekCloser(data []byte) *MemoryReadSeekCloser {
	return &MemoryReadSeekCloser{
		Reader: bytes.NewReader(data),
	}
}

func (m *MemoryReadSeekCloser) Close() error {
	return nil
}
