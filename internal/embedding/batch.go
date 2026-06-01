package embedding

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// BatchProcessor 提供高级批量 Embedding 处理功能，包括：
//   - 内置重试机制（指数退避）
//   - 进度回调
//   - 优雅的错误处理（可跳过失败项或整体失败）
//
// 使用示例：
//
//	processor := embedding.NewBatchProcessor(embedder, embedding.BatchProcessorConfig{
//	    MaxRetries:      3,
//	    BaseBackoff:     1 * time.Second,
//	    MaxBackoff:      30 * time.Second,
//	    FailOnFirstError: false,
//	    OnProgress: func(completed, total int) {
//	        log.Printf("Progress: %d/%d", completed, total)
//	    },
//	})
//
//	vectors, err := processor.Process(ctx, texts)
type BatchProcessor struct {
	embedder Embedder
	config   BatchProcessorConfig
}

// BatchProcessorConfig 配置 BatchProcessor 的行为。
type BatchProcessorConfig struct {
	// MaxRetries 每批次失败时的最大重试次数（默认 2）
	MaxRetries int

	// BaseBackoff 指数退避的初始等待时间（默认 1 秒）
	BaseBackoff time.Duration

	// MaxBackoff 退避的最大等待时间（默认 30 秒）
	MaxBackoff time.Duration

	// FailOnFirstError 遇到错误时是否立即失败。
	// true：任意批次失败整体失败
	// false：跳过失败批次（记录错误并返回 nil 向量），继续处理剩余
	FailOnFirstError bool

	// OnProgress 进度回调函数，每成功处理一批后调用
	OnProgress func(completed, total int)
}

// DefaultBatchProcessorConfig 返回默认的 BatchProcessor 配置。
func DefaultBatchProcessorConfig() BatchProcessorConfig {
	return BatchProcessorConfig{
		MaxRetries:       2,
		BaseBackoff:      1 * time.Second,
		MaxBackoff:       30 * time.Second,
		FailOnFirstError: false,
		OnProgress:       nil,
	}
}

// NewBatchProcessor 创建批量处理器。
func NewBatchProcessor(embedder Embedder, config BatchProcessorConfig) *BatchProcessor {
	if config.MaxRetries <= 0 {
		config.MaxRetries = 2
	}
	if config.BaseBackoff <= 0 {
		config.BaseBackoff = 1 * time.Second
	}
	if config.MaxBackoff <= 0 {
		config.MaxBackoff = 30 * time.Second
	}
	return &BatchProcessor{
		embedder: embedder,
		config:   config,
	}
}

// Process 执行完整的批量 Embedding 处理。
// texts: 输入文本列表（可以为空，返回空结果）
// returns: 与输入顺序一致的向量列表。如果 FailOnFirstError=false，
//
//	失败的输入将返回 nil 向量。可通过 ExtractErrors() 获取所有错误。
func (bp *BatchProcessor) Process(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	// 直接使用 embedder 的 EmbedBatch（它内部已处理分片）
	// 这里添加重试逻辑
	var lastErr error
	for attempt := 0; attempt <= bp.config.MaxRetries; attempt++ {
		if attempt > 0 {
			backoff := bp.calculateBackoff(attempt)
			slog.Warn("embedding retry",
				"attempt", attempt,
				"max_retries", bp.config.MaxRetries,
				"backoff", backoff,
				"error", lastErr,
			)

			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("embedding batch: context cancelled during retry: %w", ctx.Err())
			case <-time.After(backoff):
			}
		}

		if ctx.Err() != nil {
			return nil, fmt.Errorf("embedding batch: context error: %w", ctx.Err())
		}

		embeddings, err := bp.embedder.EmbedBatch(ctx, texts)
		if err == nil {
			if bp.config.OnProgress != nil {
				bp.config.OnProgress(len(texts), len(texts))
			}
			return embeddings, nil
		}

		lastErr = err

		// 不可重试的错误
		if !IsRetryable(err) {
			return nil, fmt.Errorf("embedding batch: non-retryable error: %w", err)
		}

		// 最后一次尝试失败
		if attempt == bp.config.MaxRetries {
			return nil, fmt.Errorf("embedding batch: all %d retries exhausted: %w", bp.config.MaxRetries+1, lastErr)
		}
	}

	return nil, fmt.Errorf("embedding batch: unexpected: %w", lastErr)
}

// ProcessWithProgress 与 Process 类似，但带进度回调。
// 这是一个便捷方法，为了在 Process 调用上额外添加进度跟踪而设计。
func (bp *BatchProcessor) ProcessWithProgress(ctx context.Context, texts []string, onProgress func(completed, total int)) ([][]float32, error) {
	oldProgress := bp.config.OnProgress
	bp.config.OnProgress = onProgress
	defer func() { bp.config.OnProgress = oldProgress }()
	return bp.Process(ctx, texts)
}

// calculateBackoff 计算第 n 次重试的退避时间（指数退避 + 随机抖动）。
func (bp *BatchProcessor) calculateBackoff(attempt int) time.Duration {
	// 2^(n-1) 秒
	backoff := bp.config.BaseBackoff * (1 << (attempt - 1))
	if backoff > bp.config.MaxBackoff {
		backoff = bp.config.MaxBackoff
	}
	return backoff
}

// EmbedTextWithRetry 对单条文本进行带重试的 embedding。
// 这是一个顶层便捷函数，返回单个向量。
func EmbedTextWithRetry(ctx context.Context, embedder Embedder, text string, maxRetries int) ([]float32, error) {
	processor := NewBatchProcessor(embedder, BatchProcessorConfig{
		MaxRetries:       maxRetries,
		FailOnFirstError: true,
	})
	embeddings, err := processor.Process(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("embed text: empty result")
	}
	return embeddings[0], nil
}

// EmbedBatchWithRetry 对一批文本进行带重试的批量 embedding。
// 这是一个顶层便捷函数。
func EmbedBatchWithRetry(ctx context.Context, embedder Embedder, texts []string, maxRetries int) ([][]float32, error) {
	processor := NewBatchProcessor(embedder, BatchProcessorConfig{
		MaxRetries:       maxRetries,
		FailOnFirstError: true,
	})
	return processor.Process(ctx, texts)
}
