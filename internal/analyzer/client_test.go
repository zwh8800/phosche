package analyzer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zwh8800/phosche/internal/types"
)

func validAnalysisResultJSON() string {
	return `{"description":"A cat sitting on a windowsill","tags":["cat","windowsill","indoor"],"objects":["cat","window"],"scene_type":"indoor","colors":[{"name":"白色","hex":"#F9FAFB"},{"name":"棕色","hex":"#92400E"}],"people_count":0,"has_text":false,"confidence":0.95}`
}

var expectedResult = &types.AnalysisResult{
	Description: "A cat sitting on a windowsill",
	Tags:        []string{"cat", "windowsill", "indoor"},
	Objects:     []string{"cat", "window"},
	SceneType:   "indoor",
	Colors:      []types.ColorInfo{{Name: "白色", Hex: "#F9FAFB"}, {Name: "棕色", Hex: "#92400E"}},
	PeopleCount: 0,
	HasText:     false,
	Confidence:  0.95,
}

// --- Ollama Provider Tests (via OpenAI-compatible protocol) ---

func TestOllamaProvider_OpenAIProtocol(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Validate method
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		// Validate path — Ollama's OpenAI-compatible endpoint
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("expected path /v1/chat/completions, got %s", r.URL.Path)
		}
		// Validate Authorization header — apiKey is "ollama" for local Ollama
		if auth := r.Header.Get("Authorization"); auth != "Bearer ollama" {
			t.Errorf("expected Authorization 'Bearer ollama', got %s", auth)
		}

		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode body: %v", err)
		}

		// Validate model field
		if model, ok := body["model"].(string); !ok || model != "llama3.2-vision" {
			t.Errorf("expected model 'llama3.2-vision', got %v", body["model"])
		}

		// Validate messages structure with multimodal content array
		messages, ok := body["messages"].([]interface{})
		if !ok || len(messages) == 0 {
			t.Fatal("expected messages array with at least one message")
		}
		msg := messages[0].(map[string]interface{})
		if msg["role"] != "user" {
			t.Errorf("expected role 'user', got %v", msg["role"])
		}

		content, ok := msg["content"].([]interface{})
		if !ok {
			t.Fatal("expected content to be an array (multimodal)")
		}

		hasImageURL := false
		for _, part := range content {
			partMap := part.(map[string]interface{})
			if partMap["type"] == "image_url" {
				hasImageURL = true
				imageURL, ok := partMap["image_url"].(map[string]interface{})
				if !ok {
					t.Error("image_url field missing or invalid")
					continue
				}
				url, ok := imageURL["url"].(string)
				if !ok || !strings.HasPrefix(url, "data:image/jpeg;base64,") {
					t.Errorf("expected data URL with 'data:image/jpeg;base64,' prefix, got %v", url)
				}
			}
		}
		if !hasImageURL {
			t.Error("expected image_url part in content array")
		}

		// Return valid OpenAI-format response
		openAIResp := map[string]interface{}{
			"id":      "chatcmpl-ollama-123",
			"object":  "chat.completion",
			"created": 1234567890,
			"model":   "llama3.2-vision",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": validAnalysisResultJSON(),
					},
					"finish_reason": "stop",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openAIResp)
	}))
	defer server.Close()

	client := NewOpenAIClient("ollama", server.URL+"/v1", "llama3.2-vision")
	result, err := client.AnalyzeImage(context.Background(), []byte("fake-image-data"), "describe this image")
	if err != nil {
		t.Fatalf("AnalyzeImage failed: %v", err)
	}

	if result.Description != expectedResult.Description {
		t.Errorf("expected description %q, got %q", expectedResult.Description, result.Description)
	}
	if result.SceneType != expectedResult.SceneType {
		t.Errorf("expected scene_type %q, got %q", expectedResult.SceneType, result.SceneType)
	}
}

// --- OpenAI Tests ---

func TestOpenAIClient_RequestFormat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Validate method
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		// Validate path
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("expected path /v1/chat/completions, got %s", r.URL.Path)
		}
		// Validate Authorization header
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-api-key" {
			t.Errorf("expected Authorization 'Bearer test-api-key', got %s", auth)
		}

		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode body: %v", err)
		}

		// Validate model field
		if model, ok := body["model"].(string); !ok || model != "gpt-4o" {
			t.Errorf("expected model 'gpt-4o', got %v", body["model"])
		}

		// Validate messages structure with image_url
		messages, ok := body["messages"].([]interface{})
		if !ok || len(messages) == 0 {
			t.Fatal("expected messages array")
		}
		msg := messages[0].(map[string]interface{})
		if msg["role"] != "user" {
			t.Errorf("expected role 'user', got %v", msg["role"])
		}

		content, ok := msg["content"].([]interface{})
		if !ok {
			t.Fatal("expected content to be an array (multimodal)")
		}

		hasImageURL := false
		for _, part := range content {
			partMap := part.(map[string]interface{})
			if partMap["type"] == "image_url" {
				hasImageURL = true
				imageURL, ok := partMap["image_url"].(map[string]interface{})
				if !ok {
					t.Error("image_url field missing or invalid")
				}
				url, ok := imageURL["url"].(string)
				if !ok || !strings.HasPrefix(url, "data:image/jpeg;base64,") {
					t.Errorf("expected data URL prefix, got %v", url)
				}
			}
		}
		if !hasImageURL {
			t.Error("expected image_url in content array")
		}

		// Return valid OpenAI response
		openAIResp := map[string]interface{}{
			"id":      "chatcmpl-123",
			"object":  "chat.completion",
			"created": 1234567890,
			"model":   "gpt-4o",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": validAnalysisResultJSON(),
					},
					"finish_reason": "stop",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openAIResp)
	}))
	defer server.Close()

	client := NewOpenAIClient("test-api-key", server.URL+"/v1", "gpt-4o")
	result, err := client.AnalyzeImage(context.Background(), []byte("fake-image-data"), "describe this image")
	if err != nil {
		t.Fatalf("AnalyzeImage failed: %v", err)
	}

	if result.Description != expectedResult.Description {
		t.Errorf("expected description %q, got %q", expectedResult.Description, result.Description)
	}
}

func TestOpenAIClient_ResponseParsing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		openAIResp := map[string]interface{}{
			"id":      "chatcmpl-123",
			"object":  "chat.completion",
			"created": 1234567890,
			"model":   "gpt-4o",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": validAnalysisResultJSON(),
					},
					"finish_reason": "stop",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openAIResp)
	}))
	defer server.Close()

	client := NewOpenAIClient("test-api-key", server.URL+"/v1", "gpt-4o")
	result, err := client.AnalyzeImage(context.Background(), []byte("fake-image-data"), "describe this image")
	if err != nil {
		t.Fatalf("AnalyzeImage failed: %v", err)
	}

	if result.Tags == nil || len(result.Tags) != 3 {
		t.Errorf("expected 3 tags, got %v", result.Tags)
	}
	if result.Confidence != 0.95 {
		t.Errorf("expected confidence 0.95, got %f", result.Confidence)
	}
}

// --- Error Handling Tests ---

func TestLLMClient_InvalidJSON_OllamaProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		openAIResp := map[string]interface{}{
			"id":      "chatcmpl-ollama-123",
			"object":  "chat.completion",
			"created": 1234567890,
			"model":   "llama3.2-vision",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": `{broken json!!!`,
					},
					"finish_reason": "stop",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openAIResp)
	}))
	defer server.Close()

	client := NewOpenAIClient("ollama", server.URL+"/v1", "llama3.2-vision")
	_, err := client.AnalyzeImage(context.Background(), []byte("fake-image-data"), "describe this image")
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestLLMClient_InvalidJSON_OpenAI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{invalid json!!!`))
	}))
	defer server.Close()

	client := NewOpenAIClient("test-api-key", server.URL+"/v1", "gpt-4o")
	_, err := client.AnalyzeImage(context.Background(), []byte("fake-image-data"), "describe this image")
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestLLMClient_HTTPError_OllamaProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"message":"internal server error"}}`))
	}))
	defer server.Close()

	client := NewOpenAIClient("ollama", server.URL+"/v1", "llama3.2-vision")
	_, err := client.AnalyzeImage(context.Background(), []byte("fake-image-data"), "describe this image")
	if err == nil {
		t.Error("expected error for HTTP 500, got nil")
	}
}

func TestLLMClient_HTTPError_OpenAI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"message":"internal server error"}}`))
	}))
	defer server.Close()

	client := NewOpenAIClient("test-api-key", server.URL+"/v1", "gpt-4o")
	_, err := client.AnalyzeImage(context.Background(), []byte("fake-image-data"), "describe this image")
	if err == nil {
		t.Error("expected error for HTTP 500, got nil")
	}
}

// --- Factory Tests ---

func TestNewLLMClient_OllamaNoAPIKey(t *testing.T) {
	cfg := LLMClientConfig{
		BaseURL: "http://localhost:11434",
		Model:   "llama3.2-vision",
	}
	client := NewLLMClient(cfg)
	oc, ok := client.(*OpenAIClient)
	if !ok {
		t.Fatalf("expected *OpenAIClient, got %T", client)
	}
	if oc.client == nil {
		t.Fatal("OpenAIClient.inner client is nil")
	}
	if oc.model != "llama3.2-vision" {
		t.Errorf("model = %q, want llama3.2-vision", oc.model)
	}
}

func TestNewLLMClient_OpenAIWithAPIKey(t *testing.T) {
	cfg := LLMClientConfig{
		BaseURL: "https://api.openai.com",
		Model:   "gpt-4o",
		APIKey:  "sk-test",
	}
	client := NewLLMClient(cfg)
	oc, ok := client.(*OpenAIClient)
	if !ok {
		t.Fatalf("expected *OpenAIClient, got %T", client)
	}
	if oc.model != "gpt-4o" {
		t.Errorf("model = %q, want gpt-4o", oc.model)
	}
}

func TestNewLLMClient_AutoAppendsV1(t *testing.T) {
	server := newMockOpenAIServer(t)
	defer server.Close()

	cfg := LLMClientConfig{
		BaseURL: server.URL,
		Model:   "llama3.2-vision",
	}
	client := NewLLMClient(cfg)
	result, err := client.AnalyzeImage(context.Background(), []byte("fake"), "describe")
	if err != nil {
		t.Fatalf("AnalyzeImage failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func newMockOpenAIServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("expected path /v1/chat/completions, got %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":      "chatcmpl-123",
			"object":  "chat.completion",
			"model":   "llama3.2-vision",
			"created": 1234567890,
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": validAnalysisResultJSON(),
					},
					"finish_reason": "stop",
				},
			},
		})
	}))
}

// --- Context Cancellation Test ---

func TestLLMClient_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify context is cancelled — should not reach here if the request is properly cancelled
		t.Error("handler should not be called when context is cancelled")
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	client := NewOpenAIClient("ollama", server.URL+"/v1", "llama3.2-vision")
	_, err := client.AnalyzeImage(ctx, []byte("fake-image-data"), "describe this image")
	if err == nil {
		t.Error("expected error for cancelled context, got nil")
	}
}
