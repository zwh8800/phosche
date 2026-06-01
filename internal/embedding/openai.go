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

// OpenAIEmbedder 实现基于 OpenAI /v1/embeddings 端点的 Embedder 接口。
//
// OpenAI Embedding API 特性（来自官方文档）：
//   - 端点：POST /v1/embeddings
//   - 认证：Bearer Token（Authorization: Bearer sk-...）
//   - 模型选项：
//     text-embedding-3-small：默认 1536 维度，最经济
//     text-embedding-3-large：默认 3072 维度，最高质量
//     text-embedding-ada-002：默认 1536 维度，上一代模型（已弃用）
//   - dimensions：支持 Matryoshka 维度截断（仅 text-embedding-3 系列）
//     可任意缩减维度而不损失概念表示能力
//   - encoding_format："float"（默认）或 "base64"
//   - 输入限制：
//     单条文本最多 8192 个 token
//     单请求最多 2048 条文本
//     单请求总计最多 300,000 个 token
//   - 输出向量已归一化（可用于点积/余弦相似度）
//   - 稳定输出：相同输入始终产生相同向量（确定性的）
//   - 价格：text-embedding-3-small $0.02/1M tokens,
//     text-embedding-3-large $0.13/1M tokens
//
// 端点：POST {baseURL}/embeddings
type OpenAIEmbedder struct {
	apiKey     string
	baseURL    string
	model      string
	dimensions int // 0 表示使用模型默认维度
	batchSize  int // 每批次最大文本数
	httpClient *http.Client
}

// NewOpenAIEmbedder 创建 OpenAI Embedding 客户端。
// model: 模型名，如 "text-embedding-3-small"
// 默认 batchSize 为 32，可根据需求调整（最大 2048）。
func NewOpenAIEmbedder(cfg Config) *OpenAIEmbedder {
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	batchSize := cfg.BatchSize
	if batchSize <= 0 {
		batchSize = 32
	}

	return &OpenAIEmbedder{
		apiKey:     cfg.OpenAI.APIKey,
		baseURL:    cfg.OpenAI.BaseURL,
		model:      cfg.Model,
		dimensions: cfg.Dimensions,
		batchSize:  batchSize,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// --- Request / Response 结构体 ---

// openAIEmbedRequest 对应 OpenAI /v1/embeddings 的请求体。
// 参考：https://platform.openai.com/docs/api-reference/embeddings/create
type openAIEmbedRequest struct {
	Model          string `json:"model"`
	Input          json.RawMessage `json:"input"`                     // string 或 []string
	Dimensions     *int   `json:"dimensions,omitempty"`                // Matryoshka 维度截断（仅 3.x 系列）
	EncodingFormat string `json:"encoding_format,omitempty"`           // "float" 或 "base64"
	User           string `json:"user,omitempty"`                      // 终端用户标识（可选）
}

// openAIEmbedResponse 对应 OpenAI /v1/embeddings 的响应体。
type openAIEmbedResponse struct {
	Object string          `json:"object"` // "list"
	Data   []openAIEmbedData `json:"data"`
	Model  string          `json:"model"`
	Usage  openAIUsage     `json:"usage"`
}

// openAIEmbedData 表示单条文本的嵌入向量。
type openAIEmbedData struct {
	Object    string        `json:"object"`    // "embedding"
	Index     int           `json:"index"`     // 对应输入数组中的位置
	Embedding []float32     `json:"embedding"` // 浮点向量
}

// openAIUsage 记录请求的 token 用量。
type openAIUsage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// openAIErrorResponse OpenAI API 标准错误响应体。
type openAIErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code,omitempty"`
	} `json:"error"`
}

// --- Embedder 接口实现 ---

// EmbedText 对单条文本生成向量嵌入。
func (c *OpenAIEmbedder) EmbedText(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := c.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("openai embed: empty response embeddings")
	}
	return embeddings[0], nil
}

// EmbedBatch 对一批文本批量生成向量嵌入。
// OpenAI 单请求最多 2048 条 / 30 万 token，超出时会自动分片。
func (c *OpenAIEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	// 如果超出批次大小或单个请求的文本数限制，自动分片
	// OpenAI 限制：单请求最多 2048 条
	const maxOpenAIBatch = 2048

	batchSize := c.batchSize
	if batchSize <= 0 || batchSize > maxOpenAIBatch {
		batchSize = maxOpenAIBatch
	}

	// 如果总文本数超过批次大小，分批处理并合并结果
	if len(texts) > batchSize {
		return c.batchSplit(ctx, texts, batchSize)
	}

	return c.doEmbed(ctx, texts)
}

// doEmbed 执行单次 Embedding API 请求。
func (c *OpenAIEmbedder) doEmbed(ctx context.Context, texts []string) ([][]float32, error) {
	inputRaw, err := json.Marshal(texts)
	if err != nil {
		return nil, fmt.Errorf("openai embed: marshal input: %w", err)
	}

	// 构造请求体
	reqBody := openAIEmbedRequest{
		Model:          c.model,
		Input:          inputRaw,
		Dimensions:     optionalInt(c.dimensions),
		EncodingFormat: "float",
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("openai embed: marshal request: %w", err)
	}

	// 创建 HTTP 请求（使用 context 控制超时/取消）
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/embeddings", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("openai embed: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	slog.Debug("openai embed: sending request",
		"url", c.baseURL+"/embeddings",
		"model", c.model,
		"batch_size", len(texts),
		"dimensions", c.dimensions,
	)

	// 发送请求
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai embed: send request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openai embed: read response: %w", err)
	}

	// 错误路径：先尝试解析为标准错误响应
	if resp.StatusCode != http.StatusOK {
		// 尝试解析 OpenAI 标准错误格式
		var errResp openAIErrorResponse
		if json.Unmarshal(respBytes, &errResp) == nil && errResp.Error.Message != "" {
			return nil, &EmbeddingError{
				Provider:    "openai",
				StatusCode:  resp.StatusCode,
				Message:     errResp.Error.Message,
				Retryable:   isOpenAIRetryable(resp.StatusCode, errResp.Error.Type),
			}
		}
		return nil, &EmbeddingError{
			Provider:    "openai",
			StatusCode:  resp.StatusCode,
			Message:     fmt.Sprintf("openai API error (status %d): %s", resp.StatusCode, string(respBytes)),
			Retryable:   resp.StatusCode >= 500,
		}
	}

	// 解析成功响应
	var embedResp openAIEmbedResponse
	if err := json.Unmarshal(respBytes, &embedResp); err != nil {
		return nil, fmt.Errorf("openai embed: unmarshal response: %w", err)
	}

	// 按 index 排序输出，保证与输入顺序一致
	result := make([][]float32, len(texts))
	for _, d := range embedResp.Data {
		if d.Index >= 0 && d.Index < len(result) {
			result[d.Index] = d.Embedding
		}
	}

	// 校验：确保每个输入都有对应的输出
	for i, emb := range result {
		if emb == nil {
			return nil, fmt.Errorf("openai embed: missing embedding at index %d", i)
		}
	}

	slog.Debug("openai embed: success",
		"model", embedResp.Model,
		"vectors", len(result),
		"dim", len(result[0]),
		"prompt_tokens", embedResp.Usage.PromptTokens,
		"total_tokens", embedResp.Usage.TotalTokens,
	)

	return result, nil
}

// batchSplit 将大量文本分片后逐一请求，合并结果。
// 每片的大小为 batchSize。
func (c *OpenAIEmbedder) batchSplit(ctx context.Context, texts []string, batchSize int) ([][]float32, error) {
	total := len(texts)
	// 预分配结果切片
	result := make([][]float32, total)

	for i := 0; i < total; i += batchSize {
		end := i + batchSize
		if end > total {
			end = total
		}

		batch := texts[i:end]
		slog.Debug("openai embed: processing batch",
			"batch_start", i,
			"batch_end", end,
			"batch_size", len(batch),
		)

		embeddings, err := c.doEmbed(ctx, batch)
		if err != nil {
			return nil, fmt.Errorf("openai embed: batch [%d:%d]: %w", i, end, err)
		}

		copy(result[i:end], embeddings)
	}

	return result, nil
}

// SetHTTPClient 允许注入自定义 HTTP 客户端（用于测试）。
func (c *OpenAIEmbedder) SetHTTPClient(client *http.Client) {
	c.httpClient = client
}

// isOpenAIRetryable 判断 OpenAI 错误是否可重试。
// 参考 OpenAI 错误码文档：
//   - 401/403：认证错误，不可重试
//   - 400/422：请求错误（超长输入等），通常不可重试
//   - 429：限流，可重试
//   - 500/503：服务端错误，可重试
func isOpenAIRetryable(statusCode int, errType string) bool {
	switch statusCode {
	case 429:
		return true // 限流，可重试
	case 500, 502, 503:
		return true // 服务端临时错误
	}

	// 根据错误类型判断
	switch errType {
	case "rate_limit_error":
		return true
	case "server_error":
		return true
	}

	return false
}
