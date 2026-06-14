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
	"fmt"
	"strings"
	"sync"

	"github.com/inferglow/audit"
	"github.com/inferglow/flow"
	"github.com/inferglow/model"
	"github.com/inferglow/observability/otel"
	"github.com/inferglow/orchestrator/actionruntime"
	"go.opentelemetry.io/otel/trace"
)

// flowContextImpl bridges orchestrator components into the flow.FlowContext
// interface. It is constructed by the Engine (or Agent) and injected into
// flow steps via flow.WithContext so that flow nodes can invoke actions,
// call the LLM, read/append session history, record audit entries, and
// share key/value state across steps without depending on orchestrator
// concrete types.
type flowContextImpl struct {
	session   *SessionExtension
	actionExt *ActionExtension
	modelReq  model.StreamRequester
	auditHook audit.AuditHook
	values    sync.Map
	// tracer 是可选的 OpenTelemetry tracer。nil 时 StartSpan 返回 no-op span。
	// 由 executeFlow 从 runConfig.tracer 注入（runConfig 又来自 Agent.tracer）。
	tracer *otel.Tracer
	// piiMasker 是可选的 PII 脱敏器。nil 时 MaskInput 直接返回原值。
	// 用于在 step 内对动态构造的输入做脱敏（如调外部 Action 前去除 PII）。
	piiMasker PIIMasker
	// outputHook 是可选的输出安全钩子。nil 时 CheckOutput 返回 nil。
	// 用于 step 在产出最终/中间结果时自检 prompt injection。
	outputHook OutputSecurityHook
	// engine 是可选的 Engine 引用。nil 时 RunAgent / RunAgentParallel 返回
	// flow.ErrAgentNotConfigured。由 executeFlow 注入，使 step 可以在内部触发
	// 多轮 Agent 循环（PLAN→EXECUTE）。
	engine *Engine
}

// otelSpanAdapter 把 otel 的 trace.Span 适配为 flow.Span 接口。
// flow 包只暴露 End() 方法，故 adapter 仅代理 End。
type otelSpanAdapter struct {
	otel trace.Span
}

func (a *otelSpanAdapter) End() {
	if a.otel != nil {
		a.otel.End()
	}
}

// flowToOtelKind 把 flow 包内部的 SpanKind 映射到 otel 的语义 SpanKind。
// flow 包不依赖 observability，故该映射必须在 orchestrator 层完成。
// 映射规则：内部 step -> SpanAgentRun（一个稳定语义名），step -> SpanFlowExecute，
// tool -> SpanToolCall。这些只是 span 名约定；调用方仍可通过 name 覆盖具体名。
func flowToOtelKind(kind flow.SpanKind) otel.SpanKind {
	switch kind {
	case flow.SpanKindInternal:
		// 内部 span 复用 SpanFlowExecute 的语义名空间。
		return otel.SpanFlowExecute
	case flow.SpanKindStep:
		return otel.SpanFlowExecute
	case flow.SpanKindTool:
		return otel.SpanToolCall
	default:
		return otel.SpanFlowExecute
	}
}

// Compile-time interface satisfaction check.
var _ flow.FlowContext = (*flowContextImpl)(nil)

// ExecuteAction invokes a registered action by name with the given params
// and returns its result. It mirrors the Engine's dispatch path: a fresh
// ActionDispatcher is created with the audit hook so per-action audit
// entries flow through the same audit chain as engine decisions.
func (fc *flowContextImpl) ExecuteAction(ctx context.Context, name string, params map[string]any) (any, error) {
	dispatcher := actionruntime.NewActionDispatcherWithAudit(fc.actionExt.GetRegistry(), fc.auditHook)
	results := dispatcher.Execute(ctx, []actionruntime.ActionCall{{Name: name, Params: params}})
	if len(results) == 0 || results[0] == nil {
		return nil, fmt.Errorf("action %q returned no result", name)
	}
	r := results[0]
	if !r.OK {
		return nil, fmt.Errorf("action %q failed: %s", name, r.Error)
	}
	return r.Result, nil
}

// GenerateModel calls the LLM with a system prompt and a single user
// message, then returns the concatenated streaming response. It follows
// the same GenerateRequestData -> RequestModel -> collect Delta path as
// Engine.executeLoop, minus the timeout/preempt machinery.
func (fc *flowContextImpl) GenerateModel(ctx context.Context, system, userMessage string) (string, error) {
	req := &model.ModelRequest{
		System:      system,
		ChatHistory: []model.ChatMessage{{Role: "user", Content: userMessage}},
	}
	data, err := fc.modelReq.GenerateRequestData(ctx, req)
	if err != nil {
		return "", err
	}
	stream, err := fc.modelReq.RequestModel(ctx, data)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	for chunk := range stream {
		sb.WriteString(chunk.Delta)
	}
	return sb.String(), nil
}

// SessionHistory returns the current context window as a slice of
// map[string]any, each containing "role" and "content" keys. It reuses
// SessionExtension.PreparePrompt so ContentBlock content is serialized
// to strings consistently with the engine's prompt-building path.
func (fc *flowContextImpl) SessionHistory() []map[string]any {
	msgs := fc.session.PreparePrompt()
	out := make([]map[string]any, len(msgs))
	for i, m := range msgs {
		out[i] = map[string]any{
			"role":    m.Role,
			"content": m.Content,
		}
	}
	return out
}

// AppendSession appends a message to the session history. The content is
// coerced to a string via fmt.Sprint to match the string-only signatures
// of SessionExtension.AddUserMessage / AddAssistantMessage. Unknown roles
// are ignored since SessionExtension only exposes user/assistant appenders.
func (fc *flowContextImpl) AppendSession(role string, content any) {
	text := fmt.Sprint(content)
	switch role {
	case "user":
		fc.session.AddUserMessage(text)
	case "assistant":
		fc.session.AddAssistantMessage(text)
	default:
		// Unknown roles are ignored rather than synthesized as a
		// different role, matching SessionExtension which only
		// exposes user/assistant appenders.
	}
}

// AuditAppend records an audit entry when an audit hook is configured.
// A nil hook makes this a no-op so flow steps can call it unconditionally.
// The Append return values are intentionally ignored so an audit failure
// cannot break the flow.
func (fc *flowContextImpl) AuditAppend(source, action string, input, output any) {
	if fc.auditHook == nil {
		return
	}
	entry := &audit.AuditEntry{
		Source: source,
		Action: action,
		Input:  input,
		Output: output,
	}
	_, _ = fc.auditHook.Append(entry)
}

// SetValue stores a value in the per-execution key/value store.
func (fc *flowContextImpl) SetValue(key string, value any) {
	fc.values.Store(key, value)
}

// GetValue retrieves a value from the per-execution key/value store.
// The bool result is false when the key is absent.
func (fc *flowContextImpl) GetValue(key string) (any, bool) {
	return fc.values.Load(key)
}

// StartSpan 启动一个 tracing span。
// 未配置 tracer (fc.tracer == nil) 时返回原始 ctx 和 no-op Span，
// 这样 step 可以无条件调用而无性能/依赖开销。
// 配置了 tracer 时通过 otel.Tracer.StartSpan 创建真实 span，并用
// otelSpanAdapter 包装为 flow.Span（仅暴露 End）。
// kind 由 flow 包定义，flowToOtelKind 负责映射到 otel 的语义 SpanKind。
func (fc *flowContextImpl) StartSpan(ctx context.Context, kind flow.SpanKind, name string) (context.Context, flow.Span) {
	if fc.tracer == nil {
		return ctx, flow.NoopSpan()
	}
	newCtx, otelSpan := fc.tracer.StartSpan(ctx, flowToOtelKind(kind), name)
	return newCtx, &otelSpanAdapter{otel: otelSpan}
}

// MaskInput 对 step 输入应用 PII 脱敏。
// 未配置 masker 时直接返回 input，避免对无 PII 风险的 step 引入额外开销。
// 配置了 masker 时调用其 MaskInput 方法（PIIMasker 接口同时支持输入/输出
// 脱敏，由 masker 自身的 ApplyOn 配置决定是否实际生效）。
func (fc *flowContextImpl) MaskInput(input string) string {
	if fc.piiMasker == nil {
		return input
	}
	return fc.piiMasker.MaskInput(input)
}

// CheckOutput 对 step 输出做注入/安全校验。
// 未配置 hook 时返回 nil（无校验），让 step 可以无条件调用。
// 配置了 hook 时调用其 CheckOutput 方法；非 nil 错误会向上冒泡让 step
// 终止执行（step 应将错误作为 StepFunc 的返回值返回）。
func (fc *flowContextImpl) CheckOutput(output string) error {
	if fc.outputHook == nil {
		return nil
	}
	return fc.outputHook.CheckOutput(output)
}

// RequestPause 让 step 主动请求挂起当前 flow。
// 返回 flow.ErrPauseRequested 哨兵错误；step 应将其作为 StepFunc 的返回值，
// flow.Execute 会捕获该错误并将状态置为 StatusPaused（而非 StatusFailed），
// 同时不把它追加到 exec.State.Errors。reason 当前仅作为审计/调试信息，
// 不进入返回值（调用方可通过 step log 的 Error 字段观察到）。
func (fc *flowContextImpl) RequestPause(_ string) error {
	return flow.ErrPauseRequested
}

// RunAgent 在 step 内部触发一次完整的多轮 Agent 循环（PLAN→EXECUTE）。
// 通过 Engine.executeLoop 实现，复用全部 PLAN→EXECUTE / LoopGuard / Cancel / L3-L4 校验逻辑。
// engine 为 nil 时返回 flow.ErrAgentNotConfigured（未配置 Agent 运行时）。
func (fc *flowContextImpl) RunAgent(ctx context.Context, userMessage string, systemPrompt string, opts *flow.AgentRunOptions) (string, error) {
	if fc.engine == nil {
		return "", flow.ErrAgentNotConfigured
	}
	maxRounds := 10
	_ = false // sessionIsolation 预留，Phase 2 使用
	if opts != nil {
		if opts.MaxRounds > 0 {
			maxRounds = opts.MaxRounds
		}
		// opts.SessionIsolation 预留：当前退化为共享 Session，
		// Phase 2 实现 Session fork 时在此处创建子 Session 快照。
	}
	resp, err := fc.engine.RunLoop(ctx, userMessage, maxRounds, systemPrompt)
	if err != nil {
		return "", err
	}
	return resp, nil
}

// RunAgentParallel 触发多个子 Agent 循环，全部完成后返回各自结果。
// 每个子任务在独立 goroutine 中并行执行，使用 sync.WaitGroup 等待全部完成。
// 结果切片保持与输入相同的顺序（索引赋值）。每个子任务使用独立的 Engine
// 副本（共享 session/actionExt/modelReq，但拥有独立的 TurnLoop/CancelManager），
// 避免并发 TurnLoop 状态冲突。
func (fc *flowContextImpl) RunAgentParallel(ctx context.Context, agents []flow.AgentSubTask) ([]string, error) {
	results := make([]string, len(agents))
	errs := make([]error, len(agents))
	var wg sync.WaitGroup
	wg.Add(len(agents))

	for i, sub := range agents {
		go func(idx int, task flow.AgentSubTask) {
			defer wg.Done()
			// Create a child engine with its own TurnLoop/CancelManager
			// so parallel sub-agents do not share turn state.
			childEngine := cloneEngineForParallel(fc.engine)
			r, err := runAgentWithEngine(childEngine, ctx, task.UserMessage, task.SystemPrompt, &flow.AgentRunOptions{
				MaxRounds:        task.MaxRounds,
				SessionIsolation: true,
			})
			if err != nil {
				errs[idx] = fmt.Errorf("parallel agent %q (index %d): %w", task.Label, idx, err)
				return
			}
			results[idx] = r
		}(i, sub)
	}
	wg.Wait()

	// Return the first error encountered, if any.
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return results, nil
}

// cloneEngineForParallel creates a shallow copy of the engine with a fresh
// TurnLoop and CancelManager, so parallel sub-agents do not contend on
// the same turn state. The session, actionExt, modelReq and other fields
// are shared (read-only or externally synchronized).
func cloneEngineForParallel(src *Engine) *Engine {
	if src == nil {
		return nil
	}
	tl, cm := newTurnLoopAndCancel()
	return &Engine{
		session:       src.session,
		actionExt:     src.actionExt,
		modelReq:      src.modelReq,
		auditHook:     src.auditHook,
		loopGuard:     src.loopGuard,
		streamTimeout: src.streamTimeout,
		turnLoop:      tl,
		cancelManager: cm,
		outputSchema:  src.outputSchema,
		tracer:        src.tracer,
		maxToolCallRounds: src.maxToolCallRounds,
	}
}

// runAgentWithEngine runs a single agent loop using the given engine.
// Used by RunAgentParallel to avoid sharing the flowContextImpl's engine.
func runAgentWithEngine(engine *Engine, ctx context.Context, userMessage string, systemPrompt string, opts *flow.AgentRunOptions) (string, error) {
	if engine == nil {
		return "", flow.ErrAgentNotConfigured
	}
	maxRounds := 10
	if opts != nil && opts.MaxRounds > 0 {
		maxRounds = opts.MaxRounds
	}
	return engine.RunLoop(ctx, userMessage, maxRounds, systemPrompt)
}
