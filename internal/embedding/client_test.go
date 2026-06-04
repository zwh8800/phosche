package embedding

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// --- Test Data ---

var testVector = []float32{0.1, 0.2, 0.3, 0.4, 0.5}
var testVector2 = []float32{0.6, 0.7, 0.8, 0.9, 1.0}

// --- OpenAI Tests ---

func TestOpenAIEmbedder_RequestFormat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/embeddings" {
			t.Errorf("expected /embeddings, got %s", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer sk-test" {
			t.Errorf("expected Bearer sk-test, got %s", auth)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", ct)
		}

		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode body: %v", err)
		}
		if model, ok := body["model"].(string); !ok || model != "text-embedding-3-small" {
			t.Errorf("expected model 'text-embedding-3-small', got %v", body["model"])
		}

		resp := openAIEmbedResponse{
			Object: "list",
			Data: []openAIEmbedData{
				{Object: "embedding", Index: 0, Embedding: testVector},
				{Object: "embedding", Index: 1, Embedding: testVector2},
			},
			Model: "text-embedding-3-small",
			Usage: openAIUsage{PromptTokens: 10, TotalTokens: 10},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.OpenAI.APIKey = "sk-test"
	cfg.OpenAI.BaseURL = server.URL
	cfg.Model = "text-embedding-3-small"
	client := NewOpenAIEmbedder(cfg)

	embeddings, err := client.EmbedBatch(context.Background(), []string{"hello", "world"})
	if err != nil {
		t.Fatalf("EmbedBatch failed: %v", err)
	}
	if len(embeddings) != 2 {
		t.Fatalf("expected 2 embeddings, got %d", len(embeddings))
	}
}

func TestOpenAIEmbedder_SingleText(t *testing.T) {
	embSingle := []float32{0.11, 0.22, 0.33, 0.44}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openAIEmbedResponse{
			Object: "list",
			Data: []openAIEmbedData{
				{Object: "embedding", Index: 0, Embedding: embSingle},
			},
			Model: "text-embedding-3-small",
			Usage: openAIUsage{PromptTokens: 5, TotalTokens: 5},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.OpenAI.APIKey = "sk-test"
	cfg.OpenAI.BaseURL = server.URL
	cfg.Model = "text-embedding-3-small"
	client := NewOpenAIEmbedder(cfg)

	vec, err := client.EmbedText(context.Background(), "test")
	if err != nil {
		t.Fatalf("EmbedText failed: %v", err)
	}
	if len(vec) != len(embSingle) {
		t.Errorf("expected vector length %d, got %d", len(embSingle), len(vec))
	}
}

func TestOpenAIEmbedder_WithDimensions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)

		if dims, ok := body["dimensions"].(float64); !ok || dims != 256 {
			t.Errorf("expected dimensions 256, got %v", body["dimensions"])
		}

		shortVec := make([]float32, 256)
		resp := openAIEmbedResponse{
			Object: "list",
			Data:   []openAIEmbedData{{Object: "embedding", Index: 0, Embedding: shortVec}},
			Model:  "text-embedding-3-small",
			Usage:  openAIUsage{PromptTokens: 5, TotalTokens: 5},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.OpenAI.APIKey = "sk-test"
	cfg.OpenAI.BaseURL = server.URL
	cfg.Model = "text-embedding-3-small"
	cfg.Dimensions = 256
	client := NewOpenAIEmbedder(cfg)

	vec, err := client.EmbedText(context.Background(), "test")
	if err != nil {
		t.Fatalf("EmbedText failed: %v", err)
	}
	if len(vec) != 256 {
		t.Errorf("expected 256-dim vector, got %d", len(vec))
	}
}

func TestOpenAIEmbedder_EmptyInput(t *testing.T) {
	cfg := DefaultConfig()
	client := NewOpenAIEmbedder(cfg)
	embeddings, err := client.EmbedBatch(context.Background(), []string{})
	if err != nil {
		t.Fatalf("EmbedBatch with empty input should succeed: %v", err)
	}
	if len(embeddings) != 0 {
		t.Errorf("expected empty result, got %d", len(embeddings))
	}
}

func TestOpenAIEmbedder_StandardErrorFormat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"Incorrect API key provided","type":"invalid_request_error","code":"invalid_api_key"}}`))
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.OpenAI.APIKey = "sk-wrong"
	cfg.OpenAI.BaseURL = server.URL
	cfg.Model = "text-embedding-3-small"
	client := NewOpenAIEmbedder(cfg)

	_, err := client.EmbedText(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for unauthorized request")
	}

	var eei *EmbeddingError
	if !errors.As(err, &eei) {
		t.Fatalf("expected EmbeddingError, got %T", err)
	}
	if eei.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", eei.StatusCode)
	}
	if !strings.Contains(eei.Message, "Incorrect API key") {
		t.Errorf("expected error message about API key, got: %s", eei.Message)
	}
	if eei.Retryable {
		t.Error("expected 401 error to be non-retryable")
	}
}

func TestOpenAIEmbedder_RateLimitError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"Rate limit exceeded","type":"rate_limit_error","code":"rate_limit"}}`))
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.OpenAI.APIKey = "sk-test"
	cfg.OpenAI.BaseURL = server.URL
	cfg.Model = "text-embedding-3-small"
	client := NewOpenAIEmbedder(cfg)

	_, err := client.EmbedText(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for rate limit")
	}

	if !IsRetryable(err) {
		t.Error("expected 429 rate limit to be retryable")
	}
}

func TestOpenAIEmbedder_BatchSplit(t *testing.T) {
	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)

		input, ok := body["input"].([]interface{})
		if !ok {
			t.Fatal("expected input array")
		}

		var data []openAIEmbedData
		for i := range input {
			vec := make([]float32, 4)
			data = append(data, openAIEmbedData{
				Object:    "embedding",
				Index:     i,
				Embedding: vec,
			})
		}
		resp := openAIEmbedResponse{
			Object: "list",
			Data:   data,
			Model:  "text-embedding-3-small",
			Usage:  openAIUsage{PromptTokens: len(input), TotalTokens: len(input)},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.OpenAI.APIKey = "sk-test"
	cfg.OpenAI.BaseURL = server.URL
	cfg.Model = "text-embedding-3-small"
	cfg.BatchSize = 3 // Force small batches
	client := NewOpenAIEmbedder(cfg)

	texts := []string{"a", "b", "c", "d", "e", "f", "g"}
	embeddings, err := client.EmbedBatch(context.Background(), texts)
	if err != nil {
		t.Fatalf("EmbedBatch failed: %v", err)
	}
	if len(embeddings) != 7 {
		t.Errorf("expected 7 embeddings, got %d", len(embeddings))
	}
	if requestCount != 3 {
		t.Errorf("expected 3 requests (3+3+1), got %d", requestCount)
	}
	for i, emb := range embeddings {
		if emb == nil {
			t.Errorf("embedding[%d] is nil", i)
		}
	}
}

// --- Error Handling Tests ---

func TestIsRetryable_ContextCancelled(t *testing.T) {
	err := context.Canceled
	if IsRetryable(err) {
		t.Error("context.Canceled should not be retryable")
	}
}

func TestIsRetryable_EmbeddingError(t *testing.T) {
	err := &EmbeddingError{
		Provider:   "test",
		StatusCode: 429,
		Message:    "rate limited",
		Retryable:  true,
	}
	if !IsRetryable(err) {
		t.Error("expected retryable")
	}

	err2 := &EmbeddingError{
		Provider:   "test",
		StatusCode: 400,
		Message:    "bad request",
		Retryable:  false,
	}
	if IsRetryable(err2) {
		t.Error("expected non-retryable")
	}
}

func TestIsRetryable_Nil(t *testing.T) {
	if IsRetryable(nil) {
		t.Error("nil should not be retryable")
	}
}

func TestGetEmbeddingError(t *testing.T) {
	err := &EmbeddingError{Provider: "test", Message: "test error"}
	eei := GetEmbeddingError(err)
	if eei == nil {
		t.Fatal("expected EmbeddingError")
	}
	if eei.Provider != "test" {
		t.Errorf("expected provider 'test', got %s", eei.Provider)
	}
}

// --- Factory Tests ---

func TestNewEmbedder_OpenAI(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Provider = ProviderOpenAI
	cfg.OpenAI.APIKey = "sk-test"
	client, err := NewEmbedder(cfg)
	if err != nil {
		t.Fatalf("NewEmbedder failed: %v", err)
	}
	if _, ok := client.(*OpenAIEmbedder); !ok {
		t.Errorf("expected *OpenAIEmbedder, got %T", client)
	}
}

// --- BatchProcessor Tests ---

func TestBatchProcessor_EmptyInput(t *testing.T) {
	cfg := DefaultConfig()
	client, _ := NewEmbedder(cfg)
	processor := NewBatchProcessor(client, DefaultBatchProcessorConfig())

	result, err := processor.Process(context.Background(), []string{})
	if err != nil {
		t.Fatalf("Process with empty input should succeed: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d", len(result))
	}
}

func TestBatchProcessor_WithProgress(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openAIEmbedResponse{
			Object: "list",
			Data: []openAIEmbedData{
				{Object: "embedding", Index: 0, Embedding: testVector},
				{Object: "embedding", Index: 1, Embedding: testVector2},
			},
			Model: "text-embedding-3-small",
			Usage: openAIUsage{PromptTokens: 10, TotalTokens: 10},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.OpenAI.APIKey = "sk-test"
	cfg.OpenAI.BaseURL = server.URL
	cfg.Model = "text-embedding-3-small"
	client := NewOpenAIEmbedder(cfg)

	var progressCalls int
	processor := NewBatchProcessor(client, BatchProcessorConfig{
		OnProgress: func(completed, total int) {
			progressCalls++
		},
	})

	_, err := processor.Process(context.Background(), []string{"hello", "world"})
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	if progressCalls == 0 {
		t.Error("expected progress callback to be called")
	}
}

func TestBatchProcessor_RetrySuccess(t *testing.T) {
	var attempt int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt++
		if attempt <= 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"error":"service unavailable"}`))
			return
		}
		resp := openAIEmbedResponse{
			Object: "list",
			Data:   []openAIEmbedData{{Object: "embedding", Index: 0, Embedding: testVector}},
			Model:  "text-embedding-3-small",
			Usage:  openAIUsage{PromptTokens: 5, TotalTokens: 5},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.OpenAI.APIKey = "sk-test"
	cfg.OpenAI.BaseURL = server.URL
	cfg.Model = "text-embedding-3-small"
	client := NewOpenAIEmbedder(cfg)

	processor := NewBatchProcessor(client, BatchProcessorConfig{
		MaxRetries:       3,
		BaseBackoff:      10 * time.Millisecond,
		FailOnFirstError: true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	vec, err := processor.Process(ctx, []string{"test"})
	if err != nil {
		t.Fatalf("Process failed after retry: %v", err)
	}
	if len(vec) != 1 {
		t.Errorf("expected 1 embedding, got %d", len(vec))
	}
	if attempt != 2 {
		t.Errorf("expected 2 attempts (1 fail + 1 success), got %d", attempt)
	}
}

// --- EmbedTextWithRetry / EmbedBatchWithRetry Tests ---

func TestEmbedTextWithRetry_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openAIEmbedResponse{
			Object: "list",
			Data:   []openAIEmbedData{{Object: "embedding", Index: 0, Embedding: testVector}},
			Model:  "text-embedding-3-small",
			Usage:  openAIUsage{PromptTokens: 5, TotalTokens: 5},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.OpenAI.APIKey = "sk-test"
	cfg.OpenAI.BaseURL = server.URL
	cfg.Model = "text-embedding-3-small"
	client := NewOpenAIEmbedder(cfg)

	vec, err := EmbedTextWithRetry(context.Background(), client, "test", 2)
	if err != nil {
		t.Fatalf("EmbedTextWithRetry failed: %v", err)
	}
	if len(vec) == 0 {
		t.Error("expected non-empty vector")
	}
}

func TestEmbedBatchWithRetry_AfterRetry(t *testing.T) {
	var attempt int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt++
		if attempt <= 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"error":"service unavailable"}`))
			return
		}
		resp := openAIEmbedResponse{
			Object: "list",
			Data:   []openAIEmbedData{{Object: "embedding", Index: 0, Embedding: testVector}},
			Model:  "text-embedding-3-small",
			Usage:  openAIUsage{PromptTokens: 5, TotalTokens: 5},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.OpenAI.APIKey = "sk-test"
	cfg.OpenAI.BaseURL = server.URL
	cfg.Model = "text-embedding-3-small"
	client := NewOpenAIEmbedder(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	vec, err := EmbedBatchWithRetry(ctx, client, []string{"test"}, 3)
	if err != nil {
		t.Fatalf("EmbedBatchWithRetry failed after retry: %v", err)
	}
	if len(vec) != 1 {
		t.Errorf("expected 1 embedding, got %d", len(vec))
	}
}
