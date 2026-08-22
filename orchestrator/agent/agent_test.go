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
	"testing"

	"github.com/inferglow/model"
	"github.com/inferglow/session"
)

func TestAgentCreation(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()

	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			return nil, nil
		},
	}

	agent := New(sess, actExt, mockReq)
	if agent == nil {
		t.Fatal("Agent should not be nil")
	}
}

func TestAgentRunResponse(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()

	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 1)
			ch <- &model.StreamChunk{
				Delta:  `{"next_action":"response","final_response":"Hello from agent!"}`,
				IsDone: true,
			}
			close(ch)
			return ch, nil
		},
	}

	agent := New(sess, actExt, mockReq)
	result, err := agent.Run(context.Background(), "Hi")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result != "Hello from agent!" {
		t.Errorf("Result: got %q, want %q", result, "Hello from agent!")
	}
}

func TestAgentRunExecuteNoResponseTriggersSynthesis(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()

	callCount := 0
	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 1)
			callCount++
			// First calls return identical execute; the stale detector trips
			// on the 3rd consecutive identical batch (toolCallStaleThreshold=3)
			// and triggers synthesis. The synthesis call is the 4th model call,
			// so return a plain summary from call 4 onward.
			if callCount <= 3 {
				ch <- &model.StreamChunk{
					Delta:  `{"next_action":"execute","action_calls":[{"name":"test","params":{}}]}`,
					IsDone: true,
				}
			} else {
				// Synthesis call returns a response
				ch <- &model.StreamChunk{
					Delta:  "synthesis summary",
					IsDone: true,
				}
			}
			close(ch)
			return ch, nil
		},
	}

	agent := New(sess, actExt, mockReq)
	// Use a small tool-call cap to keep the test fast (stale fires before it).
	agent.engine.maxToolCallRounds = 5
	result, err := agent.Run(context.Background(), "test")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if result != "synthesis summary" {
		t.Errorf("Expected synthesis summary, got %q", result)
	}
}

func TestAgentRunWithSystemPrompt(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()

	var capturedSystem string
	mockReq := &mockModelRequester{
		requestFn: func(ctx context.Context, req *model.ModelRequest) {
			capturedSystem = req.System
		},
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 1)
			ch <- &model.StreamChunk{
				Delta:  `{"next_action":"response","final_response":"OK"}`,
				IsDone: true,
			}
			close(ch)
			return ch, nil
		},
	}

	agent := New(sess, actExt, mockReq, WithSystemPrompt("You are a test assistant"))
	_, err := agent.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if capturedSystem != "You are a test assistant" {
		t.Errorf("System prompt not passed: got %q", capturedSystem)
	}
}

// TestAgent_PersistsMaxRoundsFromNew is a regression test for BUG-18:
// WithMaxRounds passed to New was previously accepted by the option
// function but never persisted on the Agent struct, so subsequent Run
// calls (without per-call opts) fell back to the runConfig default of 10.
//
// NOTE: maxRounds counts only "response" rounds (see engine.go L1123), so a
// model that always returns the same "execute" batch is bounded by the stale
// detector, not by maxRounds. Under LG-1 the loop now terminates via
// stale→synthesis instead of erroring. This test pins that deterministic
// outcome for a stuck (always-identical-execute) model.
func TestAgent_PersistsMaxRoundsFromNew(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()

	callCount := 0
	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			callCount++
			ch := make(chan *model.StreamChunk, 1)
			// First toolCallStaleThreshold calls are identical execute; the
			// stale detector fires on the 3rd, and the synthesis call is the
			// 4th (plain summary).
			if callCount <= toolCallStaleThreshold {
				ch <- &model.StreamChunk{
					Delta:  `{"next_action":"execute","action_calls":[{"name":"noop","params":{}}]}`,
					IsDone: true,
				}
			} else {
				ch <- &model.StreamChunk{Delta: "stuck summary", IsDone: true}
			}
			close(ch)
			return ch, nil
		},
	}

	agent := New(sess, actExt, mockReq, WithMaxRounds(1))
	// Run with no per-call opts.
	result, err := agent.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Expected no error for stuck loop, got %v", err)
	}
	// Stuck loop: 3 identical execute rounds then synthesis → 4 model calls.
	if callCount != toolCallStaleThreshold+1 {
		t.Errorf("Expected %d LLM calls (3 stale + 1 synthesis), got %d", toolCallStaleThreshold+1, callCount)
	}
	if result != "stuck summary" {
		t.Errorf("Expected synthesis summary, got %q", result)
	}
}

// TestAgent_RunMaxRoundsOverrideFromRunOpt verifies that a per-call
// WithMaxRounds still overrides the Agent's persisted maxRounds. Like
// TestAgent_PersistsMaxRoundsFromNew, a stuck (always-identical-execute)
// model is now bounded by stale→synthesis rather than erroring.
func TestAgent_RunMaxRoundsOverrideFromRunOpt(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()

	callCount := 0
	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			callCount++
			ch := make(chan *model.StreamChunk, 1)
			if callCount <= toolCallStaleThreshold {
				ch <- &model.StreamChunk{
					Delta:  `{"next_action":"execute","action_calls":[{"name":"noop","params":{}}]}`,
					IsDone: true,
				}
			} else {
				ch <- &model.StreamChunk{Delta: "stuck summary", IsDone: true}
			}
			close(ch)
			return ch, nil
		},
	}

	agent := New(sess, actExt, mockReq, WithMaxRounds(5))
	result, err := agent.Run(context.Background(), "test", WithMaxRounds(1))
	if err != nil {
		t.Fatalf("Expected no error for stuck loop, got %v", err)
	}
	// Stuck loop termination is identical regardless of maxRounds override.
	if callCount != toolCallStaleThreshold+1 {
		t.Errorf("Expected %d LLM calls (3 stale + 1 synthesis), got %d", toolCallStaleThreshold+1, callCount)
	}
	if result != "stuck summary" {
		t.Errorf("Expected synthesis summary, got %q", result)
	}
}
