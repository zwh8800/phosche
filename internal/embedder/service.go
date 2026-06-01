package embedder

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// EmbeddingService 是带重试和缓存的 embedding 服务封装。
type EmbeddingService struct {
	client     EmbeddingClient
	cache      *EmbeddingCache
	maxRetries int
	timeout    time.Duration
}

// NewEmbeddingService 创建 embedding 服务。
func NewEmbeddingService(client EmbeddingClient, cache *EmbeddingCache, maxRetries int, timeout time.Duration) *EmbeddingService {
	return &EmbeddingService{
		client:     client,
		cache:      cache,
		maxRetries: maxRetries,
		timeout:    timeout,
	}
}

// Embed 执行文本向量化，支持缓存和重试。
func (s *EmbeddingService) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if s.cache != nil && len(texts) == 1 {
		if cached, ok := s.cache.Get(texts[0]); ok {
			slog.Debug("embedding cache hit", "text_len", len(texts[0]))
			return [][]float32{cached}, nil
		}
	}

	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	// When embedding a single text, log its content for debugging
	var textsSummary string
	if len(texts) == 1 {
		textsSummary = texts[0]
	} else {
		textsSummary = fmt.Sprintf("[%d texts]", len(texts))
	}
	slog.Info("starting embedding", "text_count", len(texts), "timeout", s.timeout, "texts", textsSummary)
	startTime := time.Now()

	var lastErr error
	for attempt := 0; attempt <= s.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
			slog.Warn("retrying embedding request", "attempt", attempt, "error", lastErr)
		}

		result, err := s.client.Embed(ctx, texts)
		if err != nil {
			lastErr = err
			if !isRetryable(err) {
				slog.Error("embedding failed (non-retryable)", "error", err, "duration", time.Since(startTime).Round(time.Millisecond))
				return nil, err
			}
			continue
		}

		if s.cache != nil && len(texts) == 1 && len(result) > 0 {
			s.cache.Set(texts[0], result[0])
		}

		slog.Info("embedding completed",
			"duration", time.Since(startTime).Round(time.Millisecond),
			"attempts", attempt+1,
			"text_count", len(texts),
			"vector_dims", len(result[0]),
		)
		return result, nil
	}

	slog.Error("embedding failed after retries", "attempts", s.maxRetries+1, "error", lastErr, "duration", time.Since(startTime).Round(time.Millisecond))
	return nil, lastErr
}

func isRetryable(err error) bool {
	errStr := err.Error()
	if len(errStr) > 0 {
		for _, code := range []string{"status 5", "status 429", "timeout", "connection"} {
			if strings.Contains(errStr, code) {
				return true
			}
		}
		for _, code := range []string{"status 40", "status 41"} {
			if strings.Contains(errStr, code) {
				return false
			}
		}
	}
	return true
}
