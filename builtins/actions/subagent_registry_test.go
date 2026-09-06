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

package actions

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/inferglow/flow"
)

// flowErrContext returns a flow context whose RunAgent always fails.
func flowErrContext() context.Context {
	return flow.WithFlowContext(context.Background(), &subAgentFlowMock{agentErr: errors.New("mock failure")})
}

func TestSubagentRegistry_StartFinishList(t *testing.T) {
	r := NewSubagentRegistry()

	rec := r.Start("sess-a", "do the thing", "be brief")
	if rec.ID == "" || rec.Status != SpawnStatusRunning {
		t.Fatalf("start record = %+v, want running with id", rec)
	}

	// Live row visible while running, attributed to its session.
	live := r.List("sess-a")
	if len(live) != 1 || live[0].Status != SpawnStatusRunning {
		t.Fatalf("live list = %+v, want 1 running row", live)
	}

	r.Finish(rec.ID, true, "all done", "")
	got := r.List("sess-a")[0]
	if got.Status != SpawnStatusDone || got.Result != "all done" || got.EndedAt == 0 {
		t.Fatalf("finished record = %+v, want done with result", got)
	}
	if got.StartedAt > got.EndedAt {
		t.Errorf("StartedAt %d > EndedAt %d", got.StartedAt, got.EndedAt)
	}
}

func TestSubagentRegistry_ErrorFinishAndSessionFilter(t *testing.T) {
	r := NewSubagentRegistry()
	ok := r.Start("sess-a", "task a", "")
	bad := r.Start("sess-b", "task b", "")
	r.Finish(bad.ID, false, "", "boom")

	if got := r.List("sess-b")[0]; got.Status != SpawnStatusError || got.Error != "boom" {
		t.Fatalf("error record = %+v, want error/boom", got)
	}
	if got := r.List("sess-a"); len(got) != 1 || got[0].ID != ok.ID {
		t.Fatalf("session filter leaked rows: %+v", got)
	}
	if all := r.List(""); len(all) != 2 {
		t.Fatalf("unfiltered list = %d rows, want 2", len(all))
	}
	// Newest first: sess-b (started later) before sess-a.
	if all := r.List(""); all[0].ID != bad.ID {
		t.Fatalf("newest-first order violated: %+v", all)
	}
}

func TestSubagentRegistry_FinishUnknownIDNoop(t *testing.T) {
	r := NewSubagentRegistry()
	r.Finish("spawn-9999", true, "x", "") // must not panic
	if got := r.List(""); len(got) != 0 {
		t.Fatalf("unexpected rows: %+v", got)
	}
}

func TestSubAgentAction_RegistryInstrumentation(t *testing.T) {
	reg := NewSubagentRegistry()
	a := NewSubAgentAction(SubAgentConfig{
		Registry:        reg,
		ParentSessionFn: func(ctx context.Context) string { return "sess-42" },
	})
	res, err := a.Executor.Execute(subAgentFlowContext("child result"), map[string]any{
		"task": "delegate this",
	})
	if err != nil || !res.OK {
		t.Fatalf("execute failed: err=%v res=%+v", err, res)
	}
	rows := reg.List("sess-42")
	if len(rows) != 1 {
		t.Fatalf("want 1 record, got %+v", rows)
	}
	rec := rows[0]
	if rec.Status != SpawnStatusDone || rec.Result != "child result" {
		t.Errorf("record = %+v, want done/child result", rec)
	}
	if rec.Task != "delegate this" || rec.ParentSession != "sess-42" {
		t.Errorf("attribution wrong: %+v", rec)
	}
}

func TestSubAgentAction_RegistryRecordsError(t *testing.T) {
	reg := NewSubagentRegistry()
	a := NewSubAgentAction(SubAgentConfig{Registry: reg})
	a.Executor.Execute(flowErrContext(), map[string]any{"task": "will fail"})
	rows := reg.List("")
	if len(rows) == 0 || rows[0].Status != SpawnStatusError || !strings.Contains(rows[0].Error, "mock failure") {
		t.Fatalf("error record = %+v, want error containing mock failure", rows)
	}
}
