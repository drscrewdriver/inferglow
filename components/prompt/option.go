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

import "github.com/inferglow/model"

// Option 是配置 StringTemplate 的函数式选项。
type Option func(*StringTemplate)

// WithSystemMessage 便捷添加一条 system 消息模板。
func WithSystemMessage(templateStr string) Option {
	return func(t *StringTemplate) {
		if err := t.AddMessage(model.RoleSystem, templateStr); err != nil {
			t.parseErr = err
		}
	}
}

// WithUserMessage 便捷添加一条 user 消息模板。
func WithUserMessage(templateStr string) Option {
	return func(t *StringTemplate) {
		if err := t.AddMessage(model.RoleUser, templateStr); err != nil {
			t.parseErr = err
		}
	}
}

// WithAssistantMessage 便捷添加一条 assistant 消息模板。
func WithAssistantMessage(templateStr string) Option {
	return func(t *StringTemplate) {
		if err := t.AddMessage(model.RoleAssistant, templateStr); err != nil {
			t.parseErr = err
		}
	}
}
