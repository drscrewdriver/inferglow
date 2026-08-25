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

package agent

import (
	"context"

	"github.com/inferglow/action"
	"github.com/inferglow/model"
)

// ToolCallDecision 是 PreToolCall 钩子返回的干预决策。
// 零值（或返回 nil）表示放行，行为与未安装钩子时完全一致（向后兼容硬约束）。
type ToolCallDecision struct {
	// Block 为 true 时阻断该次工具调用：调用不会进入执行器，
	// 而是产出与 approval 拦截同形的 blocked 结果（模型可读）。
	Block bool
	// BlockReason 是阻断原因，写入 blocked 结果的 Error 字段，
	// 随工具结果进入下一轮模型输入。Block 为 true 且原因为空时
	// 使用默认文案。
	BlockReason string
	// RewriteParams 非空时替换该次调用的原始参数，执行器只会看到
	// 改写后的参数（原始参数会记入审计，若审计钩子启用）。
	RewriteParams map[string]any
	// AppendContext 非空时作为附加上下文拼接到该次调用的工具结果
	// 内容，随结果进入下一轮模型输入。
	AppendContext string
}

// ToolCallFeedback 是 PostToolCall 钩子返回的反馈。
// 零值（或返回 nil）表示不干预。
type ToolCallFeedback struct {
	// AppendContext 非空时作为附加上下文拼接到该次调用的工具结果
	// 内容，随结果进入下一轮模型输入。
	AppendContext string
}

// AgentCallbacks provides lifecycle hooks for observing Agent execution.
// All fields are optional; nil fields are silently skipped (zero overhead).
// Use WithCallbacks to install callbacks on an Agent.Run call.
type AgentCallbacks struct {
	// OnRunStart is called at the beginning of Agent.Run, before executeLoop.
	OnRunStart func(ctx context.Context, userMessage string)
	// OnRunEnd is called when Agent.Run completes (success or error).
	OnRunEnd func(ctx context.Context, response string, err error)
	// OnLLMCallStart is called before each LLM invocation in executeLoop.
	// round is the current iteration number (0-based).
	OnLLMCallStart func(ctx context.Context, round int)
	// OnLLMCallEnd is called after each LLM invocation completes.
	// tokens is the approximate token count of the response; usage is the
	// provider-reported token usage (nil when the provider returns none).
	OnLLMCallEnd func(ctx context.Context, round int, tokens int, usage *model.UsageInfo)
	// OnToolCallStart is called before a tool/action is executed.
	OnToolCallStart func(ctx context.Context, toolName string)
	// OnToolCallEnd is called after a tool/action completes.
	OnToolCallEnd func(ctx context.Context, toolName string, err error)
	// OnToken is called for each text delta chunk from the LLM stream.
	// Enables real-time token-by-token display in CLI/TUI frontends.
	OnToken func(ctx context.Context, delta string)
	// OnReasoning is called for each reasoning delta chunk (DeepSeek/MiMo).
	// Enables real-time reasoning display in CLI/TUI frontends.
	OnReasoning func(ctx context.Context, delta string)
	// OnApprovalRequired is called when a tool is blocked by approval policy.
	OnApprovalRequired func(ctx context.Context, toolName, recordID string)
	// OnCompression is called when context compression occurs.
	OnCompression func(ctx context.Context, stepsCompressed int)
	// PreToolCall 在每个工具调用派发给执行器前调用，可返回干预决策：
	// 阻断（Block）、改写参数（RewriteParams）、附加上下文（AppendContext）。
	// 返回 nil 或零值决策 = 放行，行为与未安装钩子时完全一致。
	PreToolCall func(ctx context.Context, toolName string, params map[string]any) *ToolCallDecision
	// PostToolCall 在每个工具调用完成后（结果进入 session 前）调用，
	// 可返回附加上下文（AppendContext）拼接进工具结果，供下一轮
	// LLM 调用读取。返回 nil 或零值反馈 = 不干预。
	PostToolCall func(ctx context.Context, toolName string, result *action.ActionResult) *ToolCallFeedback
}

// WithCallbacks installs lifecycle callbacks for this Run call.
// Pass nil to explicitly disable callbacks. Each callback field is
// independently optional; nil fields are skipped.
func WithCallbacks(cb *AgentCallbacks) RunOption {
	return func(c *runConfig) {
		c.callbacks = cb
	}
}

// fireOnRunStart invokes OnRunStart if non-nil. Safe to call with nil callbacks.
func fireOnRunStart(cb *AgentCallbacks, ctx context.Context, userMessage string) {
	if cb != nil && cb.OnRunStart != nil {
		cb.OnRunStart(ctx, userMessage)
	}
}

// fireOnRunEnd invokes OnRunEnd if non-nil. Safe to call with nil callbacks.
func fireOnRunEnd(cb *AgentCallbacks, ctx context.Context, response string, err error) {
	if cb != nil && cb.OnRunEnd != nil {
		cb.OnRunEnd(ctx, response, err)
	}
}

// fireOnLLMCallStart invokes OnLLMCallStart if non-nil.
func fireOnLLMCallStart(cb *AgentCallbacks, ctx context.Context, round int) {
	if cb != nil && cb.OnLLMCallStart != nil {
		cb.OnLLMCallStart(ctx, round)
	}
}

// fireOnLLMCallEnd invokes OnLLMCallEnd if non-nil.
func fireOnLLMCallEnd(cb *AgentCallbacks, ctx context.Context, round int, tokens int, usage *model.UsageInfo) {
	if cb != nil && cb.OnLLMCallEnd != nil {
		cb.OnLLMCallEnd(ctx, round, tokens, usage)
	}
}

// fireOnToolCallStart invokes OnToolCallStart if non-nil.
func fireOnToolCallStart(cb *AgentCallbacks, ctx context.Context, toolName string) {
	if cb != nil && cb.OnToolCallStart != nil {
		cb.OnToolCallStart(ctx, toolName)
	}
}

// fireOnToolCallEnd invokes OnToolCallEnd if non-nil.
func fireOnToolCallEnd(cb *AgentCallbacks, ctx context.Context, toolName string, err error) {
	if cb != nil && cb.OnToolCallEnd != nil {
		cb.OnToolCallEnd(ctx, toolName, err)
	}
}

// fireOnToken invokes OnToken if non-nil. Called for each text delta chunk.
func fireOnToken(cb *AgentCallbacks, ctx context.Context, delta string) {
	if cb != nil && cb.OnToken != nil {
		cb.OnToken(ctx, delta)
	}
}

// fireOnReasoning invokes OnReasoning if non-nil. Called for each reasoning delta.
func fireOnReasoning(cb *AgentCallbacks, ctx context.Context, delta string) {
	if cb != nil && cb.OnReasoning != nil {
		cb.OnReasoning(ctx, delta)
	}
}

// fireOnApprovalRequired invokes OnApprovalRequired if non-nil.
func fireOnApprovalRequired(cb *AgentCallbacks, ctx context.Context, toolName, recordID string) {
	if cb != nil && cb.OnApprovalRequired != nil {
		cb.OnApprovalRequired(ctx, toolName, recordID)
	}
}

// fireOnCompression invokes OnCompression if non-nil.
func fireOnCompression(cb *AgentCallbacks, ctx context.Context, stepsCompressed int) {
	if cb != nil && cb.OnCompression != nil {
		cb.OnCompression(ctx, stepsCompressed)
	}
}

// firePreToolCall 调用 PreToolCall 钩子；nil 回调或 nil 字段时返回 nil（放行）。
func firePreToolCall(cb *AgentCallbacks, ctx context.Context, toolName string, params map[string]any) *ToolCallDecision {
	if cb != nil && cb.PreToolCall != nil {
		return cb.PreToolCall(ctx, toolName, params)
	}
	return nil
}

// firePostToolCall 调用 PostToolCall 钩子；nil 回调或 nil 字段时返回 nil（不干预）。
func firePostToolCall(cb *AgentCallbacks, ctx context.Context, toolName string, result *action.ActionResult) *ToolCallFeedback {
	if cb != nil && cb.PostToolCall != nil {
		return cb.PostToolCall(ctx, toolName, result)
	}
	return nil
}
