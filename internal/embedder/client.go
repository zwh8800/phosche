// Package embedder 提供文本向量化功能，通过 OpenAI 兼容 API 实现。
package embedder

import (
	"context"
	"fmt"
)

// EmbeddingClient 定义文本向量化客户端的统一接口。
type EmbeddingClient interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// EmbeddingClientConfig 聚合 embedding 配置参数。
type EmbeddingClientConfig struct {
	Provider   string
	OpenAI     OpenAIEmbeddingConfig
	Dimensions int
}

// OpenAIEmbeddingConfig 包含 OpenAI embedding 配置。
type OpenAIEmbeddingConfig struct {
	APIKey  string
	BaseURL string
	Model   string
}

// NewEmbeddingClient 是 embedding 客户端的工厂方法，创建 OpenAI 兼容的 embedding 客户端。
func NewEmbeddingClient(cfg EmbeddingClientConfig) (EmbeddingClient, error) {
	switch cfg.Provider {
	case "openai":
		return NewOpenAIEmbeddingClient(cfg.OpenAI.APIKey, cfg.OpenAI.BaseURL, cfg.OpenAI.Model, cfg.Dimensions), nil
	default:
		return nil, fmt.Errorf("unsupported embedding provider: %q", cfg.Provider)
	}
}
