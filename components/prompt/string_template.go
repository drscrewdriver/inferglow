// Copyright 2026 InferGlow Authors
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package prompt

import (
	"context"
	"fmt"
	"strings"
	"text/template"

	"github.com/inferglow/model"
)

// StringTemplate 是基于 Go text/template 的 ChatTemplate 实现。
// 每个模板生成一条消息，支持多消息组合（如 system + user）。
type StringTemplate struct {
	templates []*template.Template // 每个模板生成一条消息
	roles     []model.Role         // 对应每条消息的角色
	parseErr  error                // 从 Option 应用时捕获的解析错误
}

// NewStringTemplate 创建一个空的 StringTemplate，可应用选项。
func NewStringTemplate(opts ...Option) *StringTemplate {
	t := &StringTemplate{}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// AddMessage 添加一条消息模板。模板字符串使用 Go text/template 语法。
func (t *StringTemplate) AddMessage(role model.Role, templateStr string) error {
	tmpl, err := template.New("").Parse(templateStr)
	if err != nil {
		return err
	}
	t.templates = append(t.templates, tmpl)
	t.roles = append(t.roles, role)
	return nil
}

// Format 渲染所有模板，返回 ChatMessage 列表。
func (t *StringTemplate) Format(ctx context.Context, vars map[string]any) ([]model.ChatMessage, error) {
	if t.parseErr != nil {
		return nil, t.parseErr
	}
	messages := make([]model.ChatMessage, 0, len(t.templates))
	for i, tmpl := range t.templates {
		var buf strings.Builder
		if err := tmpl.Execute(&buf, vars); err != nil {
			return nil, fmt.Errorf("execute template %d: %w", i, err)
		}
		messages = append(messages, model.ChatMessage{
			Role:    string(t.roles[i]),
			Content: buf.String(),
		})
	}
	return messages, nil
}
