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

// DefaultPrompt 是默认的图片分析提示词，当用户未提供自定义提示词时使用。
// 该提示词指示 LLM 以中文描述图片内容，并返回包含以下字段的结构化 JSON：
//
//	description  — 图片内容的详细中文描述
//	tags         — 分类检索标签
//	objects      — 检测到的具体物体
//	scene_type   — 场景类型枚举（indoor/outdoor/underwater/aerial/studio/night/unknown）
//	colors       — 主要颜色列表
//	people_count — 图片中的人数
//	has_text     — 是否包含可见文字
//	text         — 提取的文字内容
const DefaultPrompt = `你是一个专业的图片内容分析助手。请仔细观察这张图片，按照以下要求进行分析。

## 分析规则

1. 只描述图片中实际可见的内容，不要猜测或编造看不到的细节
2. 如果某个字段无法从图片中判断，使用合理的默认值

## 输出格式

返回一个 JSON 对象，严格包含以下字段：

description（字符串）：用中文详细描述图片内容，包括主体、环境、氛围、构图等，至少50字
tags（字符串数组）：5-10个相关标签，用于分类和检索，如["风景","天空","云","户外","自然"]
objects（字符串数组）：检测到的具体物体，如["云","太阳","树","长椅"]
scene_type（字符串）：场景类型，只能是以下枚举值之一：
  - "indoor"：室内场景
  - "outdoor"：室外场景
  - "underwater"：水下场景
  - "aerial"：航拍或无人机视角
  - "studio"：影棚或专业拍摄环境
  - "night"：夜景或低光环境
  - "unknown"：无法判断
colors（对象数组）：3-6个主要颜色，每个颜色对象包含：
  - name（字符串）：颜色的中文名称
  - hex（字符串）：CSS 十六进制颜色码，如 "#3B82F6"
people_count（整数）：图片中的人数，0表示无人
has_text（布尔值）：图片中是否有可见文字
text（字符串）：如果 has_text 为 true，提取图片中的文字内容；否则返回空字符串""

## 示例

{"description":"这是一张户外风景照片，画面中可以看到蓝天白云和远处的青山绿水，前景有一棵大树和一条蜿蜒的小路","tags":["风景","天空","云","户外","自然","山水","树木"],"objects":["云","山","树","小路","天空"],"scene_type":"outdoor","colors":[{"name":"蓝色","hex":"#3B82F6"},{"name":"白色","hex":"#F9FAFB"},{"name":"绿色","hex":"#22C55E"},{"name":"青色","hex":"#06B6D4"}],"people_count":0,"has_text":false,"text":""}

## 常用颜色 hex 对照表

请从下表中选取与图片颜色最匹配的颜色：

| 颜色 | hex |
|------|-----|
| 红色 | #EF4444 |
| 深红 | #DC2626 |
| 浅红 | #FCA5A5 |
| 橙色 | #F97316 |
| 黄色 | #EAB308 |
| 深黄 | #CA8A04 |
| 金黄色 | #F59E0B |
| 绿色 | #22C55E |
| 深绿 | #166534 |
| 浅绿 | #86EFAC |
| 碧绿 | #10B981 |
| 蓝色 | #3B82F6 |
| 深蓝 | #1D4ED8 |
| 浅蓝 | #93C5FD |
| 天蓝 | #0EA5E9 |
| 紫色 | #A855F7 |
| 深紫 | #7E22CE |
| 粉色 | #EC4899 |
| 深粉 | #DB2777 |
| 棕色 | #92400E |
| 深棕 | #78350F |
| 黑色 | #1F2937 |
| 白色 | #F9FAFB |
| 灰色 | #9CA3AF |
| 深灰 | #4B5563 |
| 浅灰 | #E5E7EB |
| 青色 | #06B6D4 |
| 金色 | #D97706 |
| 银色 | #D1D5DB |
| 米色 | #F5DEB3 |
| 卡其色 | #C3B091 |

## 注意事项

- 只返回 JSON，不要包含其他文字
- 确保 JSON 格式正确可解析
- description 必须至少50字
- tags 必须5-10个
- colors 必须3-6个，每个必须包含 name 和 hex
- scene_type 只能是上述枚举值之一
- hex 必须是有效的 CSS 十六进制颜色码（以 # 开头）`

// ImageAnalyzer 图片分析器，封装了 LLM 客户端、图片预处理、重试逻辑和结果校验。
// 负责将原始图片数据缩放、编码后发送给 LLM 进行分析，并对返回结果进行验证。
type ImageAnalyzer struct {
	client      LLMClient     // LLM 客户端，用于发送图片分析请求
	prompt      string        // 分析提示词，指导 LLM 如何描述图片内容
	maxRetries  int           // 最大重试次数
	timeout     time.Duration // 单次分析请求的超时时间
	maxImageDim int           // 图片最大尺寸（像素），超过此尺寸将被等比缩放（默认 1536）
}

// NewImageAnalyzer 创建一个新的 ImageAnalyzer 实例。
// 如果 prompt 为空，则使用 DefaultPrompt 作为默认分析提示词。
// maxImageDim 固定为 1536 像素。
func NewImageAnalyzer(client LLMClient, prompt string, maxRetries int, timeout time.Duration) *ImageAnalyzer {
	if prompt == "" {
		prompt = DefaultPrompt
	}
	return &ImageAnalyzer{
		client:      client,
		prompt:      prompt,
		maxRetries:  maxRetries,
		timeout:     timeout,
		maxImageDim: 1536,
	}
}

// Analyze 分析图片内容，包含完整的处理流程：
//  1. 应用超时上下文
//  2. 预处理图片（缩放到最大 2048px，重新编码为 JPEG 质量 85%）
//  3. 指数退避重试循环（1s, 2s, 4s...，最多 maxRetries 次）
//  4. 每次重试前检查上下文是否已取消
//  5. 调用 LLM 客户端进行分析
//  6. 校验返回结果中的必填字段
func (a *ImageAnalyzer) Analyze(ctx context.Context, imageData []byte, imageInfo string) (*types.AnalysisResult, error) {
	// 设置超时上下文
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()

	prompt := a.prompt
	if imageInfo != "" {
		prompt = a.prompt + "\n\n## 图片信息\n\n" + imageInfo
	}

	originalSize := len(imageData)

	// 预处理图片：缩放、重新编码
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
	// 指数退避重试循环
	for attempt := 0; attempt <= a.maxRetries; attempt++ {
		if attempt > 0 {
			// 指数退避：1s, 2s, 4s, 8s...
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

		result, err := a.client.AnalyzeImage(ctx, processed, prompt)
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
		// 检查是否为不可重试错误（4xx 等）
		if !isRetryable(err) {
			return nil, fmt.Errorf("non-retryable error: %w", err)
		}

		if ctx.Err() != nil {
			return nil, fmt.Errorf("context expired after attempt %d: %w", attempt, ctx.Err())
		}
	}

	return nil, fmt.Errorf("all %d retries exhausted: %w", a.maxRetries, lastErr)
}

// truncate 截断字符串，用于日志输出中控制长度。
// 如果字符串长度超过 maxLen，截断后追加 "..." 后缀。
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// preprocessImage 对原始图片数据进行预处理以减小传输体积。
// 处理流程：
//  1. 解码图片数据
//  2. 如果宽高均不超过 maxImageDim（1536px），保持原始尺寸，重新编码为 JPEG 质量 85%
//  3. 如果超过限制，使用 CatmullRom 插值算法等比缩放至 maxImageDim 以内
//  4. 将缩放后的图片编码为 JPEG 质量 85% 并返回
func (a *ImageAnalyzer) preprocessImage(data []byte) ([]byte, error) {
	// 步骤1：解码图片
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}

	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// 如果图片尺寸在限制以内且文件不超过 500KB，直接编码为 JPEG（质量 85%）
	const maxFileSize = 500 * 1024
	if width <= a.maxImageDim && height <= a.maxImageDim && len(data) <= maxFileSize {
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
			return nil, fmt.Errorf("encode image: %w", err)
		}
		return buf.Bytes(), nil
	}

	// 步骤2：计算等比缩放后的新尺寸
	var newWidth, newHeight int
	if width > height {
		newWidth = a.maxImageDim
		newHeight = int(float64(height) * float64(a.maxImageDim) / float64(width))
	} else {
		newHeight = a.maxImageDim
		newWidth = int(float64(width) * float64(a.maxImageDim) / float64(height))
	}

	// 步骤3：使用 CatmullRom 算法缩放图片
	dst := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Over, nil)

	// 步骤4：编码为 JPEG（质量 85%）
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 85}); err != nil {
		return nil, fmt.Errorf("encode scaled image: %w", err)
	}

	return buf.Bytes(), nil
}

// validateResult 校验 LLM 返回的分析结果是否包含所有必填字段。
// 必填字段：Description（非空）、Tags（非 nil）、Objects（非 nil）、Colors（非 nil）。
// 任一字段缺失则返回描述性错误。
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
	for i, c := range result.Colors {
		if c.Name == "" {
			return fmt.Errorf("colors[%d]: name is empty", i)
		}
		if c.Hex == "" {
			return fmt.Errorf("colors[%d]: hex is empty", i)
		}
		if !strings.HasPrefix(c.Hex, "#") {
			return fmt.Errorf("colors[%d]: hex must start with '#', got %q", i, c.Hex)
		}
	}
	return nil
}

// isRetryable 判断 LLM 客户端返回的错误是否可重试。
// 错误分类规则：
//   - 4xx HTTP 状态码 → 不可重试（客户端错误，如认证失败、参数错误）
//   - 5xx HTTP 状态码 → 可重试（服务端临时错误）
//   - net.Error（网络错误） → 可重试（连接超时、DNS 解析失败等）
//   - context.Canceled → 不可重试（用户主动取消）
//   - *url.Error → 解包后检查内部错误类型
//   - 其他未识别错误 → 默认可重试（保守策略）
func isRetryable(err error) bool {
	if err == nil {
		return false
	}

	// context.Canceled 不可重试（用户主动取消）
	if errors.Is(err, context.Canceled) {
		return false
	}

	errStr := err.Error()

	// 4xx 客户端错误不可重试
	if strings.Contains(errStr, "status 4") {
		return false
	}

	// 5xx 服务端错误可重试
	if strings.Contains(errStr, "status 5") {
		return true
	}

	// 解包错误链，检查是否为网络错误
	for unwrapped := err; unwrapped != nil; {
		switch e := unwrapped.(type) {
		case *url.Error:
			// URL 错误：解包后继续检查内部错误
			unwrapped = e.Err
			continue
		case net.Error:
			// 网络错误（连接超时、DNS 失败等）可重试
			return true
		}
		unwrapped = errors.Unwrap(unwrapped)
	}

	// 默认：未识别的错误也允许重试（保守策略）
	return true
}
