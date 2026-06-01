package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// OllamaEmbedder 实现基于 Ollama /api/embed 端点的 Embedder 接口。
//
// Ollama /api/embed 特性（来自官方文档）：
//   - 模型：bge-m3（1.2GB，8K 上下文）、mxbai-embed-large（334M）、
//     nomic-embed-text（137M）、all-minilm（23M）等
//   - 输入：单条文本字符串 或 字符串数组（批量）
//   - truncate：默认 true，超出上下文窗口时截断而非报错
//   - dimensions：支持维度截断（Matryoshka），仅部分模型支持
//   - 返回：L2 归一化浮点向量（float32）
//   - 输出维度取决于模型：mxbai-embed-large=1024, nomic-embed-text=768,
//     all-minilm=384, bge-m3=1024
//   - 旧 /api/embeddings 端点已废弃，返回 float64 且没有归一化
//
// 端点：POST http://{baseURL}/api/embed
type OllamaEmbedder struct {
	baseURL    string
	model      string
	dimensions int        // 0 表示使用模型默认维度
	httpClient *http.Client
}

// NewOllamaEmbedder 创建 Ollama Embedding 客户端。
// 使用独立的 HTTP 客户端，配置 30 秒超时（可通过 Config.TimeoutSeconds 覆盖）。
func NewOllamaEmbedder(cfg Config) *OllamaEmbedder {
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	return &OllamaEmbedder{
		baseURL:    cfg.Ollama.BaseURL,
		model:      cfg.Model,
		dimensions: cfg.Dimensions,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// --- Request / Response 结构体 ---

// ollamaEmbedRequest 对应 Ollama /api/embed 的请求体。
// 参考：https://docs.ollama.com/api/embed
type ollamaEmbedRequest struct {
	Model      string         `json:"model"`
	Input      json.RawMessage `json:"input"` // string 或 []string，用 RawMessage 动态处理
	Truncate   *bool          `json:"truncate,omitempty"`   // 默认 true
	Dimensions *int           `json:"dimensions,omitempty"`  // 可选维度截断
	KeepAlive  string         `json:"keep_alive,omitempty"`  // 模型保持加载时长
	Options    map[string]any `json:"options,omitempty"`     // 模型参数（如 num_ctx）
}

// ollamaEmbedResponse 对应 Ollama /api/embed 的响应体。
type ollamaEmbedResponse struct {
	Model           string      `json:"model"`
	Embeddings      [][]float32 `json:"embeddings"`
	TotalDuration   int64       `json:"total_duration"`
	LoadDuration    int64       `json:"load_duration"`
	PromptEvalCount int         `json:"prompt_eval_count"`
}

// --- Embedder 接口实现 ---

// EmbedText 对单条文本生成向量嵌入。
// 实现：将单条文本包装为数组后调用 EmbedBatch，再取第一项返回。
func (c *OllamaEmbedder) EmbedText(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := c.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("ollama embed: empty response embeddings")
	}
	return embeddings[0], nil
}

// EmbedBatch 对一批文本批量生成向量嵌入。
// Ollama /api/embed 原生支持数组输入，无需手动分片。
// 如果配置了 Dimensions 且大于 0，会请求模型返回指定维度的向量。
func (c *OllamaEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	// 序列化 input 为 JSON 数组
	inputRaw, err := json.Marshal(texts)
	if err != nil {
		return nil, fmt.Errorf("ollama embed: marshal input: %w", err)
	}

	// 构造请求体
	reqBody := ollamaEmbedRequest{
		Model:      c.model,
		Input:      inputRaw,
		Dimensions: optionalInt(c.dimensions),
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("ollama embed: marshal request: %w", err)
	}

	// 创建 HTTP 请求（使用 context 控制超时/取消）
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/embed", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("ollama embed: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	slog.Debug("ollama embed: sending request",
		"url", c.baseURL+"/api/embed",
		"model", c.model,
		"batch_size", len(texts),
		"dimensions", c.dimensions,
	)

	// 发送请求
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama embed: send request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ollama embed: read response: %w", err)
	}

	slog.Debug("ollama embed: received response",
		"status", resp.StatusCode,
		"body_size", len(respBytes),
	)

	// 检查 HTTP 状态码
	if resp.StatusCode != http.StatusOK {
		return nil, &EmbeddingError{
			Provider:    "ollama",
			StatusCode:  resp.StatusCode,
			Message:     fmt.Sprintf("ollama API error (status %d): %s", resp.StatusCode, string(respBytes)),
			Retryable:   resp.StatusCode >= 500,
		}
	}

	// 解析响应
	var embedResp ollamaEmbedResponse
	if err := json.Unmarshal(respBytes, &embedResp); err != nil {
		return nil, fmt.Errorf("ollama embed: unmarshal response: %w", err)
	}

	if len(embedResp.Embeddings) == 0 {
		return nil, fmt.Errorf("ollama embed: empty embeddings in response")
	}

	slog.Debug("ollama embed: success",
		"model", embedResp.Model,
		"vectors", len(embedResp.Embeddings),
		"dim", len(embedResp.Embeddings[0]),
		"total_duration_ns", embedResp.TotalDuration,
		"prompt_eval_count", embedResp.PromptEvalCount,
	)

	return embedResp.Embeddings, nil
}

// SetHTTPClient 允许注入自定义 HTTP 客户端（用于测试）。
func (c *OllamaEmbedder) SetHTTPClient(client *http.Client) {
	c.httpClient = client
}

// optionalInt 将 int 值转换为 *int，值为 0 时返回 nil（omitempty 不发送）。
func optionalInt(v int) *int {
	if v > 0 {
		return &v
	}
	return nil
}
