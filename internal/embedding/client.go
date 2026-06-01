// Package embedding 提供统一的文本向量化（Embedding）客户端，支持 Ollama 和 OpenAI 两种后端。
// 功能特性：
//   - 统一的 Embedder 接口，屏蔽后端差异
//   - 工厂方法 NewEmbedder() 根据配置自动选择实现
//   - 支持单文本和批量文本嵌入
//   - 支持 Matryoshka 维度截断（dimensions 参数）
//   - 生产级别的错误处理：超时控制、上下文取消、错误分类（可重试/不可重试）
package embedding

import (
	"context"
	"fmt"
)

// Embedder 定义了统一的 Embedding 客户端接口。
// EmbedText 对单条文本生成向量，EmbedBatch 对文本数组批量生成。
// 两者都接收 context 用于超时/取消控制。
type Embedder interface {
	// EmbedText 对单条文本生成向量嵌入。
	// text: 待编码的文本
	// returns: 归一化后的浮点向量，以及可能的错误
	EmbedText(ctx context.Context, text string) ([]float32, error)

	// EmbedBatch 对一批文本批量生成向量嵌入。
	// texts: 待编码的文本数组
	// returns: 与输入顺序一致的向量数组，以及可能的错误。
	// 实现应正确处理后端限制（Ollama 无限制，OpenAI 单请求上限 2048 条/30 万 token）
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
}

// Provider 表示 Embedding 服务提供商枚举。
type Provider string

const (
	ProviderOllama Provider = "ollama"
	ProviderOpenAI Provider = "openai"
)

// Config 聚合了 Embedding 客户端的全部配置。
type Config struct {
	// Provider 选择 Embedding 提供商："ollama" 或 "openai"
	Provider Provider `yaml:"provider"`

	// Ollama 配置，当 Provider 为 "ollama" 时使用
	Ollama OllamaConfig `yaml:"ollama"`

	// OpenAI 配置，当 Provider 为 "openai" 时使用
	OpenAI OpenAIConfig `yaml:"openai"`

	// Model 使用的模型名称。
	// Ollama 可选：mxbai-embed-large, nomic-embed-text, all-minilm, bge-m3 等
	// OpenAI 可选：text-embedding-3-small, text-embedding-3-large, text-embedding-ada-002
	Model string `yaml:"model"`

	// Dimensions 输出向量的维度数。
	// Ollama: 仅部分模型支持（如 bge-m3），为空时使用模型默认维度
	// OpenAI: text-embedding-3 系列支持 Matryoshka 截断，为空时使用模型默认维度
	//   text-embedding-3-small 默认 1536，text-embedding-3-large 默认 3072
	Dimensions int `yaml:"dimensions"`

	// BatchSize 批量嵌入时每批次的最大文本数。
	// Ollama: 无上限
	// OpenAI: 最大 2048 条，且单请求总 token 不超过 30 万
	BatchSize int `yaml:"batch_size"`

	// TimeoutSeconds 单次请求的超时时间（秒）。
	// 批量请求会使用此超时 × 批次数作为总超时
	TimeoutSeconds int `yaml:"timeout_seconds"`
}

// OllamaConfig 是 Ollama Embedding 连接配置。
type OllamaConfig struct {
	BaseURL string `yaml:"base_url"` // Ollama 服务地址，如 http://localhost:11434
}

// OpenAIConfig 是 OpenAI Embedding API 连接配置。
type OpenAIConfig struct {
	APIKey  string `yaml:"api_key"`  // OpenAI API 密钥
	BaseURL string `yaml:"base_url"` // API 基础地址，如 https://api.openai.com/v1
}

// DefaultConfig 返回推荐的默认配置。
func DefaultConfig() Config {
	return Config{
		Provider:       ProviderOllama,
		Model:          "mxbai-embed-large",
		Dimensions:     0, // 使用模型默认维度
		BatchSize:      32,
		TimeoutSeconds: 30,
		Ollama: OllamaConfig{
			BaseURL: "http://localhost:11434",
		},
		OpenAI: OpenAIConfig{
			BaseURL: "https://api.openai.com/v1",
		},
	}
}

// NewEmbedder 是 Embedder 的工厂方法，根据 cfg.Provider 创建对应的实现。
func NewEmbedder(cfg Config) (Embedder, error) {
	switch cfg.Provider {
	case ProviderOllama:
		return NewOllamaEmbedder(cfg), nil
	case ProviderOpenAI:
		return NewOpenAIEmbedder(cfg), nil
	default:
		return nil, fmt.Errorf("unsupported embedding provider: %q (supported: ollama, openai)", cfg.Provider)
	}
}
