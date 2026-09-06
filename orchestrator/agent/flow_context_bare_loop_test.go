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
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/inferglow/action"
	"github.com/inferglow/flow"
	"github.com/inferglow/model"
	"github.com/inferglow/session"
)

// TestRunAgent_DepthEnforcedForSequentialSubRuns guards the R9 fix: the
// sequential RunAgent path runs on the SAME engine, so it must count toward
// MaxDepth — previously depth only incremented on the parallel branch and a
// model alternating spawn_agent → RunAgent → spawn_agent never tripped the
// limit.
func TestRunAgent_DepthEnforcedForSequentialSubRuns(t *testing.T) {
	sess := session.NewSession("depth-test", 10000)
	actExt := NewActionExtension()
	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 1)
			ch <- &model.StreamChunk{Delta: `{"next_action":"response","final_response":"sub-ok"}`, IsDone: true}
			close(ch)
			return ch, nil
		},
	}
	ag := New(sess, actExt, mockReq)
	fc := &flowContextImpl{engine: ag.engine}

	// At the limit: refused outright.
	ag.engine.depth = 3
	if _, err := fc.RunAgent(context.Background(), "x", "", &flow.AgentRunOptions{MaxDepth: 3}); !errors.Is(err, flow.ErrAgentDepthExceeded) {
		t.Fatalf("expected ErrAgentDepthExceeded at limit, got %v", err)
	}

	// Below the limit: the sub-run executes and the depth is restored — a
	// nested spawn inside the sub-loop would see the incremented value.
	ag.engine.depth = 0
	resp, err := fc.RunAgent(context.Background(), "x", "", &flow.AgentRunOptions{MaxDepth: 3, MaxRounds: 2})
	if err != nil {
		t.Fatalf("RunAgent below limit failed: %v", err)
	}
	if resp != "sub-ok" {
		t.Errorf("resp = %q, want sub-ok", resp)
	}
	if ag.engine.depth != 0 {
		t.Fatalf("depth not restored after sub-run: %d", ag.engine.depth)
	}
}

// TestRunAgent_NestedSpawnSeesIncrementedDepth drives the real nesting path:
// a sub-run whose model calls an action that spawns again must hit the depth
// ceiling on the second (nested) RunAgent when MaxDepth is 1.
func TestRunAgent_NestedSpawnSeesIncrementedDepth(t *testing.T) {
	sess := session.NewSession("nested-test", 10000)
	actExt := NewActionExtension()

	var nestedErr error
	var mu sync.Mutex
	nested := &action.Action{
		Name:        "nested_spawn",
		Description: "spawn again from inside a sub-run",
		Executor: &nestedSpawnExecutor{onResult: func(err error) {
			mu.Lock()
			nestedErr = err
			mu.Unlock()
		}},
	}
	if err := actExt.Register(nested); err != nil {
		t.Fatalf("register nested_spawn: %v", err)
	}

	calls := 0
	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			calls++
			ch := make(chan *model.StreamChunk, 1)
			if calls == 1 {
				ch <- &model.StreamChunk{Delta: `{"next_action":"execute","action_calls":[{"name":"nested_spawn","params":{}}]}`, IsDone: true}
			} else {
				ch <- &model.StreamChunk{Delta: `{"next_action":"response","final_response":"sub-ok"}`, IsDone: true}
			}
			close(ch)
			return ch, nil
		},
	}
	ag := New(sess, actExt, mockReq)

	fc := &flowContextImpl{engine: ag.engine}
	resp, err := fc.RunAgent(context.Background(), "outer", "", &flow.AgentRunOptions{MaxDepth: 1, MaxRounds: 4})
	if err != nil {
		t.Fatalf("outer RunAgent failed: %v", err)
	}
	if resp != "sub-ok" {
		t.Errorf("resp = %q, want sub-ok", resp)
	}
	mu.Lock()
	defer mu.Unlock()
	if !errors.Is(nestedErr, flow.ErrAgentDepthExceeded) {
		t.Fatalf("nested RunAgent error = %v, want ErrAgentDepthExceeded (depth not enforced for sequential sub-runs)", nestedErr)
	}
}

type nestedSpawnExecutor struct {
	onResult func(err error)
}

func (e *nestedSpawnExecutor) Execute(ctx context.Context, input map[string]any) (*action.ActionResult, error) {
	fc, ok := flow.ContextFrom(ctx)
	if !ok {
		return &action.ActionResult{OK: false, Status: "error", Error: "no flow ctx"}, nil
	}
	_, err := fc.RunAgent(ctx, "inner task", "", &flow.AgentRunOptions{MaxDepth: 1, MaxRounds: 2})
	e.onResult(err)
	if err != nil {
		return &action.ActionResult{OK: false, Status: "error", Error: fmt.Sprintf("%v", err)}, nil
	}
	return &action.ActionResult{OK: true, Status: "ok", Result: "inner done"}, nil
}

// TestExecuteLoop_InstallsFlowContext verifies the R9 bare-loop fix: without
// a flow configured, Agent.Run → executeLoop must still install a flow
// context so Context-dependent builtin actions (spawn_agent, …) work.
func TestExecuteLoop_InstallsFlowContext(t *testing.T) {
	sess := session.NewSession("bare-loop-test", 10000)
	actExt := NewActionExtension()

	var gotFC flow.Context
	var gotOK bool
	probe := &action.Action{
		Name: "flow_probe",
		Executor: &flowProbeExecutor{onProbe: func(fc flow.Context, ok bool) {
			gotFC, gotOK = fc, ok
		}},
	}
	if err := actExt.Register(probe); err != nil {
		t.Fatalf("register flow_probe: %v", err)
	}

	calls := 0
	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			calls++
			ch := make(chan *model.StreamChunk, 1)
			if calls == 1 {
				ch <- &model.StreamChunk{Delta: `{"next_action":"execute","action_calls":[{"name":"flow_probe","params":{}}]}`, IsDone: true}
			} else {
				ch <- &model.StreamChunk{Delta: `{"next_action":"response","final_response":"done"}`, IsDone: true}
			}
			close(ch)
			return ch, nil
		},
	}
	ag := New(sess, actExt, mockReq)
	resp, err := ag.Run(context.Background(), "start") // NO WithFlow — bare path
	if err != nil {
		t.Fatalf("bare Run failed: %v", err)
	}
	if resp != "done" {
		t.Errorf("resp = %q, want done", resp)
	}
	if !gotOK || gotFC == nil {
		t.Fatal("executeLoop did not install a flow context; Context-dependent actions would fail")
	}
}

type flowProbeExecutor struct {
	onProbe func(fc flow.Context, ok bool)
}

func (e *flowProbeExecutor) Execute(ctx context.Context, input map[string]any) (*action.ActionResult, error) {
	fc, ok := flow.ContextFrom(ctx)
	e.onProbe(fc, ok)
	return &action.ActionResult{OK: true, Status: "ok", Result: "probed"}, nil
}
