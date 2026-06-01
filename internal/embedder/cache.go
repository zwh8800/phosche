package embedder

import (
	"time"

	lru "github.com/hashicorp/golang-lru/v2/expirable"
)

// EmbeddingCache 是查询 embedding 的 LRU + TTL 缓存。
type EmbeddingCache struct {
	cache *lru.LRU[string, []float32]
}

// NewEmbeddingCache 创建缓存实例。
func NewEmbeddingCache(size int, ttl time.Duration) *EmbeddingCache {
	return &EmbeddingCache{
		cache: lru.NewLRU[string, []float32](size, nil, ttl),
	}
}

// Get 从缓存获取 embedding。
func (c *EmbeddingCache) Get(text string) ([]float32, bool) {
	return c.cache.Get(text)
}

// Set 将 embedding 写入缓存。
func (c *EmbeddingCache) Set(text string, embedding []float32) {
	c.cache.Add(text, embedding)
}
