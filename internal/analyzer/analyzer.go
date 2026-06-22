package analyzer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"log/slog"
	"net"
	"net/url"
	"strings"
	"time"

	openai "github.com/sashabaranov/go-openai"

	"github.com/zwh8800/phosche/internal/util"
	"github.com/zwh8800/phosche/internal/types"
	"golang.org/x/image/draw"
)

// DefaultSystemPrompt 是默认的系统提示词，定义了 LLM 的角色、分析规则、输出格式和约束条件。
// 当用户未提供自定义提示词时使用。系统提示词与用户提示词分离，以提升多模态 LLM 的指令遵循能力。
//
// 包含以下部分：
//   - 角色定义：专业的图片内容分析助手
//   - 分析规则：只描述可见内容、合理默认值
//   - 输出格式：结构化 JSON schema（description/tags/objects/scene_type/colors/people_count/has_text/text）
//   - 示例：户外风景照片示例
//   - 颜色对照表：28 个常用颜色的中文名称与 hex 对照
//   - 注意事项：JSON 格式约束、字段数量要求等
const DefaultSystemPrompt = `你是一个专业的图片内容分析助手。

## 分析规则

1. 只描述图片中实际可见的内容，不要猜测或编造看不到的细节
2. 如果某个字段无法从图片中判断，使用合理的默认值

## 输出格式

返回一个 JSON 对象，严格包含以下字段：

description（字符串）：用中文详细描述图片内容，至少80字。请按以下维度组织描述：
  - 主体：画面中最突出的人、物或场景焦点
  - 环境：背景、周围场景、空间关系
  - 构图：视角、拍摄角度、画面布局
  - 细节：光影、纹理、表情、动作等值得注意的细节
  - 氛围：整体感受、情绪基调（如温馨、宁静、热闹）
  不要机械罗列各维度，应自然融合成流畅的描述段落
tags（字符串数组）：5-10个抽象分类标签，用于场景归类和检索。标签应是场景、活动、氛围等抽象概念，而非具体物体。如["风景","户外","自然","宁静","旅行","山水"]
objects（字符串数组）：3-8个画面中可见的具体物体名称。只列出肉眼可辨认的实体物品，不包含抽象概念。如["山","树","小路","天空","云"]
注意：tags 和 objects 不应重叠。tags 描述"这是什么类型的场景"，objects 描述"画面中有什么东西"
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

### 室内场景
{"description":"一张温馨的室内照片，画面中央是一张木质餐桌上摆放着精致的菜肴和餐具，桌上还有一杯红酒和几支蜡烛，背景是暖色调的厨房，墙上挂着几幅装饰画，整体氛围温馨浪漫","tags":["美食","室内","餐桌","浪漫","晚餐","蜡烛"],"objects":["餐桌","盘子","酒杯","蜡烛","装饰画"],"scene_type":"indoor","colors":[{"name":"棕色","hex":"#92400E"},{"name":"橙色","hex":"#F97316"},{"name":"白色","hex":"#F9FAFB"}],"people_count":0,"has_text":false,"text":""}

### 户外风景
{"description":"这是一张户外风景照片，画面中可以看到蓝天白云和远处的青山绿水，前景有一棵大树和一条蜿蜒的小路，阳光从侧面照射，树叶投下斑驳的光影，整体氛围宁静自然","tags":["风景","户外","自然","宁静","山水","旅行"],"objects":["山","树","小路","天空","云"],"scene_type":"outdoor","colors":[{"name":"蓝色","hex":"#3B82F6"},{"name":"白色","hex":"#F9FAFB"},{"name":"绿色","hex":"#22C55E"},{"name":"青色","hex":"#06B6D4"}],"people_count":0,"has_text":false,"text":""}

### 人像场景
{"description":"一张户外人像照片，画面中一位年轻女性站在樱花树下，微风吹动她的长发，她穿着白色连衣裙面带微笑，背景是粉色的樱花和蓝天，阳光透过花瓣洒下斑驳光影，氛围清新浪漫","tags":["人像","樱花","春天","微笑","户外","女性"],"objects":["人","樱花树","花瓣"],"scene_type":"outdoor","colors":[{"name":"粉色","hex":"#EC4899"},{"name":"白色","hex":"#F9FAFB"},{"name":"蓝色","hex":"#3B82F6"}],"people_count":1,"has_text":false,"text":""}

## 常用颜色 hex 对照表

可参考下表来选取与图片颜色最匹配的颜色：

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

- 直接输出 JSON 对象本身，绝对不要用 markdown 代码块包裹（不要输出三个反引号 json 或三个反引号）
- 不要添加任何解释、前言、后记或 markdown 格式
- 回复必须以 { 开头，以 } 结尾
- 确保 JSON 格式正确可解析
- description 必须至少80字
- tags 必须5-10个
- objects 必须3-8个
- colors 必须3-6个，每个必须包含 name 和 hex
- scene_type 只能是上述枚举值之一
- hex 必须是有效的 CSS 十六进制颜色码（以 # 开头）`

// DefaultUserPrompt 是默认的用户提示词，指示 LLM 按照系统提示词中的要求分析图片。
const DefaultUserPrompt = `请仔细观察这张图片，按照系统提示中的要求进行分析。`

// ImageAnalyzer 图片分析器，封装了 LLM 客户端、图片预处理、重试逻辑和结果校验。
// 负责将原始图片数据缩放、编码后发送给 LLM 进行分析，并对返回结果进行验证。
type ImageAnalyzer struct {
	client       LLMClient     // LLM 客户端，用于发送图片分析请求
	systemPrompt string        // system message with role, rules, format
	userPrompt   string        // user message with analysis instruction
	maxRetries   int           // 最大重试次数
	timeout      time.Duration // 单次分析请求的超时时间
	maxImageDim  int           // 图片最大尺寸（像素），超过此尺寸将被等比缩放（默认 1024）
}

// NewImageAnalyzer 创建一个新的 ImageAnalyzer 实例。
// 如果 prompt 为空，则使用 DefaultPrompt 作为默认分析提示词。
// maxImageDim 固定为 1024 像素。
func NewImageAnalyzer(client LLMClient, prompt string, maxRetries int, timeout time.Duration) *ImageAnalyzer {
	systemPrompt := DefaultSystemPrompt
	userPrompt := DefaultUserPrompt
	if prompt != "" {
		userPrompt = prompt
	}
	return &ImageAnalyzer{
		client:       client,
		systemPrompt: systemPrompt,
		userPrompt:   userPrompt,
		maxRetries:   maxRetries,
		timeout:      timeout,
		maxImageDim:  1024,
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

	prompt := a.userPrompt
	if imageInfo != "" {
		prompt = a.userPrompt + "\n\n## 图片元数据\n\n" + imageInfo + "\n\n请结合以上元数据信息辅助分析，但以视觉内容为主。元数据可帮助确认拍摄场景（如夜景、水下）、地点（如海边、山区）和构图意图（如长曝光、微距），但不要编造元数据中未体现的视觉内容。"
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

		result, err := a.client.AnalyzeImage(ctx, processed, a.systemPrompt, prompt)
		if err == nil {
			if err := a.validateResult(result); err != nil {
				return nil, fmt.Errorf("invalid analysis result: %w", err)
			}
			slog.Info("LLM analysis completed",
				"duration", time.Since(startTime).Round(time.Millisecond),
				"attempts", attempt+1,
				"description", util.Truncate(result.Description, 80),
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

// preprocessImage 对原始图片数据进行预处理以减小传输体积。
// 处理流程：
//  1. 解码图片数据
//  2. 如果宽高均不超过 maxImageDim（1024px），保持原始尺寸，重新编码为 JPEG 质量 85%
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

	// 如果图片尺寸在限制以内，直接编码为 JPEG（质量 85%）
	if width <= a.maxImageDim && height <= a.maxImageDim {
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
//   - *openai.APIError：429 → 可重试（限流），4xx → 不可重试，5xx → 可重试
//   - 4xx HTTP 状态码（字符串匹配兜底） → 不可重试（客户端错误，如认证失败、参数错误）
//   - 5xx HTTP 状态码（字符串匹配兜底） → 可重试（服务端临时错误）
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

	// go-openai SDK typed error: prefer structured HTTP status code over string matching
	var apiErr *openai.APIError
	if errors.As(err, &apiErr) {
		if apiErr.HTTPStatusCode == 429 {
			return true
		}
		if apiErr.HTTPStatusCode >= 400 && apiErr.HTTPStatusCode < 500 {
			return false
		}
		if apiErr.HTTPStatusCode >= 500 {
			return true
		}
	}

	errStr := err.Error()

	// 4xx 客户端错误不可重试（兼容旧格式 "status 400" 和 SDK 格式 "status: 400"）
	if strings.Contains(errStr, "status 4") || strings.Contains(errStr, "status: 4") {
		return false
	}

	// 5xx 服务端错误可重试（兼容旧格式 "status 500" 和 SDK 格式 "status: 500"）
	if strings.Contains(errStr, "status 5") || strings.Contains(errStr, "status: 5") {
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
