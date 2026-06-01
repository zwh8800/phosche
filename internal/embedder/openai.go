package embedder

import (
	"context"
	"fmt"

	openai "github.com/sashabaranov/go-openai"
)

// OpenAIEmbeddingClient 通过 go-openai 库实现文本向量化。
type OpenAIEmbeddingClient struct {
	client     *openai.Client
	model      string
	dimensions int
}

// NewOpenAIEmbeddingClient 创建 OpenAI embedding 客户端。
func NewOpenAIEmbeddingClient(apiKey, baseURL, model string, dimensions int) *OpenAIEmbeddingClient {
	config := openai.DefaultConfig(apiKey)
	if baseURL != "" {
		config.BaseURL = baseURL
	}
	return &OpenAIEmbeddingClient{
		client:     openai.NewClientWithConfig(config),
		model:      model,
		dimensions: dimensions,
	}
}

// Embed 调用 OpenAI embeddings 端点对文本进行向量化。
func (c *OpenAIEmbeddingClient) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	req := openai.EmbeddingRequest{
		Model: openai.EmbeddingModel(c.model),
		Input: texts,
	}
	if c.dimensions > 0 {
		req.Dimensions = c.dimensions
	}

	resp, err := c.client.CreateEmbeddings(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("openai embed request: %w", err)
	}

	if len(texts) == 1 {
		return [][]float32{resp.Data[0].Embedding}, nil
	}

	// Build result array ordered by index
	embeddings := make([][]float32, len(texts))
	for _, d := range resp.Data {
		if d.Index >= len(texts) {
			return nil, fmt.Errorf("unexpected index %d (have %d texts)", d.Index, len(texts))
		}
		embeddings[d.Index] = d.Embedding
	}

	for i, emb := range embeddings {
		if emb == nil {
			return nil, fmt.Errorf("missing embedding for text at index %d", i)
		}
	}

	return embeddings, nil
}
