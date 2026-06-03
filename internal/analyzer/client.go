// Package analyzer provides multimodal LLM-based image content analysis
// using the OpenAI-compatible protocol.
package analyzer

import (
	"context"
	"strings"

	"github.com/zwh8800/phosche/internal/types"
)

// LLMClient 定义了大语言模型客户端的统一接口，基于 OpenAI 兼容协议抽象为单一的 AnalyzeImage 方法。
// 调用方无需关心底层是本地 Ollama 还是云端 OpenAI，只需传入图片数据和提示词即可获得结构化的分析结果。
type LLMClient interface {
	AnalyzeImage(ctx context.Context, imageData []byte, prompt string) (*types.AnalysisResult, error)
}

// LLMClientConfig 使用 OpenAI 兼容协议统一配置本地和云端 LLM。
type LLMClientConfig struct {
	BaseURL        string // OpenAI 兼容 API 地址（不含 /v1，工厂自动追加）
	Model          string // 模型名称，如 llama3.2-vision 或 gpt-4o
	APIKey         string // API 密钥（可选，留空时使用 "ollama" 占位符）
	ResponseFormat string // 响应格式：json_object / json_schema / text，空字符串默认 json_object
}

// NewLLMClient 基于 OpenAI 兼容协议创建 LLM 客户端。
// 如果 APIKey 为空则使用 "ollama" 占位符；如果 BaseURL 不以 /v1 结尾则自动追加。
func NewLLMClient(cfg LLMClientConfig) (LLMClient, error) {
	apiKey := cfg.APIKey
	if apiKey == "" {
		apiKey = "ollama"
	}
	baseURL := cfg.BaseURL
	if !strings.HasSuffix(baseURL, "/v1") {
		baseURL += "/v1"
	}
	return NewOpenAIClient(apiKey, baseURL, cfg.Model, cfg.ResponseFormat)
}
