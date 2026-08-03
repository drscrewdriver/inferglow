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

package flow

import (
	"fmt"
)

// OpAction 是声明式 Action 调用的算子类型。
// 在 TriggerFlow 编排中使用，通过 Context.ExecuteAction 调用已注册 Action。
const OpAction OperatorKind = "action"

// ActionOperatorHandler 实现 OperatorHandler 接口，
// 从 OperatorContext 中提取 Context 并调用指定 Action。
//
// Operator.Options 配置：
//   - "action_name" (string, 必需): 要调用的 Action 注册名
//   - "action_params" (map[string]any, 可选): 静态参数（与 Input 合并，Input 优先）
//
// 行为：
//  1. 从 oc.Ctx 提取 Context（通过 ContextFrom）
//  2. 若无 Context → 返回错误 "action operator requires Context"
//  3. 合并 Options["action_params"] 和 oc.Input（若 input 是 map[string]any）
//  4. 调用 fc.ExecuteAction(oc.Ctx, name, mergedParams)
//  5. 返回执行结果
type ActionOperatorHandler struct{}

// Kind 返回 OpAction。
func (h *ActionOperatorHandler) Kind() OperatorKind { return OpAction }

// Execute 执行 Action 调用。
func (h *ActionOperatorHandler) Execute(oc *OperatorContext) (any, error) {
	// 1. 提取 Context
	fc, ok := ContextFrom(oc.Ctx)
	if !ok || fc == nil {
		return nil, fmt.Errorf("action operator %q requires Context in context; "+
			"use flow.WithFlowContext to inject one", oc.Operator.ID)
	}

	// 2. 读取 action_name
	name, _ := oc.Operator.Options["action_name"].(string)
	if name == "" {
		return nil, fmt.Errorf("action operator %q: missing required option \"action_name\"", oc.Operator.ID)
	}

	// 3. 合并参数
	params := map[string]any{}
	if staticParams, ok := oc.Operator.Options["action_params"].(map[string]any); ok {
		for k, v := range staticParams {
			params[k] = v
		}
	}
	if inputMap, ok := oc.Input.(map[string]any); ok {
		for k, v := range inputMap {
			params[k] = v // Input 覆盖静态参数
		}
	}

	// 4. 调用 Action
	return fc.ExecuteAction(oc.Ctx, name, params)
}
