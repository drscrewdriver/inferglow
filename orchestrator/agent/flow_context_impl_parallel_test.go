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
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/inferglow/flow"
	"github.com/inferglow/model"
	"github.com/inferglow/session"
)

// TestRunAgentParallel_Timing verifies that 3 parallel sub-tasks complete
// in less time than sequential execution would require. Each sub-task
// simulates a 100ms delay via the mock model requester.
func TestRunAgentParallel_Timing(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()

	var callCount atomic.Int32
	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			// Simulate some work per sub-task.
			time.Sleep(100 * time.Millisecond)
			n := callCount.Add(1)
			ch := make(chan *model.StreamChunk, 1)
			ch <- &model.StreamChunk{
				Delta:  fmt.Sprintf(`{"next_action":"response","final_response":"result-%d"}`, n),
				IsDone: true,
			}
			close(ch)
			return ch, nil
		},
	}

	step := flow.NewStep("parallel-timing", func(ctx context.Context, input any) (any, error) {
		fc, ok := flow.FlowContextFrom(ctx)
		if !ok {
			return nil, errors.New("FlowContext not found")
		}
		results, err := fc.RunAgentParallel(ctx, []flow.AgentSubTask{
			{Label: "t1", UserMessage: "a", SystemPrompt: "s", MaxRounds: 3},
			{Label: "t2", UserMessage: "b", SystemPrompt: "s", MaxRounds: 3},
			{Label: "t3", UserMessage: "c", SystemPrompt: "s", MaxRounds: 3},
		})
		if err != nil {
			return nil, fmt.Errorf("RunAgentParallel: %w", err)
		}
		return strings.Join(results, ","), nil
	}).Build()
	f := flow.NewFlow().AddStep(step).Build()

	agent := New(sess, actExt, mockReq, WithFlow(f))

	start := time.Now()
	resp, err := agent.Run(context.Background(), "start")
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if callCount.Load() != 3 {
		t.Errorf("expected 3 model calls, got %d", callCount.Load())
	}

	// Sequential would take at least 300ms (3 × 100ms).
	// Parallel should complete in well under 300ms (allowing for scheduling overhead).
	if elapsed > 280*time.Millisecond {
		t.Errorf("parallel execution took %v, expected < 280ms (sequential would be ≥300ms)", elapsed)
	}

	// Verify all 3 results are present (order may vary).
	parts := strings.Split(resp, ",")
	if len(parts) != 3 {
		t.Fatalf("expected 3 results, got %d: %q", len(parts), resp)
	}
	seen := make(map[string]bool)
	for _, p := range parts {
		seen[p] = true
	}
	for i := 1; i <= 3; i++ {
		key := fmt.Sprintf("result-%d", i)
		if !seen[key] {
			t.Errorf("missing result %q in %q", key, resp)
		}
	}
}

// TestRunAgentParallel_ErrorPropagation verifies that an error in one
// sub-task is propagated to the caller.
func TestRunAgentParallel_ErrorPropagation(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()

	var callCount atomic.Int32
	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			n := callCount.Add(1)
			if n == 2 {
				return nil, fmt.Errorf("simulated model error")
			}
			ch := make(chan *model.StreamChunk, 1)
			ch <- &model.StreamChunk{
				Delta:  fmt.Sprintf(`{"next_action":"response","final_response":"ok-%d"}`, n),
				IsDone: true,
			}
			close(ch)
			return ch, nil
		},
	}

	step := flow.NewStep("parallel-error", func(ctx context.Context, input any) (any, error) {
		fc, ok := flow.FlowContextFrom(ctx)
		if !ok {
			return nil, errors.New("FlowContext not found")
		}
		_, err := fc.RunAgentParallel(ctx, []flow.AgentSubTask{
			{Label: "ok", UserMessage: "a", SystemPrompt: "s", MaxRounds: 3},
			{Label: "fail", UserMessage: "b", SystemPrompt: "s", MaxRounds: 3},
		})
		if err != nil {
			return nil, err
		}
		return "should-not-reach", nil
	}).Build()
	f := flow.NewFlow().AddStep(step).Build()

	agent := New(sess, actExt, mockReq, WithFlow(f))
	_, err := agent.Run(context.Background(), "start")
	if err == nil {
		t.Fatal("expected error from parallel execution, got nil")
	}
	if !strings.Contains(err.Error(), "simulated model error") {
		t.Errorf("expected error to contain 'simulated model error', got: %v", err)
	}
}
