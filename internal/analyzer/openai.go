package analyzer

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"

	openai "github.com/sashabaranov/go-openai"

	"github.com/zwh8800/phosche/internal/types"
)

// OpenAIClient wraps the go-openai library to implement LLMClient.
type OpenAIClient struct {
	client *openai.Client
	model  string
}

// NewOpenAIClient creates an OpenAIClient. An empty baseURL uses the go-openai default.
func NewOpenAIClient(apiKey, baseURL, model string) *OpenAIClient {
	config := openai.DefaultConfig(apiKey)
	if baseURL != "" {
		config.BaseURL = baseURL
	}
	return &OpenAIClient{
		client: openai.NewClientWithConfig(config),
		model:  model,
	}
}

// AnalyzeImage sends the image and prompt to the OpenAI-compatible API and returns a structured result.
func (c *OpenAIClient) AnalyzeImage(ctx context.Context, imageData []byte, prompt string) (*types.AnalysisResult, error) {
	encodedImage := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(imageData)

	slog.Debug("openai: sending request",
		"model", c.model,
		"prompt", truncate(prompt, 200),
		"image_bytes", len(imageData),
	)

	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: c.model,
		Messages: []openai.ChatCompletionMessage{
			{
				Role: openai.ChatMessageRoleUser,
				MultiContent: []openai.ChatMessagePart{
					{
						Type: openai.ChatMessagePartTypeText,
						Text: prompt,
					},
					{
						Type: openai.ChatMessagePartTypeImageURL,
						ImageURL: &openai.ChatMessageImageURL{
							URL: encodedImage,
						},
					},
				},
			},
		},
		ResponseFormat: &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONObject,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("openai request failed: %w", err)
	}

	slog.Debug("openai: received response",
		"choices", len(resp.Choices),
		"usage_prompt_tokens", resp.Usage.PromptTokens,
		"usage_completion_tokens", resp.Usage.CompletionTokens,
	)

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("openai response has no choices")
	}

	content := resp.Choices[0].Message.Content
	slog.Debug("openai: response content", "content", truncate(content, 2000))

	var result types.AnalysisResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("unmarshal analysis result: %w", err)
	}

	return &result, nil
}
