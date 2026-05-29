// Package analyzer 提供基于多模态 LLM 的图片内容分析功能。支持 Ollama（本地）和 OpenAI（云端）两种后端。
package analyzer

import (
	"context"
	"fmt"

	"github.com/zwh8800/phosche/internal/types"
)

// LLMClient 定义了大语言模型客户端的统一接口，将不同的 LLM 后端（Ollama、OpenAI）抽象为单一的 AnalyzeImage 方法。
// 调用方无需关心底层协议差异，只需传入图片数据和提示词即可获得结构化的分析结果。
type LLMClient interface {
	AnalyzeImage(ctx context.Context, imageData []byte, prompt string) (*types.AnalysisResult, error)
}

// LLMClientConfig 聚合了 LLM 提供商选择及其对应的配置参数。
// Provider 字段决定使用哪种后端（"ollama" 或 "openai"），Ollama 和 OpenAI 字段分别包含对应后端的连接和模型参数。
type LLMClientConfig struct {
	Provider string
	Ollama   OllamaClientConfig
	OpenAI   OpenAIClientConfig
}

// OllamaClientConfig 包含连接本地 Ollama 服务所需的配置参数。
type OllamaClientConfig struct {
	BaseURL string // Ollama 服务地址，如 http://localhost:11434
	Model   string // 视觉模型名称，如 llama3.2-vision
}

// OpenAIClientConfig 包含连接 OpenAI（或兼容 API）服务所需的配置参数。
type OpenAIClientConfig struct {
	APIKey  string // API 认证密钥
	BaseURL string // API 基础地址，如 https://api.openai.com/v1
	Model   string // 模型名称，如 gpt-4o
}

// NewLLMClient 是 LLM 客户端的工厂方法，根据 cfg.Provider 的值创建对应的客户端实现。
// 支持 "ollama" 和 "openai" 两种 provider，其他值返回错误。
func NewLLMClient(cfg LLMClientConfig) (LLMClient, error) {
	switch cfg.Provider {
	case "ollama":
		return NewOllamaClient(cfg.Ollama.BaseURL, cfg.Ollama.Model), nil
	case "openai":
		return NewOpenAIClient(cfg.OpenAI.APIKey, cfg.OpenAI.BaseURL, cfg.OpenAI.Model), nil
	default:
		return nil, fmt.Errorf("unsupported LLM provider: %s", cfg.Provider)
	}
}
