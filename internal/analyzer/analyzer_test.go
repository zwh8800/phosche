package analyzer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"testing"
	"time"

	"github.com/zwh8800/phosche/internal/types"
)

func makeTestJPEG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		panic(fmt.Sprintf("makeTestJPEG: %v", err))
	}
	return buf.Bytes()
}

func validResult() *types.AnalysisResult {
	return &types.AnalysisResult{
		Description: "A cat sitting on a windowsill",
		Tags:        []string{"cat", "windowsill", "indoor"},
		Objects:     []string{"cat", "window"},
		SceneType:   "indoor",
		Colors:      []types.ColorInfo{{Name: "白色", Hex: "#F9FAFB"}, {Name: "棕色", Hex: "#92400E"}},
		PeopleCount: 0,
		HasText:     false,
		Confidence:  0.95,
	}
}

type mockCall struct {
	result *types.AnalysisResult
	err    error
}

type mockLLMClient struct {
	calls   []mockCall
	callIdx int
}

func (m *mockLLMClient) AnalyzeImage(_ context.Context, _ []byte, _ string) (*types.AnalysisResult, error) {
	if m.callIdx >= len(m.calls) {
		return nil, fmt.Errorf("mock: unexpected call (callIdx=%d, total=%d)", m.callIdx, len(m.calls))
	}
	c := m.calls[m.callIdx]
	m.callIdx++
	return c.result, c.err
}

type capturingMockClient struct {
	receivedData []byte
	result       *types.AnalysisResult
}

func (m *capturingMockClient) AnalyzeImage(_ context.Context, imageData []byte, _ string) (*types.AnalysisResult, error) {
	m.receivedData = append([]byte(nil), imageData...)
	return m.result, nil
}

type blockingMockClient struct{}

func (m *blockingMockClient) AnalyzeImage(ctx context.Context, _ []byte, _ string) (*types.AnalysisResult, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestAnalyzer_ValidResponse(t *testing.T) {
	mock := &mockLLMClient{
		calls: []mockCall{
			{result: validResult(), err: nil},
		},
	}
	analyzer := NewImageAnalyzer(mock, "", 2, 30*time.Second)

	result, err := analyzer.Analyze(context.Background(), makeTestJPEG(), "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Description != validResult().Description {
		t.Errorf("description = %q, want %q", result.Description, validResult().Description)
	}
	if result.SceneType != validResult().SceneType {
		t.Errorf("scene_type = %q, want %q", result.SceneType, validResult().SceneType)
	}
}

func TestAnalyzer_RetrySuccess(t *testing.T) {
	mock := &mockLLMClient{
		calls: []mockCall{
		{err: errors.New("error, status code: 500, status: 500 Internal Server Error, message: internal error")},
		{err: errors.New("error, status code: 502, status: 502 Bad Gateway, message: bad gateway")},
			{result: validResult(), err: nil},
		},
	}
	analyzer := NewImageAnalyzer(mock, "", 3, 30*time.Second)

	result, err := analyzer.Analyze(context.Background(), makeTestJPEG(), "")
	if err != nil {
		t.Fatalf("expected no error after retry, got %v", err)
	}
	if result.Description != validResult().Description {
		t.Errorf("description = %q, want %q", result.Description, validResult().Description)
	}
	if mock.callIdx != 3 {
		t.Errorf("call count = %d, want 3", mock.callIdx)
	}
}

func TestAnalyzer_RetryExhausted(t *testing.T) {
	mock := &mockLLMClient{
		calls: []mockCall{
			{err: errors.New("error, status code: 500, status: 500 Internal Server Error, message: internal error")},
			{err: errors.New("error, status code: 500, status: 500 Internal Server Error, message: internal error")},
			{err: errors.New("error, status code: 500, status: 500 Internal Server Error, message: internal error")},
		},
	}
	analyzer := NewImageAnalyzer(mock, "", 2, 30*time.Second)

	_, err := analyzer.Analyze(context.Background(), makeTestJPEG(), "")
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if mock.callIdx != 3 {
		t.Errorf("call count = %d, want 3", mock.callIdx)
	}
}

func TestAnalyzer_MissingField(t *testing.T) {
	mock := &mockLLMClient{
		calls: []mockCall{
			{result: &types.AnalysisResult{}, err: nil},
		},
	}
	analyzer := NewImageAnalyzer(mock, "", 2, 30*time.Second)

	_, err := analyzer.Analyze(context.Background(), makeTestJPEG(), "")
	if err == nil {
		t.Fatal("expected validation error for missing description")
	}
}

func TestAnalyzer_NonRetryableError(t *testing.T) {
	mock := &mockLLMClient{
		calls: []mockCall{
			{err: errors.New("error, status code: 400, status: 400 Bad Request, message: bad request")},
		},
	}
	analyzer := NewImageAnalyzer(mock, "", 2, 30*time.Second)

	_, err := analyzer.Analyze(context.Background(), makeTestJPEG(), "")
	if err == nil {
		t.Fatal("expected error for 4xx")
	}
	if mock.callIdx != 1 {
		t.Errorf("call count = %d, want 1 (no retry)", mock.callIdx)
	}
}

func TestAnalyzer_Timeout(t *testing.T) {
	blocking := &blockingMockClient{}
	analyzer := NewImageAnalyzer(blocking, "", 2, 100*time.Millisecond)

	_, err := analyzer.Analyze(context.Background(), makeTestJPEG(), "")
	if err == nil {
		t.Fatal("expected context deadline error")
	}
}

func TestAnalyzer_ImageResize(t *testing.T) {
	largeImg := image.NewRGBA(image.Rect(0, 0, 1000, 1000))
	var originalBuf bytes.Buffer
	if err := jpeg.Encode(&originalBuf, largeImg, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("failed to encode test image: %v", err)
	}
	originalData := originalBuf.Bytes()

	capturing := &capturingMockClient{
		result: validResult(),
	}

	analyzer := &ImageAnalyzer{
		client:      capturing,
		prompt:      DefaultPrompt,
		maxRetries:  2,
		timeout:     30 * time.Second,
		maxImageDim: 200,
	}

	_, err := analyzer.Analyze(context.Background(), originalData, "")
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	decoded, _, err := image.Decode(bytes.NewReader(capturing.receivedData))
	if err != nil {
		t.Fatalf("failed to decode received image: %v", err)
	}

	bounds := decoded.Bounds()
	if bounds.Dx() > analyzer.maxImageDim || bounds.Dy() > analyzer.maxImageDim {
		t.Errorf("received image dimensions %dx%d exceed max %d", bounds.Dx(), bounds.Dy(), analyzer.maxImageDim)
	}

	if len(capturing.receivedData) >= len(originalData) {
		t.Errorf("received data size %d not smaller than original %d", len(capturing.receivedData), len(originalData))
	}
}
