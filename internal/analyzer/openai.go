package analyzer

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	openai "github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"

	"github.com/zwh8800/phosche/internal/types"
)

// OpenAIClient wraps the go-openai library to implement LLMClient.
type OpenAIClient struct {
	client         *openai.Client
	model          string
	responseFormat *openai.ChatCompletionResponseFormat
}

// NewOpenAIClient creates an OpenAIClient.
// responseFormatType 支持 json_object / json_schema / text，空字符串默认为 json_object。
func NewOpenAIClient(apiKey, baseURL, model, responseFormatType string) (*OpenAIClient, error) {
	config := openai.DefaultConfig(apiKey)
	if baseURL != "" {
		config.BaseURL = baseURL
	}

	rf, err := buildResponseFormat(responseFormatType)
	if err != nil {
		return nil, err
	}

	return &OpenAIClient{
		client:         openai.NewClientWithConfig(config),
		model:          model,
		responseFormat: rf,
	}, nil
}

func buildResponseFormat(responseFormatType string) (*openai.ChatCompletionResponseFormat, error) {
	switch responseFormatType {
	case "", "json_object":
		return &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONObject,
		}, nil
	case "json_schema":
		schema, err := jsonschema.GenerateSchemaForType(types.AnalysisResult{})
		if err != nil {
			return nil, fmt.Errorf("generate json schema: %w", err)
		}
		return &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONSchema,
			JSONSchema: &openai.ChatCompletionResponseFormatJSONSchema{
				Name:   "analysis_result",
				Schema: schema,
				Strict: true,
			},
		}, nil
	case "text":
		return &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeText,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported response_format %q, must be one of [json_object, json_schema, text]", responseFormatType)
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
		ResponseFormat: c.responseFormat,
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
	if err := unmarshalAnalysisResult(&result, content, c.responseFormat); err != nil {
		return nil, err
	}

	return &result, nil
}

func unmarshalAnalysisResult(result *types.AnalysisResult, content string, rf *openai.ChatCompletionResponseFormat) error {
	cleaned := stripMarkdownCodeFence(content)
	if rf != nil && rf.JSONSchema != nil {
		if def, ok := rf.JSONSchema.Schema.(*jsonschema.Definition); ok {
			if err := def.Unmarshal(cleaned, result); err != nil {
				return fmt.Errorf("unmarshal analysis result against schema: %w", err)
			}
			return nil
		}
	}
	if err := json.Unmarshal([]byte(cleaned), result); err != nil {
		return fmt.Errorf("unmarshal analysis result: %w", err)
	}
	return nil
}

func stripMarkdownCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	// 找到第一行结尾（跳过 ```json 等语言标识符）
	if idx := strings.Index(s, "\n"); idx != -1 {
		s = s[idx+1:]
	}
	// 去掉尾部的 ```
	if strings.HasSuffix(s, "```") {
		s = s[:len(s)-3]
	}
	return strings.TrimSpace(s)
}
