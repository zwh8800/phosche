package embedder

import (
	"strings"
	"testing"

	"github.com/zwh8800/phosche/internal/types"
)

func TestBuildEmbeddingTextDefault(t *testing.T) {
	tests := []struct {
		name string
		doc  types.PhotoDocument
		want []string // substrings that MUST appear in output
		not  []string // substrings that MUST NOT appear
	}{
		{
			name: "with SceneType and all fields",
			doc: types.PhotoDocument{
				AnalysisResult: types.AnalysisResult{
					SceneType:   "outdoor",
					Description: "海滩日落的美丽景色",
					Tags:        []string{"海滩", "日落", "橙色"},
					Objects:     []string{"大海", "天空", "夕阳"},
					Text:        "中山路",
				},
				GeoInfo: types.GeoInfo{
					FormattedAddress: "海南省三亚市亚龙湾",
				},
			},
			want: []string{"这是一张outdoor场景的照片", "海滩日落", "海滩", "日落", "橙色", "大海", "天空", "夕阳", "三亚", "中山路"},
			not:  []string{"这是一张照片", "标签:", "物体:", "地点:", "地址:", "文字:"},
		},
		{
			name: "empty SceneType",
			doc: types.PhotoDocument{
				AnalysisResult: types.AnalysisResult{
					Description: "室内拍摄的静物",
					Tags:        []string{"静物"},
				},
			},
			want: []string{"这是一张照片", "室内拍摄", "静物"},
			not:  []string{"场景的"},
		},
		{
			name: "minimal document - only description",
			doc: types.PhotoDocument{
				AnalysisResult: types.AnalysisResult{
					Description: "一张照片",
				},
			},
			want: []string{"这是一张照片", "一张照片"},
			not:  []string{"标签", "物体", "场景", "地点", "文字"},
		},
		{
			name: "empty document",
			doc:  types.PhotoDocument{},
			want: []string{"这是一张照片"},
			not:  []string{"内容描述", "关键标签", "画面内容", "拍摄地点", "图中文字"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildEmbeddingTextDefault(tt.doc)
			for _, s := range tt.want {
				if !strings.Contains(got, s) {
					t.Errorf("BuildEmbeddingTextDefault() output missing %q\nGot: %s", s, got)
				}
			}
			for _, s := range tt.not {
				if strings.Contains(got, s) {
					t.Errorf("BuildEmbeddingTextDefault() output should NOT contain %q\nGot: %s", s, got)
				}
			}
		})
	}
}

func TestBuildEmbeddingText(t *testing.T) {
	doc := types.PhotoDocument{
		AnalysisResult: types.AnalysisResult{
			SceneType:   "indoor",
			Description: "咖啡馆内的温馨场景",
			Tags:        []string{"咖啡", "室内"},
			Objects:     []string{"桌子", "杯子"},
		},
	}

	t.Run("default template via empty string", func(t *testing.T) {
		got, err := BuildEmbeddingText(doc, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(got, "这是一张indoor场景的照片") {
			t.Errorf("missing SceneType in output: %s", got)
		}
	})

	t.Run("custom template with SceneType", func(t *testing.T) {
		customTmpl := `[{{.SceneType}}] {{.Description}}`
		got, err := BuildEmbeddingText(doc, customTmpl)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "[indoor] 咖啡馆内的温馨场景" {
			t.Errorf("got %q, want [indoor] 咖啡馆内的温馨场景", got)
		}
	})

	t.Run("invalid field in template returns error", func(t *testing.T) {
		_, err := BuildEmbeddingText(doc, "{{.BadField}}")
		if err == nil {
			t.Fatal("expected error for unknown field, got nil")
		}
	})
}

func TestBuildEmbeddingText_EmptySceneType(t *testing.T) {
	doc := types.PhotoDocument{
		AnalysisResult: types.AnalysisResult{
			Description: "无场景类型的照片",
			Tags:        []string{"测试"},
		},
	}

	got, err := BuildEmbeddingText(doc, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(got, "这是一张照片") {
		t.Errorf("expected '这是一张照片' when SceneType is empty, got: %s", got)
	}
	if strings.Contains(got, "场景的") {
		t.Errorf("should not contain '场景的' when SceneType is empty, got: %s", got)
	}
}
