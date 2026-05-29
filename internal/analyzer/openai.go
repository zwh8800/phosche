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

type OpenAIClient struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
}

func NewOpenAIClient(apiKey, baseURL, model string) *OpenAIClient {
	return &OpenAIClient{
		apiKey:     apiKey,
		baseURL:    baseURL,
		model:      model,
		httpClient: &http.Client{},
	}
}

type openAIChatRequest struct {
	Model          string                 `json:"model"`
	Messages       []openAIMessage        `json:"messages"`
	ResponseFormat *openAIResponseFormat  `json:"response_format,omitempty"`
}

type openAIMessage struct {
	Role    string        `json:"role"`
	Content []openAIContentPart `json:"content"`
}

type openAIContentPart struct {
	Type     string              `json:"type"`
	Text     string              `json:"text,omitempty"`
	ImageURL *openAIImageURL     `json:"image_url,omitempty"`
}

type openAIImageURL struct {
	URL string `json:"url"`
}

type openAIResponseFormat struct {
	Type string `json:"type"`
}

type openAIChatResponse struct {
	ID      string              `json:"id"`
	Object  string              `json:"object"`
	Created int64               `json:"created"`
	Model   string              `json:"model"`
	Choices []openAIChoice      `json:"choices"`
}

type openAIChoice struct {
	Index        int            `json:"index"`
	Message      openAIMessageResp `json:"message"`
	FinishReason string         `json:"finish_reason"`
}

type openAIMessageResp struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (c *OpenAIClient) AnalyzeImage(ctx context.Context, imageData []byte, prompt string) (*types.AnalysisResult, error) {
	encodedImage := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(imageData)

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
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	slog.Debug("openai: sending request",
		"url", c.baseURL+"/v1/chat/completions",
		"model", c.model,
		"prompt", truncate(prompt, 200),
		"image_bytes", len(imageData),
	)

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
