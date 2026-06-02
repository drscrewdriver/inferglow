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
	"context"
	"errors"
)

// ErrPauseRequested 是 step 主动调用 FlowContext.RequestPause 时返回的
// 哨兵错误。flow.Execute 在 step.Func 返回该错误时将状态置为 StatusPaused
// 而非 StatusFailed，且不会把它追加到 exec.State.Errors。
var ErrPauseRequested = errors.New("flow: pause requested by step")

// SpanKind 标识 flow 包内部 span 的语义类别。它独立于 otel 的 trace.SpanKind
// （后者描述 client/server 角色），仅为 flow 包提供稳定的语义类型枚举。
// flowContextImpl 在 orchestrator/agent 中负责把 SpanKind 映射到 otel 的
// 实际 span；flow 包本身不依赖 observability。
type SpanKind int

const (
	// SpanKindInternal 标识 flow 内部步骤（如 loop / state 处理）的 span。
	SpanKindInternal SpanKind = iota
	// SpanKindStep 标识单个 flow step 执行的 span。
	SpanKindStep
	// SpanKindTool 标识 flow step 内调用外部工具/Action 的 span。
	SpanKindTool
)

// Span 是 flow 包对 tracing span 的最小抽象。flow 包不直接依赖 observability，
// 仅暴露 End() 方法供调用方在 step 结束时调用。当未配置 tracer 时，
// FlowContext.StartSpan 返回一个 no-op Span。
type Span interface {
	End()
}

// noopSpan 是 StartSpan 在没有 tracer 时返回的空 span。
type noopSpan struct{}

func (noopSpan) End() {}

// NoopSpan 返回一个 no-op Span，调用方可以在没有 tracer 的代码路径上
// 使用同一份代码避免 nil 检查。End() 调用是空操作。
func NoopSpan() Span { return noopSpan{} }

// flowContextKey 是 context.Value 的私有 key 类型。
type flowContextKey struct{}

// FlowContext 为 flow 步骤提供横切关注点访问。
// 定义在 flow 包（不依赖 orchestrator）；由 orchestrator 提供具体实现并注入。
// 接口方法仅使用基础类型（context.Context, string, map[string]any, any），
// 避免引入 action/model/session 依赖。
type FlowContext interface {
	// ExecuteAction 按名称调用已注册的 Action。
	ExecuteAction(ctx context.Context, name string, params map[string]any) (any, error)

	// GenerateModel 调用 LLM 生成回复。
	GenerateModel(ctx context.Context, system string, userMessage string) (string, error)

	// SessionHistory 返回当前对话历史（[]map[string]any 格式，
	// 每个元素含 "role" 和 "content" 键）。
	SessionHistory() []map[string]any

	// AppendSession 向会话追加一条消息。
	AppendSession(role string, content any)

	// AuditAppend 记录审计条目。审计未启用时为 no-op。
	AuditAppend(source, action string, input, output any)

	// SetValue / GetValue 提供 flow 执行期间的键值存储。
	SetValue(key string, value any)
	GetValue(key string) (any, bool)

	// StartSpan 启动一个 tracing span。未配置 tracer 时返回原始 ctx 和 no-op Span。
	// kind 是 flow 内部的语义类别（非 otel 的 trace.SpanKind）。
	StartSpan(ctx context.Context, kind SpanKind, name string) (context.Context, Span)

	// MaskInput 对 step 输入应用 PII 脱敏。未配置 masker 时返回原始 input。
	MaskInput(input string) string

	// CheckOutput 对 step 输出做注入/安全校验。未配置 hook 时返回 nil。
	CheckOutput(output string) error

	// RequestPause 让 step 主动请求挂起当前 flow 执行。返回 ErrPauseRequested；
	// step 应将该错误作为 StepFunc 的返回值，flow.Execute 会捕获它并将状态
	// 置为 StatusPaused（而不是 StatusFailed），同时不把它写入 Errors。
	RequestPause(reason string) error

	// RunAgent 在 step 内部触发一次完整的多轮 Agent 循环（PLAN→EXECUTE）。
	// opts 为 nil 时使用零值（MaxRounds=10, SessionIsolation=false）。
	// 返回 Agent 最终回复文本。未配置 Agent 运行时返回 ErrAgentNotConfigured。
	RunAgent(ctx context.Context, userMessage string, systemPrompt string, opts *AgentRunOptions) (string, error)

	// RunAgentParallel 触发多个子 Agent 循环，全部完成后返回各自结果。
	// 当前实现为顺序降级执行；后续可升级为真并行（goroutine + WorkerPool），
	// 调用方代码无需修改。每个子 Agent 自动使用 SessionIsolation=true。
	RunAgentParallel(ctx context.Context, agents []AgentSubTask) ([]string, error)
}

// ErrAgentNotConfigured 是 RunAgent / RunAgentParallel 在未配置 Agent 运行时
// 返回的哨兵错误。通常出现在直接使用 flow.Execute 而非通过 orchestrator/agent
// 的 executeFlow 路径时（后者会注入 engine 引用）。
var ErrAgentNotConfigured = errors.New("flow: agent runtime not configured")

// AgentRunOptions 配置单次 Agent 循环的行为。
// 采用结构化选项而非位置参数，为后续并行子 Agent / Session 隔离 / Token 预算等
// 扩展预留空间，不破坏已有调用方。
type AgentRunOptions struct {
	// MaxRounds 最大迭代轮数。0 = 默认 10。
	MaxRounds int
	// SessionIsolation 为 true 时，内嵌 Agent 使用独立 Session（不污染外层会话历史）。
	// 默认为 false（共享 Session）。
	SessionIsolation bool
}

// AgentSubTask 描述 RunAgentParallel 中的一个并行子 Agent 任务。
type AgentSubTask struct {
	// Label 是子 Agent 的标识（用于日志/审计区分）。
	Label string
	// UserMessage 是该子 Agent 的用户输入。
	UserMessage string
	// SystemPrompt 是该子 Agent 的系统提示词。
	SystemPrompt string
	// MaxRounds 该子 Agent 的最大迭代轮数。0 = 默认 10。
	MaxRounds int
}

// WithFlowContext 将 FlowContext 注入到 context.Context 中。
// 返回新的 context（不修改原始 ctx）。
func WithFlowContext(ctx context.Context, fc FlowContext) context.Context {
	return context.WithValue(ctx, flowContextKey{}, fc)
}

// FlowContextFrom 从 context.Context 中提取 FlowContext。
// 若未注入，返回 (nil, false)。
func FlowContextFrom(ctx context.Context) (FlowContext, bool) {
	fc, ok := ctx.Value(flowContextKey{}).(FlowContext)
	return fc, ok
}

// pauseSignalKey 是 context.Value 的私有 key 类型，用于携带暂停信号。
type pauseSignalKey struct{}

// WithPauseSignal 将一个暂停信号 channel 注入到 context.Context 中。
// Flow.Execute 在每个步骤执行前会通过 PauseSignalFrom 检查该 channel：
// 当 channel 被关闭（或写入）时，执行立即暂停并返回 StatusPaused。
// 传入 nil channel 等同于不注入（PauseSignalFrom 返回 false）。
func WithPauseSignal(ctx context.Context, ch <-chan struct{}) context.Context {
	return context.WithValue(ctx, pauseSignalKey{}, ch)
}

// PauseSignalFrom 从 context.Context 中提取暂停信号 channel。
// 若未注入或值为 nil，返回 (nil, false)。
func PauseSignalFrom(ctx context.Context) (<-chan struct{}, bool) {
	ch, ok := ctx.Value(pauseSignalKey{}).(<-chan struct{})
	return ch, ok
}
