package analyzer

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/zwh8800/phosche/internal/types"
)

// OpenAIClient 实现基于 OpenAI（或兼容 API）协议的大语言模型客户端，
// 通过 POST /v1/chat/completions 接口以 data URL 格式发送图片，使用 Bearer Token 认证。
type OpenAIClient struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
}

// NewOpenAIClient 创建一个新的 OpenAIClient 实例。
// apiKey 为 API 认证密钥，baseURL 为 API 基础地址，model 为视觉模型名称。
func NewOpenAIClient(apiKey, baseURL, model string) *OpenAIClient {
	return &OpenAIClient{
		apiKey:     apiKey,
		baseURL:    baseURL,
		model:      model,
		httpClient: &http.Client{},
	}
}

// openAIChatRequest 映射 OpenAI /v1/chat/completions 接口的请求格式。
// ResponseFormat 设置为 json_object 以强制 LLM 返回结构化 JSON 输出。
// openAIChatRequest 是发送给 OpenAI /v1/chat/completions 端点的请求体结构。
type openAIChatRequest struct {
	Model          string                `json:"model"`
	Messages       []openAIMessage       `json:"messages"`
	ResponseFormat *openAIResponseFormat `json:"response_format,omitempty"`
}

// openAIMessage 表示一条 OpenAI 聊天消息，使用多部分内容（text + image_url）来描述用户输入。
// Content 字段为 openAIContentPart 切片，支持同时包含文本和图片内容。
// openAIMessage 是 OpenAI 对话消息，Content 为多部分内容（文本 + 图片）数组。
type openAIMessage struct {
	Role    string              `json:"role"`
	Content []openAIContentPart `json:"content"`
}

// openAIContentPart 表示消息内容的一个部分。
// Type 字段区分内容类型："text" 为纯文本，"image_url" 为图片数据。
// openAIContentPart 是多部分消息中的一个内容块，可以是文本或图片。
type openAIContentPart struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	ImageURL *openAIImageURL `json:"image_url,omitempty"`
}

// openAIImageURL 包装图片的 data URL。
// URL 字段使用 data URL 格式：data:image/jpeg;base64,<base64编码的数据>
// openAIImageURL 包含图片的 data URL（data:image/jpeg;base64,...）。
type openAIImageURL struct {
	URL string `json:"url"`
}

// openAIResponseFormat 指定 LLM 响应的输出格式。
// Type 设置为 "json_object" 时，模型将强制返回合法的 JSON 对象。
// openAIResponseFormat 指定 OpenAI 返回的响应格式，设为 json_object 以确保结构化输出。
type openAIResponseFormat struct {
	Type string `json:"type"`
}

// openAIChatResponse 映射 OpenAI /v1/chat/completions 接口的响应格式。
type openAIChatResponse struct {
	ID      string              `json:"id"`
	Object  string              `json:"object"`
	Created int64               `json:"created"`
	Model   string              `json:"model"`
	Choices []openAIChoice      `json:"choices"`
}

// openAIChoice 表示 LLM 返回的一个候选回复。
type openAIChoice struct {
	Index        int            `json:"index"`
	Message      openAIMessageResp `json:"message"`
	FinishReason string         `json:"finish_reason"`
}

// openAIMessageResp 表示 LLM 返回的消息内容。
// Content 字段包含模型生成的文本，在本应用中为 JSON 格式的分析结果。
type openAIMessageResp struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// AnalyzeImage 使用 OpenAI 视觉模型分析图片内容，返回结构化的分析结果。
// 处理流程：
//  1. 将图片数据编码为 data URL 格式（data:image/jpeg;base64,...）
//  2. 构造多部分消息，包含提示词文本（type=text）和图片（type=image_url）
//  3. POST 请求到 {baseURL}/v1/chat/completions，使用 Bearer Token 认证
//  4. 解析响应中的 choices[0].message.content（JSON 字符串）
//  5. 将 content 反序列化为 AnalysisResult 结构体并返回
func (c *OpenAIClient) AnalyzeImage(ctx context.Context, imageData []byte, prompt string) (*types.AnalysisResult, error) {
	// 构造 data URL：data:image/jpeg;base64,<base64数据>
	encodedImage := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(imageData)

	// 构造 OpenAI chat completion 请求，content 包含 text 和 image_url 两部分
	reqBody := openAIChatRequest{
		Model: c.model,
		Messages: []openAIMessage{
			{
				Role: "user",
				Content: []openAIContentPart{
					{
						Type: "text",
						Text: prompt,
					},
					{
						Type: "image_url",
						ImageURL: &openAIImageURL{
							URL: encodedImage,
						},
					},
				},
			},
		},
		ResponseFormat: &openAIResponseFormat{
			Type: "json_object",
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// Bearer Token 认证
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	slog.Debug("openai: sending request",
		"url", c.baseURL+"/v1/chat/completions",
		"model", c.model,
		"prompt", truncate(prompt, 200),
		"image_bytes", len(imageData),
	)

	// 发送请求并读取响应
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	slog.Debug("openai: received response",
		"status", resp.StatusCode,
		"body", truncate(string(respBytes), 2000),
	)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai API error (status %d): %s", resp.StatusCode, string(respBytes))
	}

	// 解析 OpenAI 响应，从 choices[0].message.content 提取 JSON 分析结果
	var chatResp openAIChatResponse
	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("openai response has no choices")
	}

	content := chatResp.Choices[0].Message.Content
	var result types.AnalysisResult

	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("unmarshal analysis result: %w", err)
	}

	return &result, nil
}
