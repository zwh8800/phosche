package embedder

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/zwh8800/phosche/internal/types"
)

// DefaultSourceTemplate 是默认的 embedding 输入文本模板。
const DefaultSourceTemplate = `这是一张{{if .SceneType}}{{.SceneType}}场景的{{end}}照片。
内容描述：{{.Description}}
关键标签：{{join .Tags "、"}}
画面内容：{{join .Objects "、"}}
{{if .FormattedAddress}}拍摄地点：{{.FormattedAddress}}{{end}}
{{if .Text}}图中文字：{{.Text}}{{end}}`

// templateData 是模板渲染用的数据结构。
type templateData struct {
	Description      string
	Tags             []string
	Objects          []string
	FormattedAddress string
	Address          string
	Text             string
	SceneType        string
}

// BuildEmbeddingText 根据 PhotoDocument 构建 embedding 输入文本。
// tmplStr 为空时使用默认模板。
func BuildEmbeddingText(doc types.PhotoDocument, tmplStr string) (string, error) {
	if tmplStr == "" {
		return BuildEmbeddingTextDefault(doc), nil
	}

	funcMap := template.FuncMap{
		"join": strings.Join,
	}

	tmpl, err := template.New("embedding").Funcs(funcMap).Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("parse embedding template: %w", err)
	}

	data := templateData{
		Description:      doc.Description,
		Tags:             doc.Tags,
		Objects:          doc.Objects,
		FormattedAddress: doc.FormattedAddress,
		Address:          doc.Address,
		Text:             doc.Text,
		SceneType:        doc.SceneType,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute embedding template: %w", err)
	}

	return strings.TrimSpace(buf.String()), nil
}

// BuildEmbeddingTextDefault 使用默认模板构建 embedding 输入文本。
// 这是无模板解析的快速路径。
func BuildEmbeddingTextDefault(doc types.PhotoDocument) string {
	var b strings.Builder

	if doc.SceneType != "" {
		b.WriteString("这是一张")
		b.WriteString(doc.SceneType)
		b.WriteString("场景的照片。\n")
	} else {
		b.WriteString("这是一张照片。\n")
	}

	if doc.Description != "" {
		b.WriteString("内容描述：")
		b.WriteString(doc.Description)
		b.WriteByte('\n')
	}

	if len(doc.Tags) > 0 {
		b.WriteString("关键标签：")
		b.WriteString(strings.Join(doc.Tags, "、"))
		b.WriteByte('\n')
	}

	if len(doc.Objects) > 0 {
		b.WriteString("画面内容：")
		b.WriteString(strings.Join(doc.Objects, "、"))
		b.WriteByte('\n')
	}

	if doc.FormattedAddress != "" {
		b.WriteString("拍摄地点：")
		b.WriteString(doc.FormattedAddress)
		b.WriteByte('\n')
	}

	if doc.Text != "" {
		b.WriteString("图中文字：")
		b.WriteString(doc.Text)
	}

	return strings.TrimSpace(b.String())
}
