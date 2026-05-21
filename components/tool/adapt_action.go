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

package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/inferglow/action"
)

// actionTool 将 action.Action 适配为 BaseTool 接口。
type actionTool struct {
	act *action.Action
}

// ActionToTool 从 action.Action 创建一个 BaseTool。
//
// Info 从 Action 的 Name/Description/Schema 字段生成 ToolInfo；
// Invoke 将输入字符串解析为 map[string]any 后委托给 Action 的 Executor。
func ActionToTool(act *action.Action) BaseTool {
	return &actionTool{act: act}
}

// Info 返回从 Action 派生的工具元数据。
func (t *actionTool) Info(ctx context.Context) (*ToolInfo, error) {
	return &ToolInfo{
		Name:        t.act.Name,
		Description: t.act.Description,
		Params:      schemaToParams(t.act.Schema),
		Tags:        t.act.Tags,
	}, nil
}

// Invoke 将输入字符串解析为 map[string]any，然后委托给 Action 的 Executor。
// 若输入不是合法的 JSON 对象，则将原始字符串包装在 "input" 键下。
func (t *actionTool) Invoke(ctx context.Context, input string) (string, error) {
	var args map[string]any
	if err := json.Unmarshal([]byte(input), &args); err != nil {
		args = map[string]any{"input": input}
	}
	result, err := t.act.Executor.Execute(ctx, args)
	if err != nil {
		return "", fmt.Errorf("tool %q execution failed: %w", t.act.Name, err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Sprintf("%v", result), nil
	}
	return string(data), nil
}

// schemaToParams 将 action 的 JSON Schema map 转换为 ParameterInfo。
func schemaToParams(schema map[string]any) *ParameterInfo {
	if schema == nil {
		return nil
	}
	params := &ParameterInfo{}
	if v, ok := schema["type"].(string); ok {
		params.Type = v
	}
	if v, ok := schema["properties"].(map[string]any); ok {
		params.Properties = v
	}
	switch v := schema["required"].(type) {
	case []string:
		params.Required = v
	case []any:
		reqs := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				reqs = append(reqs, s)
			}
		}
		if len(reqs) > 0 {
			params.Required = reqs
		}
	}
	return params
}
