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

type OllamaClient struct {
	baseURL    string
	model      string
	httpClient *http.Client
}

func NewOllamaClient(baseURL, model string) *OllamaClient {
	return &OllamaClient{
		baseURL:    baseURL,
		model:      model,
		httpClient: &http.Client{},
	}
}

type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	Format   string          `json:"format"`
}

type ollamaMessage struct {
	Role    string   `json:"role"`
	Content string   `json:"content"`
	Images  []string `json:"images,omitempty"`
}

type ollamaChatResponse struct {
	Model     string         `json:"model"`
	CreatedAt string         `json:"created_at"`
	Message   ollamaMessage  `json:"message"`
	Done      bool           `json:"done"`
}

func (c *OllamaClient) AnalyzeImage(ctx context.Context, imageData []byte, prompt string) (*types.AnalysisResult, error) {
	encodedImage := base64.StdEncoding.EncodeToString(imageData)

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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/chat", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama API error (status %d): %s", resp.StatusCode, string(respBytes))
	}

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
