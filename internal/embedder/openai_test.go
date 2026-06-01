package embedder

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// embeddingDataObj represents one item in the "data" array of a mock embedding response.
type embeddingDataObj struct {
	Object    string    `json:"object"`
	Index     int       `json:"index"`
	Embedding []float32 `json:"embedding"`
}

// embeddingResp mirrors the OpenAI Embeddings API response format.
type embeddingResp struct {
	Object string             `json:"object"`
	Data   []embeddingDataObj `json:"data"`
	Model  string             `json:"model"`
	Usage  map[string]any     `json:"usage"`
}

// startEmbeddingServer creates and registers cleanup for a httptest server.
// The caller can construct a client baseURL with server.URL+"/v1".
func startEmbeddingServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

// float32SliceEqual compares float32 slices with a small epsilon tolerance.
func float32SliceEqual(a, b []float32) bool {
	if len(a) != len(b) {
		return false
	}
	const eps = 1e-5
	for i := range a {
		if math.Abs(float64(a[i]-b[i])) > eps {
			return false
		}
	}
	return true
}

// decodeEmbeddingReqRaw decodes the request body into a generic map so we can
// inspect presence/absence of optional fields (such as "dimensions").
func decodeEmbeddingReqRaw(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	var raw map[string]any
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	return raw
}

// validEmbeddingResp builds a valid embedding API JSON response body.
func validEmbeddingResp(items []embeddingDataObj, model string) embeddingResp {
	return embeddingResp{
		Object: "list",
		Data:   items,
		Model:  model,
		Usage:  map[string]any{"prompt_tokens": 10, "total_tokens": 10},
	}
}

// writeEmbeddingResp writes the given embedding response to w as JSON.
func writeEmbeddingResp(t *testing.T, w http.ResponseWriter, resp embeddingResp) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		t.Errorf("encode response: %v", err)
	}
}

func TestOpenAIEmbeddingClient_RequestFormat(t *testing.T) {
	var (
		gotMethod string
		gotPath   string
		gotAuth   string
		gotBody   map[string]any
		called    atomic.Int32
	)

	server := startEmbeddingServer(t, func(w http.ResponseWriter, r *http.Request) {
		called.Add(1)
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotBody = decodeEmbeddingReqRaw(t, r)

		writeEmbeddingResp(t, w, validEmbeddingResp(
			[]embeddingDataObj{
				{Object: "embedding", Index: 0, Embedding: []float32{0.1, 0.2, 0.3}},
			},
			"text-embedding-3-small",
		))
	})

	client := NewOpenAIEmbeddingClient(
		"test-api-key",
		server.URL+"/v1",
		"text-embedding-3-small",
		0,
	)

	got, err := client.Embed(context.Background(), []string{"hello world"})
	if err != nil {
		t.Fatalf("Embed() unexpected error: %v", err)
	}

	if called.Load() != 1 {
		t.Fatalf("expected exactly 1 server call, got %d", called.Load())
	}
	if gotMethod != http.MethodPost {
		t.Errorf("HTTP method: want POST, got %s", gotMethod)
	}
	if gotPath != "/v1/embeddings" {
		t.Errorf("URL path: want /v1/embeddings, got %s", gotPath)
	}
	if want := "Bearer test-api-key"; gotAuth != want {
		t.Errorf("Authorization header: want %q, got %q", want, gotAuth)
	}

	if m, _ := gotBody["model"].(string); m != "text-embedding-3-small" {
		t.Errorf("body.model: want %q, got %q", "text-embedding-3-small", m)
	}
	input, _ := gotBody["input"].([]any)
	if len(input) != 1 || input[0] != "hello world" {
		t.Errorf("body.input: want [\"hello world\"], got %v", input)
	}
	if _, ok := gotBody["dimensions"]; ok {
		t.Errorf("body.dimensions: should be absent when 0, got %v", gotBody["dimensions"])
	}

	want := [][]float32{{0.1, 0.2, 0.3}}
	if len(got) != 1 || !float32SliceEqual(got[0], want[0]) {
		t.Errorf("embedding result: want %v, got %v", want, got)
	}
}

func TestOpenAIEmbeddingClient_SingleText(t *testing.T) {
	embSingle := []float32{0.11, 0.22, 0.33, 0.44}

	server := startEmbeddingServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeEmbeddingResp(t, w, validEmbeddingResp(
			[]embeddingDataObj{
				{Object: "embedding", Index: 0, Embedding: embSingle},
			},
			"text-embedding-3-small",
		))
	})

	client := NewOpenAIEmbeddingClient("k", server.URL+"/v1", "text-embedding-3-small", 0)

	got, err := client.Embed(context.Background(), []string{"one text"})
	if err != nil {
		t.Fatalf("Embed() error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 embedding, got %d", len(got))
	}
	if !float32SliceEqual(got[0], embSingle) {
		t.Errorf("embedding[0]: want %v, got %v", embSingle, got[0])
	}
}

func TestOpenAIEmbeddingClient_BatchText(t *testing.T) {
	emb0 := []float32{1.0, 2.0, 3.0}
	emb1 := []float32{4.0, 5.0, 6.0}
	emb2 := []float32{7.0, 8.0, 9.0}

	server := startEmbeddingServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Intentionally return items out of index order to exercise index-based ordering.
		writeEmbeddingResp(t, w, validEmbeddingResp(
			[]embeddingDataObj{
				{Object: "embedding", Index: 2, Embedding: emb2},
				{Object: "embedding", Index: 0, Embedding: emb0},
				{Object: "embedding", Index: 1, Embedding: emb1},
			},
			"text-embedding-3-small",
		))
	})

	client := NewOpenAIEmbeddingClient("k", server.URL+"/v1", "text-embedding-3-small", 0)

	got, err := client.Embed(context.Background(), []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("Embed() error: %v", err)
	}

	want := [][]float32{emb0, emb1, emb2}
	if len(got) != len(want) {
		t.Fatalf("want %d embeddings, got %d", len(want), len(got))
	}
	for i, w := range want {
		if !float32SliceEqual(got[i], w) {
			t.Errorf("embedding[%d]: want %v, got %v", i, w, got[i])
		}
	}
}

func TestOpenAIEmbeddingClient_Dimensions(t *testing.T) {
	var body map[string]any

	server := startEmbeddingServer(t, func(w http.ResponseWriter, r *http.Request) {
		body = decodeEmbeddingReqRaw(t, r)
		writeEmbeddingResp(t, w, validEmbeddingResp(
			[]embeddingDataObj{
				{Object: "embedding", Index: 0, Embedding: []float32{0.1, 0.2}},
			},
			"text-embedding-3-small",
		))
	})

	const dims = 256
	client := NewOpenAIEmbeddingClient("k", server.URL+"/v1", "text-embedding-3-small", dims)

	_, err := client.Embed(context.Background(), []string{"with dims"})
	if err != nil {
		t.Fatalf("Embed() error: %v", err)
	}

	d, ok := body["dimensions"]
	if !ok {
		t.Fatalf("expected dimensions field in request body, got nil")
	}
	dimVal, ok := d.(float64)
	if !ok {
		t.Fatalf("body.dimensions: expected float64, got %T (%v)", d, d)
	}
	if int(dimVal) != dims {
		t.Errorf("dimensions: want %d, got %d", dims, int(dimVal))
	}
}

func TestOpenAIEmbeddingClient_NoDimensions(t *testing.T) {
	var body map[string]any

	server := startEmbeddingServer(t, func(w http.ResponseWriter, r *http.Request) {
		body = decodeEmbeddingReqRaw(t, r)
		writeEmbeddingResp(t, w, validEmbeddingResp(
			[]embeddingDataObj{
				{Object: "embedding", Index: 0, Embedding: []float32{0.1, 0.2}},
			},
			"text-embedding-3-small",
		))
	})

	client := NewOpenAIEmbeddingClient("k", server.URL+"/v1", "text-embedding-3-small", 0)

	_, err := client.Embed(context.Background(), []string{"no dims"})
	if err != nil {
		t.Fatalf("Embed() error: %v", err)
	}

	if _, ok := body["dimensions"]; ok {
		t.Errorf("dimensions field should be absent when 0, got %v", body["dimensions"])
	}
}

func TestOpenAIEmbeddingClient_InvalidJSON(t *testing.T) {
	server := startEmbeddingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{this is not valid json`))
	})

	client := NewOpenAIEmbeddingClient("k", server.URL+"/v1", "text-embedding-3-small", 0)

	_, err := client.Embed(context.Background(), []string{"bad response"})
	if err == nil {
		t.Fatal("expected error for invalid JSON response, got nil")
	}
	if !strings.Contains(err.Error(), "openai embed request") {
		t.Errorf("error should wrap with 'openai embed request', got: %v", err)
	}
}

func TestOpenAIEmbeddingClient_HTTPError(t *testing.T) {
	server := startEmbeddingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		errBody := map[string]any{
			"error": map[string]any{
				"message": "internal server error",
				"type":    "server_error",
				"code":    "500",
			},
		}
		if err := json.NewEncoder(w).Encode(errBody); err != nil {
			t.Errorf("encode error body: %v", err)
		}
	})

	client := NewOpenAIEmbeddingClient("k", server.URL+"/v1", "text-embedding-3-small", 0)

	_, err := client.Embed(context.Background(), []string{"failing request"})
	if err == nil {
		t.Fatal("expected error for HTTP 500 response, got nil")
	}
	if !strings.Contains(err.Error(), "openai embed request") {
		t.Errorf("error should wrap with 'openai embed request', got: %v", err)
	}
}

func TestOpenAIEmbeddingClient_MissingEmbedding(t *testing.T) {
	// Send 3 texts but only return embeddings for indices 0 and 2.
	// Index 1 is missing, which should trigger "missing embedding for text at index 1".
	server := startEmbeddingServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeEmbeddingResp(t, w, validEmbeddingResp(
			[]embeddingDataObj{
				{Object: "embedding", Index: 0, Embedding: []float32{0.1}},
				{Object: "embedding", Index: 2, Embedding: []float32{0.3}},
			},
			"text-embedding-3-small",
		))
	})

	client := NewOpenAIEmbeddingClient("k", server.URL+"/v1", "text-embedding-3-small", 0)

	_, err := client.Embed(context.Background(), []string{"a", "b", "c"})
	if err == nil {
		t.Fatal("expected error for missing embedding at index 1, got nil")
	}
	if !strings.Contains(err.Error(), "missing embedding for text at index 1") {
		t.Errorf("error should report missing index 1, got: %v", err)
	}
}
