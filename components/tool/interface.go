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

// Package tool defines the Tool abstraction used across InferGlow to expose
// callable units of work to agents and LLMs.
//
// A Tool is a thin, transport-agnostic wrapper around a piece of functionality
// (a calculator, a web fetch, an Action from the action runtime, etc.). The
// package intentionally keeps the interface minimal so that adapters from
// other abstractions (e.g. action.Action) can be provided without pulling in
// heavyweight dependencies.
package tool

import "context"

// BaseTool 定义工具的基础接口。
type BaseTool interface {
	// Info 返回描述工具的元数据。
	Info(ctx context.Context) (*ToolInfo, error)
	// Invoke 使用给定的输入字符串执行工具，并返回结果。
	Invoke(ctx context.Context, input string) (string, error)
}

// ToolInfo 描述工具的元数据。
type ToolInfo struct { //nolint:revive
	Name        string
	Description string
	Params      *ParameterInfo // 参数 schema
	Tags        []string
	Metadata    map[string]any
}

// ParameterInfo 描述工具参数，遵循 JSON Schema 风格。
type ParameterInfo struct {
	Type       string         // "object", "array" 等
	Properties map[string]any // JSON Schema 属性
	Required   []string
}
