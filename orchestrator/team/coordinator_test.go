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

package team

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/inferglow/orchestrator/middleware"
)

// mockRunner is a test AgentRunner that returns a fixed response.
type mockRunner struct {
	response string
	err      error
	calls    int
}

func (m *mockRunner) Run(ctx context.Context, userMessage string) (string, error) {
	m.calls++
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

func TestCoordinator_SingleMember(t *testing.T) {
	runner := &mockRunner{response: "done"}
	c := NewCoordinator([]Member{
		{Agent: runner, Role: "worker"},
	})

	result, err := c.Round(context.Background(), "do something")
	if err != nil {
		t.Fatalf("Round error: %v", err)
	}
	if result.FinalResponse != "done" {
		t.Errorf("FinalResponse = %q, want %q", result.FinalResponse, "done")
	}
	if runner.calls != 1 {
		t.Errorf("calls = %d, want 1", runner.calls)
	}
}

func TestCoordinator_MultiMember(t *testing.T) {
	r1 := &mockRunner{response: "planned"}
	r2 := &mockRunner{response: "coded"}
	r3 := &mockRunner{response: "reviewed"}

	c := NewCoordinator([]Member{
		{Agent: r1, Role: "planner"},
		{Agent: r2, Role: "coder"},
		{Agent: r3, Role: "reviewer"},
	})

	result, err := c.Round(context.Background(), "build feature")
	if err != nil {
		t.Fatalf("Round error: %v", err)
	}
	if result.FinalResponse != "reviewed" {
		t.Errorf("FinalResponse = %q, want %q", result.FinalResponse, "reviewed")
	}
	if len(result.MemberOutputs) != 3 {
		t.Errorf("MemberOutputs len = %d, want 3", len(result.MemberOutputs))
	}
	if result.MemberOutputs["planner"] != "planned" {
		t.Errorf("planner output = %q, want %q", result.MemberOutputs["planner"], "planned")
	}
}

func TestCoordinator_ContextCancel(t *testing.T) {
	runner := &mockRunner{response: "ok"}
	c := NewCoordinator([]Member{
		{Agent: runner, Role: "worker"},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := c.Round(ctx, "task")
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestCoordinator_MemberError(t *testing.T) {
	runner := &mockRunner{err: fmt.Errorf("agent failed")}
	c := NewCoordinator([]Member{
		{Agent: runner, Role: "worker"},
	})

	_, err := c.Round(context.Background(), "task")
	if err == nil {
		t.Fatal("expected error from member")
	}
	if !strings.Contains(err.Error(), "agent failed") {
		t.Errorf("error = %q, want to contain 'agent failed'", err.Error())
	}
}

func TestCoordinator_WithMiddleware(t *testing.T) {
	var middlewareCalled bool
	mw := func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context, input *middleware.Input) (*middleware.Output, error) {
			middlewareCalled = true
			return next(ctx, input)
		}
	}

	runner := &mockRunner{response: "ok"}
	c := NewCoordinator(
		[]Member{{Agent: runner, Role: "worker"}},
		WithMiddleware(mw),
	)

	_, err := c.Round(context.Background(), "task")
	if err != nil {
		t.Fatalf("Round error: %v", err)
	}
	if !middlewareCalled {
		t.Error("middleware was not called")
	}
}

func TestCoordinator_MessageBus(t *testing.T) {
	r1 := &mockRunner{response: "from planner"}
	r2 := &mockRunner{response: "from coder"}

	c := NewCoordinator([]Member{
		{Agent: r1, Role: "planner"},
		{Agent: r2, Role: "coder"},
	})

	_, err := c.Round(context.Background(), "task")
	if err != nil {
		t.Fatalf("Round error: %v", err)
	}

	history := c.Bus().History()
	if len(history) < 2 {
		t.Fatalf("bus history len = %d, want >= 2", len(history))
	}
	// First message: planner → coder
	if history[0].From != "planner" || history[0].To != "coder" {
		t.Errorf("first message: %s→%s, want planner→coder", history[0].From, history[0].To)
	}
}

func TestMessageBus_ConcurrentSafety(t *testing.T) {
	bus := newMessageBus()
	done := make(chan struct{})

	go func() {
		for i := 0; i < 100; i++ {
			bus.Post(Message{From: "a", To: "b", Content: fmt.Sprintf("msg-%d", i)})
		}
		close(done)
	}()
	go func() {
		for i := 0; i < 100; i++ {
			_ = bus.History()
		}
	}()

	<-done
	if len(bus.History()) != 100 {
		t.Errorf("expected 100 messages, got %d", len(bus.History()))
	}
}
