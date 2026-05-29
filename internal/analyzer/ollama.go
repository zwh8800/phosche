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

// OllamaClient 实现基于 Ollama 协议的大语言模型客户端，通过 POST /api/chat 接口
// 以 base64 编码发送图片和分析请求，适用于本地部署的 Ollama 视觉模型。
type OllamaClient struct {
	baseURL    string
	model      string
	httpClient *http.Client
}

// NewOllamaClient 创建一个新的 OllamaClient 实例。
// baseURL 为 Ollama 服务地址（如 http://localhost:11434），model 为视觉模型名称。
func NewOllamaClient(baseURL, model string) *OllamaClient {
	return &OllamaClient{
		baseURL:    baseURL,
		model:      model,
		httpClient: &http.Client{},
	}
}

// ollamaChatRequest 映射 Ollama /api/chat 接口的请求格式。
// Stream 固定为 false（非流式），Format 固定为 "json"（要求结构化 JSON 输出）。
// ollamaChatRequest 是发送给 Ollama /api/chat 端点的请求体结构。
type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	Format   string          `json:"format"`
}

// ollamaMessage 表示 Ollama 聊天消息，包含角色、文本内容和图片数据。
// Images 字段存储 base64 编码的图片字符串，omitempty 表示无图片时可省略。
// ollamaMessage 是 Ollama 对话消息结构，Images 字段携带 base64 编码的图片数据。
type ollamaMessage struct {
	Role    string   `json:"role"`
	Content string   `json:"content"`
	Images  []string `json:"images,omitempty"`
}

// ollamaChatResponse 映射 Ollama /api/chat 接口的响应格式。
// ollamaChatResponse 是 Ollama /api/chat 端点的响应体结构。
type ollamaChatResponse struct {
	Model     string        `json:"model"`
	CreatedAt string        `json:"created_at"`
	Message   ollamaMessage `json:"message"`
	Done      bool          `json:"done"`
}

// AnalyzeImage 使用 Ollama 视觉模型分析图片内容，返回结构化的分析结果。
// 处理流程：
//  1. 将图片数据 base64 编码为字符串
//  2. 构造 Ollama /api/chat 请求，图片放入 images 数组，设置 Stream=false, Format="json"
//  3. POST 请求到 {baseURL}/api/chat
//  4. 解析 JSON 格式的响应，提取 message.content
//  5. 将 content 反序列化为 AnalysisResult 结构体并返回
func (c *OllamaClient) AnalyzeImage(ctx context.Context, imageData []byte, prompt string) (*types.AnalysisResult, error) {
	// 将图片数据编码为 base64 字符串
	encodedImage := base64.StdEncoding.EncodeToString(imageData)

	// 构造 Ollama /api/chat 请求体
	reqBody := ollamaChatRequest{
		Model:  c.model,
		Stream: false,
		Format: "json",
		Messages: []ollamaMessage{
			{
				Role:    "user",
				Content: prompt,
				Images:  []string{encodedImage},
			},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// 构造 HTTP POST 请求
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/chat", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	slog.Debug("ollama: sending request",
		"url", c.baseURL+"/api/chat",
		"model", c.model,
		"prompt", truncate(prompt, 200),
		"image_bytes", len(imageData),
		"image_base64_len", len(encodedImage),
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

	slog.Debug("ollama: received response",
		"status", resp.StatusCode,
		"body", truncate(string(respBytes), 2000),
	)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama API error (status %d): %s", resp.StatusCode, string(respBytes))
	}

	// 解析 Ollama 响应，提取 Message.Content 中的 JSON 分析结果
	var chatResp ollamaChatResponse
	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	content := chatResp.Message.Content
	var result types.AnalysisResult

	if err := json.Unmarshal([]byte(content), &result); err != nil {
		slog.Warn("ollama: failed to parse JSON response", "raw_content", truncate(content, 500))
		return nil, fmt.Errorf("unmarshal analysis result: %w", err)
	}

	return &result, nil
}
