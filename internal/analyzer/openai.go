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

// OpenAIClient 是基于 go-openai 库的 LLM 客户端实现。
// 支持 OpenAI 官方 API 和兼容接口（如本地 Ollama）。
type OpenAIClient struct {
	client              *openai.Client
	model               string
	responseFormat      *openai.ChatCompletionResponseFormat
	maxTokens           int
	maxCompletionTokens int
}

// NewOpenAIClient 创建一个新的 OpenAI 兼容客户端。
// responseFormatType 支持 json_object / json_schema / text，空字符串默认为 json_object。
// maxTokens 控制可见输出 token 数（兼容 LMStudio/Ollama），0 表示不设置。
// maxCompletionTokens 控制总生成 token 数（含 reasoning，兼容 OpenAI 新 API），0 表示不设置。
func NewOpenAIClient(apiKey, baseURL, model, responseFormatType string, maxTokens, maxCompletionTokens int) (*OpenAIClient, error) {
	config := openai.DefaultConfig(apiKey)
	if baseURL != "" {
		config.BaseURL = baseURL
	}

	rf, err := buildResponseFormat(responseFormatType)
	if err != nil {
		return nil, err
	}

	return &OpenAIClient{
		client:              openai.NewClientWithConfig(config),
		model:               model,
		responseFormat:      rf,
		maxTokens:           maxTokens,
		maxCompletionTokens: maxCompletionTokens,
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

// AnalyzeImage 使用多模态 LLM 分析图片内容。
// 返回结构化的分析结果，包含描述、标签、场景类型等。
// 如果 API 调用失败或响应格式无效，返回错误。
func (c *OpenAIClient) AnalyzeImage(ctx context.Context, imageData []byte, systemPrompt string, userPrompt string) (*types.AnalysisResult, error) {
	encodedImage := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(imageData)

	req := openai.ChatCompletionRequest{
		Model: c.model,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: systemPrompt,
			},
			{
				Role: openai.ChatMessageRoleUser,
				MultiContent: []openai.ChatMessagePart{
					{
						Type: openai.ChatMessagePartTypeText,
						Text: userPrompt,
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
		ResponseFormat:      c.responseFormat,
		MaxTokens:           c.maxTokens,
		MaxCompletionTokens: c.maxCompletionTokens,
	}

	logRequest(req)

	resp, err := c.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("openai request failed: %w", err)
	}

	logResponse(resp)

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("openai response has no choices")
	}

	content := resp.Choices[0].Message.Content

	var result types.AnalysisResult
	if err := unmarshalAnalysisResult(&result, content, c.responseFormat); err != nil {
		return nil, err
	}

	return &result, nil
}

func logRequest(req openai.ChatCompletionRequest) {
	data, err := json.Marshal(req)
	if err != nil {
		return
	}

	var reqForLog openai.ChatCompletionRequest
	if err := json.Unmarshal(data, &reqForLog); err != nil {
		return
	}

	imageURL := reqForLog.Messages[1].MultiContent[1].ImageURL.URL
	reqForLog.Messages[1].MultiContent[1].ImageURL = &openai.ChatMessageImageURL{
		URL: imageURL[:min(len(imageURL), 100)] + "...(truncated)",
	}

	if reqJSON, err := json.Marshal(reqForLog); err == nil {
		slog.Debug("openai: sending request", "request", string(reqJSON))
	}
}

func logResponse(resp openai.ChatCompletionResponse) {
	if respJSON, err := json.Marshal(resp); err == nil {
		slog.Debug("openai: received response", "response", string(respJSON))
	}
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
