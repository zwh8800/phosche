// Package embedder 提供文本向量化功能，支持 Ollama 和 OpenAI 两种 embedding 后端。
package embedder

import (
	"context"
	"fmt"
)

// EmbeddingClient 定义文本向量化客户端的统一接口。
type EmbeddingClient interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// EmbeddingClientConfig 聚合 embedding 提供商选择及其配置参数。
type EmbeddingClientConfig struct {
	Provider   string
	Ollama     OllamaEmbeddingConfig
	OpenAI     OpenAIEmbeddingConfig
	Dimensions int
}

// OllamaEmbeddingConfig 包含 Ollama embedding 配置。
type OllamaEmbeddingConfig struct {
	BaseURL string
	Model   string
}

// OpenAIEmbeddingConfig 包含 OpenAI embedding 配置。
type OpenAIEmbeddingConfig struct {
	APIKey  string
	BaseURL string
	Model   string
}

// NewEmbeddingClient 是 embedding 客户端的工厂方法。
func NewEmbeddingClient(cfg EmbeddingClientConfig) (EmbeddingClient, error) {
	switch cfg.Provider {
	case "ollama":
		return NewOllamaEmbeddingClient(cfg.Ollama.BaseURL, cfg.Ollama.Model, cfg.Dimensions), nil
	case "openai":
		return NewOpenAIEmbeddingClient(cfg.OpenAI.APIKey, cfg.OpenAI.BaseURL, cfg.OpenAI.Model, cfg.Dimensions), nil
	default:
		return nil, fmt.Errorf("unsupported embedding provider: %s", cfg.Provider)
	}
}
