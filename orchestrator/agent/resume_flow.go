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

	"github.com/inferglow/flow"
	"github.com/inferglow/observability/otel"
)

// ResumeFlow loads a checkpoint snapshot identified by snapshotID from store
// and resumes execution of f from the paused step. The returned *Execution
// contains both the restored history (from the snapshot) and the newly
// executed steps (from the resume).
//
// A4: 在入口创建 SpanResume span，记录 resume 事件。Engine 不持有 tracer，
// 故通过 e.tracer 字段读取（如果存在）；当前 Engine 结构未持久化 tracer，
// ResumeFlow 的 caller（如 daemon）需要在调用前通过其他方式注入或接受
// 该 span 为 no-op。后续如需在 daemon 路径观察 resume span，可通过为
// Engine 增加 SetTracer 方法或改用 runConfig 注入。
//
// A nil store or a failed load returns an error. The resumeInput parameter is
// reserved for future use; the current implementation derives the resume input
// from the snapshot (the paused step's recorded output) via
// Flow.ResumeFromSnapshot, matching the crash-recovery contract.
func (e *Engine) ResumeFlow(ctx context.Context, f *flow.Flow, store flow.CheckpointStore, snapshotID string, resumeInput any) (*flow.Execution, error) {
	// A4: SpanResume span。Engine.tracer 为 nil 时返回 no-op，零开销。
	_, resumeSpan := startFlowSpan(ctx, e.tracer, otel.SpanResume, "")
	defer resumeSpan.End()

	_ = ctx
	_ = resumeInput
	if store == nil {
		return nil, fmt.Errorf("resume flow: checkpoint store is nil")
	}
	snapshot, err := store.Load(snapshotID)
	if err != nil {
		return nil, fmt.Errorf("resume flow: load checkpoint %q: %w", snapshotID, err)
	}
	exec := f.ResumeFromSnapshot(snapshot)
	return exec, nil
}
