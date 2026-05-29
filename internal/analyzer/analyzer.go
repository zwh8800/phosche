package analyzer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/gif"
	_ "image/png"
	"log/slog"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/zwh8800/phosche/internal/types"
	"golang.org/x/image/draw"
)

// DefaultPrompt is used when no custom prompt is provided to NewImageAnalyzer.
const DefaultPrompt = `请仔细观察这张图片，然后返回一个JSON对象，包含以下字段：

description（用中文详细描述图片内容，至少50字，包括主体、环境、氛围、构图等）
tags（相关的标签数组，5-10个，如["风景","天空","云","户外","自然"]）
objects（检测到的具体物体数组，如["云","太阳","树","长椅"]）
scene_type（场景类型，只能是以下之一：indoor-室内、outdoor-室外、underwater-水下、aerial-航拍或无人机视角、studio-影棚或专业拍摄环境、night-夜景或低光环境、unknown-无法判断）
colors（主要颜色数组，3-6个，如["蓝色","白色","黄色"]）
people_count（图片中的人数，整数，0表示无人）
has_text（图片中是否有可见文字，布尔值）
text（如果has_text为true则提取图片中的文字内容，否则返回空字符串）

示例格式：
{"description":"这是一张户外风景照片，画面中可以看到蓝天白云和远处的青山绿水","tags":["风景","天空","云","户外","自然","山水"],"objects":["云","山","树","湖泊"],"scene_type":"outdoor","colors":["蓝色","白色","绿色","青色"],"people_count":0,"has_text":false,"text":""}

只返回JSON，不要其他文字。`

// ImageAnalyzer wraps an LLMClient with prompt building, retry logic, image
// preprocessing, and response validation.
type ImageAnalyzer struct {
	client      LLMClient
	prompt      string
	maxRetries  int
	timeout     time.Duration
	maxImageDim int
}

// NewImageAnalyzer creates an ImageAnalyzer with the given dependencies.
// If prompt is empty, DefaultPrompt is used.
func NewImageAnalyzer(client LLMClient, prompt string, maxRetries int, timeout time.Duration) *ImageAnalyzer {
	if prompt == "" {
		prompt = DefaultPrompt
	}
	return &ImageAnalyzer{
		client:      client,
		prompt:      prompt,
		maxRetries:  maxRetries,
		timeout:     timeout,
		maxImageDim: 2048,
	}
}

// Analyze preprocesses the image, calls the LLM client with retry logic, and
// validates the result.
func (a *ImageAnalyzer) Analyze(ctx context.Context, imageData []byte) (*types.AnalysisResult, error) {
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()

	originalSize := len(imageData)

	processed, err := a.preprocessImage(imageData)
	if err != nil {
		return nil, fmt.Errorf("preprocess image: %w", err)
	}

	scaled := len(processed) != originalSize
	slog.Info("starting LLM analysis",
		"original_bytes", originalSize,
		"processed_bytes", len(processed),
		"scaled", scaled,
		"timeout", a.timeout,
	)

	startTime := time.Now()

	var lastErr error
	for attempt := 0; attempt <= a.maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<(attempt-1)) * time.Second
			slog.Warn("retrying LLM analysis", "attempt", attempt, "backoff", backoff, "error", lastErr)

			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("context cancelled during retry: %w", ctx.Err())
			case <-time.After(backoff):
			}
		}

		if ctx.Err() != nil {
			return nil, fmt.Errorf("context expired before attempt %d: %w", attempt, ctx.Err())
		}

		result, err := a.client.AnalyzeImage(ctx, processed, a.prompt)
		if err == nil {
			if err := a.validateResult(result); err != nil {
				return nil, fmt.Errorf("invalid analysis result: %w", err)
			}
			slog.Info("LLM analysis completed",
				"duration", time.Since(startTime).Round(time.Millisecond),
				"attempts", attempt+1,
				"description", truncate(result.Description, 80),
				"tags_count", len(result.Tags),
				"scene_type", result.SceneType,
				"confidence", result.Confidence,
			)
			return result, nil
		}

		lastErr = err
		if !isRetryable(err) {
			return nil, fmt.Errorf("non-retryable error: %w", err)
		}

		if ctx.Err() != nil {
			return nil, fmt.Errorf("context expired after attempt %d: %w", attempt, ctx.Err())
		}
	}

	return nil, fmt.Errorf("all %d retries exhausted: %w", a.maxRetries, lastErr)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func (a *ImageAnalyzer) preprocessImage(data []byte) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}

	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	if width <= a.maxImageDim && height <= a.maxImageDim {
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
			return nil, fmt.Errorf("encode image: %w", err)
		}
		return buf.Bytes(), nil
	}

	var newWidth, newHeight int
	if width > height {
		newWidth = a.maxImageDim
		newHeight = int(float64(height) * float64(a.maxImageDim) / float64(width))
	} else {
		newHeight = a.maxImageDim
		newWidth = int(float64(width) * float64(a.maxImageDim) / float64(height))
	}

	dst := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Over, nil)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 85}); err != nil {
		return nil, fmt.Errorf("encode scaled image: %w", err)
	}

	return buf.Bytes(), nil
}

func (a *ImageAnalyzer) validateResult(result *types.AnalysisResult) error {
	if result == nil {
		return errors.New("analysis result is nil")
	}
	if result.Description == "" {
		return errors.New("analysis result missing description")
	}
	if result.Tags == nil {
		return errors.New("analysis result tags is nil")
	}
	if result.Objects == nil {
		return errors.New("analysis result objects is nil")
	}
	if result.Colors == nil {
		return errors.New("analysis result colors is nil")
	}
	return nil
}

// isRetryable determines whether an error from the LLM client should be
// retried. Network errors and 5xx server errors are retryable. Client
// errors (4xx) and explicit cancellation are not.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, context.Canceled) {
		return false
	}

	errStr := err.Error()

	if strings.Contains(errStr, "status 4") {
		return false
	}

	if strings.Contains(errStr, "status 5") {
		return true
	}

	for unwrapped := err; unwrapped != nil; {
		switch e := unwrapped.(type) {
		case *url.Error:
			unwrapped = e.Err
			continue
		case net.Error:
			return true
		}
		unwrapped = errors.Unwrap(unwrapped)
	}

	return true
}
