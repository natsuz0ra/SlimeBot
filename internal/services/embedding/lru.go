package embedding

import "github.com/hashicorp/golang-lru/v2"

type embeddingLRU struct {
	cache *lru.Cache[string, []float32]
}

func newEmbeddingLRU(cap int) *embeddingLRU {
	if cap < 1 {
		cap = 512
	}
	cache, err := lru.New[string, []float32](cap)
	if err != nil {
		cache, _ = lru.New[string, []float32](512)
	}
	return &embeddingLRU{cache: cache}
}

func (c *embeddingLRU) get(key string) ([]float32, bool) {
	if c == nil || c.cache == nil {
		return nil, false
	}
	v, ok := c.cache.Get(key)
	if !ok {
		return nil, false
	}
	out := make([]float32, len(v))
	copy(out, v)
	return out, true
}

func (c *embeddingLRU) put(key string, val []float32) {
	if c == nil || c.cache == nil {
		return
	}
	copyVal := make([]float32, len(val))
	copy(copyVal, val)
	c.cache.Add(key, copyVal)
}
