package analyzer

import (
	"context"
	"fmt"

	"github.com/zwh8800/phosche/internal/types"
)

type LLMClient interface {
	AnalyzeImage(ctx context.Context, imageData []byte, prompt string) (*types.AnalysisResult, error)
}

type LLMClientConfig struct {
	Provider string
	Ollama   OllamaClientConfig
	OpenAI   OpenAIClientConfig
}

type OllamaClientConfig struct {
	BaseURL string
	Model   string
}

type OpenAIClientConfig struct {
	APIKey  string
	BaseURL string
	Model   string
}

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
