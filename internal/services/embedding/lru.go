package embedding

import (
	"container/list"
	"sync"
)

type embeddingLRU struct {
	mu  sync.Mutex
	cap int
	ll  *list.List
	m   map[string]*list.Element
}

type lruEntry struct {
	key string
	val []float32
}

func newEmbeddingLRU(cap int) *embeddingLRU {
	if cap < 1 {
		cap = 512
	}
	return &embeddingLRU{
		cap: cap,
		ll:  list.New(),
		m:   make(map[string]*list.Element),
	}
}

func (c *embeddingLRU) get(key string) ([]float32, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.m[key]; ok {
		c.ll.MoveToFront(el)
		v := el.Value.(*lruEntry).val
		out := make([]float32, len(v))
		copy(out, v)
		return out, true
	}
	return nil, false
}

func (c *embeddingLRU) put(key string, val []float32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	copyVal := make([]float32, len(val))
	copy(copyVal, val)
	if el, ok := c.m[key]; ok {
		el.Value.(*lruEntry).val = copyVal
		c.ll.MoveToFront(el)
		return
	}
	ne := &lruEntry{key: key, val: copyVal}
	elem := c.ll.PushFront(ne)
	c.m[key] = elem
	for c.ll.Len() > c.cap {
		back := c.ll.Back()
		if back == nil {
			break
		}
		old := back.Value.(*lruEntry)
		delete(c.m, old.key)
		c.ll.Remove(back)
	}
}
