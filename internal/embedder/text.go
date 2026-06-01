package embedder

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/zwh8800/phosche/internal/types"
)

// DefaultSourceTemplate 是默认的 embedding 输入文本模板。
const DefaultSourceTemplate = `{{.Description}}
标签: {{join .Tags "、"}}
物体: {{join .Objects "、"}}
地点: {{.FormattedAddress}}
地址: {{.Address}}
文字: {{.Text}}`

// templateData 是模板渲染用的数据结构。
type templateData struct {
	Description      string
	Tags             []string
	Objects          []string
	FormattedAddress string
	Address          string
	Text             string
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

	if doc.Description != "" {
		b.WriteString(doc.Description)
		b.WriteByte('\n')
	}

	if len(doc.Tags) > 0 {
		b.WriteString("标签: ")
		b.WriteString(strings.Join(doc.Tags, "、"))
		b.WriteByte('\n')
	}

	if len(doc.Objects) > 0 {
		b.WriteString("物体: ")
		b.WriteString(strings.Join(doc.Objects, "、"))
		b.WriteByte('\n')
	}

	if doc.FormattedAddress != "" {
		b.WriteString("地点: ")
		b.WriteString(doc.FormattedAddress)
		b.WriteByte('\n')
	}

	if doc.Address != "" {
		b.WriteString("地址: ")
		b.WriteString(doc.Address)
		b.WriteByte('\n')
	}

	if doc.Text != "" {
		b.WriteString("文字: ")
		b.WriteString(doc.Text)
	}

	return strings.TrimSpace(b.String())
}
