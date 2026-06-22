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
	"encoding/json"
	"errors"
	"fmt"

	"github.com/inferglow/flow"
)

// RunMeta 携带一次 daemon 级别 flow 执行的元数据。当 RunID 非空时，
// executeFlow 会注入一个 WithStateModifier，将 RunID/OwnerID/LeaseTTL
// 写入持久化的 ExecutionSnapshot，以便 inferflow daemon 在重启后按
// RunID 认领（claim）并续租（renew lease）该执行。
type RunMeta struct {
	RunID    string
	OwnerID  string
	LeaseTTL int64
}

// executeFlow 使用 flow 编排执行用户请求。
// 这是 Agent.Run 的 flow 模式路径，与 executeLoop（oneshot 模式）互斥。
//
// A1: 返回 (*flow.Execution, string, error) —— 首个返回值为 exec 句柄，
// 供 daemon 在暂停后通过 ResumeFlow 续跑。暂停时不运行输出钩子/审计。
// A2: opts 在 Execute 前通过 f.ApplyOptions 应用到已构建的 Flow 上。
// A5: 当 meta.RunID 非空时，自动追加 WithStateModifier 填充快照字段。
func (e *Engine) executeFlow(ctx context.Context, f *flow.Flow, userMessage string, systemPrompt string, c *runConfig, opts []flow.FlowOption, meta RunMeta) (*flow.Execution, string, error) {
	// A4: 在 executeFlow 入口创建 SpanFlowExecute span。defer span.End()
	// 保证所有返回路径（暂停/失败/完成）都关闭 span。未配置 tracer 时
	// flow.NoopSpan() 提供零开销 no-op 实现，无需 nil 检查。
	var tracer SpanStarter
	if c != nil {
		tracer = c.tracer
	}
	_, execSpan := startFlowSpan(ctx, tracer, SpanFlowExecute, "")
	defer execSpan.End()

	// A5: 当 meta.RunID 非空时，追加 WithStateModifier 把 RunID/OwnerID/LeaseTTL
	// 写入快照。daemon 重启后可按 RunID 加载快照并按 OwnerID/LeaseTTL 认领。
	if meta.RunID != "" {
		opts = append(opts, flow.WithStateModifier(func(snap *flow.ExecutionSnapshot) *flow.ExecutionSnapshot {
			if snap != nil {
				snap.ExecutionID = meta.RunID
				snap.OwnerID = meta.OwnerID
				snap.LeaseTTL = meta.LeaseTTL
			}
			return snap
		}))
	}

	// A2: 将 opts 应用到已构建的 Flow 上（构建期未设置 checkpoint 配置时，
	// 由 daemon/调用方在运行期注入）。
	if len(opts) > 0 {
		f.ApplyOptions(opts...)
	}

	// 1. 构建 FlowContext
	// A3: 注入 tracer / piiMasker / outputHook，让 step 可以通过
	// FlowContext.StartSpan / MaskInput / CheckOutput 访问横切能力。
	fc := &flowContextImpl{
		session:    e.session,
		actionExt:  e.actionExt,
		modelReq:   e.modelReq,
		auditHook:  e.auditHook,
		tracer:     tracer,
		piiMasker:  nil,
		outputHook: nil,
		engine:     e, // 注入 engine，使 step 可调用 RunAgent
	}
	if c != nil {
		// 仅在对应 feature 启用时注入 masker/hook，保持与 executeFlow
		// 已完成路径一致的特性门控行为。
		if c.features.PIIMasking {
			fc.piiMasker = c.piiMasker
		}
		if c.features.PromptInjection {
			fc.outputHook = c.outputHook
		}
	}

	// 2. 注入到 context
	ctx = flow.WithFlowContext(ctx, fc)

	// 3. 添加用户消息到 session
	e.session.AddUserMessage(userMessage)

	// 4. 执行 flow
	exec := f.Execute(ctx, userMessage)

	// 5. 暂停路径：返回 exec 句柄，不运行输出钩子/审计/会话。
	//    若配置了 auto-checkpoint，通过 f.Pause 触发快照持久化，
	//    使 daemon 在重启后可按 checkPointID/RunID 加载并续跑。
	if exec.State.Status == flow.StatusPaused {
		// A4: 在调用 f.Pause 前创建 SpanPause span，记录暂停事件。
		// 用独立 ctx 而非 execSpan 的 ctx，避免成为其 child（语义上
		// pause 是 execute 的兄弟事件而非子操作）。
		_, pauseSpan := startFlowSpan(ctx, tracer, SpanPause, "")
		f.Pause(exec, "pause-signal")
		pauseSpan.End()
		return exec, "", nil
	}

	// 6. 失败路径
	// A6: 在返回错误前对错误消息做 PII 脱敏，避免 step 抛出的错误
	// 把敏感数据（如邮箱、手机号）冒泡到上层日志/响应。仅在 PIIMasking
	// 启用且配置了 masker 时生效；masker 自身 ApplyOn 决定是否实际替换。
	if exec.State.Status == flow.StatusFailed {
		errMsg := "flow execution failed with unknown error"
		if len(exec.State.Errors) > 0 {
			errMsg = fmt.Sprintf("flow execution failed: %v", exec.State.Errors[0])
		}
		if fc.piiMasker != nil {
			errMsg = fc.piiMasker.MaskOutput(errMsg)
		}
		return exec, "", errors.New(errMsg)
	}

	// 7. 完成路径：提取响应文本
	response := extractFlowResponse(exec.State.Result)

	// 8. 输出安全钩子（复用 Run 中的逻辑）
	if c.features.PromptInjection && c.outputHook != nil {
		if err := c.outputHook.CheckOutput(response); err != nil {
			return exec, "", err
		}
	}
	if c.features.PIIMasking && c.piiMasker != nil {
		response = c.piiMasker.MaskOutput(response)
	}

	// 9. 记录审计
	fc.AuditAppend("flow", "execute", userMessage, response)

	// 10. 添加助手回复到 session
	e.session.AddAssistantMessage(response)

	return exec, response, nil
}

// startFlowSpan 是 executeFlow / ResumeFlow 共用的 span 启动 helper。
// tracer 为 nil 时返回原始 ctx 和 flow.NoopSpan()，避免在每个调用点重复
// nil 检查。返回的 flow.Span 由调用方 defer End()。
func startFlowSpan(ctx context.Context, tracer SpanStarter, kind SemanticSpanKind, name string) (context.Context, flow.Span) {
	if tracer == nil {
		return ctx, flow.NoopSpan()
	}
	newCtx, otelSpan := tracer.Start(ctx, semanticSpanName(kind, name))
	return newCtx, &otelSpanAdapter{otel: otelSpan}
}

// extractFlowResponse 从 flow 执行结果中提取响应文本。
//
// A7: 当 result 是 map[string]any 时按顺序尝试 final_response / response /
// output / result / answer 五个常见键；若都没有字符串值，则用 json.Marshal
// 输出稳定 JSON（比 fmt.Sprint 的 map[...] 表示更可读且可解析）。
// Marshal 失败时回退到 fmt.Sprint 以保证总有非空返回。
func extractFlowResponse(result any) string {
	switch v := result.(type) {
	case string:
		return v
	case map[string]any:
		for _, key := range []string{"final_response", "response", "output", "result", "answer"} {
			if s, ok := v[key].(string); ok {
				return s
			}
		}
		data, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprint(v)
		}
		return string(data)
	default:
		return fmt.Sprint(v)
	}
}
