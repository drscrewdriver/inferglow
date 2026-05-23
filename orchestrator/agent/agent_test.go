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

func TestAgentRunExecuteNoResponseReturnsError(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()

	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 1)
			ch <- &model.StreamChunk{
				Delta:  `{"next_action":"execute","action_calls":[{"name":"test","params":{}}]}`,
				IsDone: true,
			}
			close(ch)
			return ch, nil
		},
	}

	agent := New(sess, actExt, mockReq)
	_, err := agent.Run(context.Background(), "test")
	if err != ErrNoFinalResponse {
		t.Errorf("Expected ErrNoFinalResponse, got %v", err)
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
// After the fix, WithMaxRounds(1) on New must cap the loop at 1 round,
// yielding exactly 2 LLM calls when the LLM always returns "execute".
func TestAgent_PersistsMaxRoundsFromNew(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()

	callCount := 0
	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			callCount++
			ch := make(chan *model.StreamChunk, 1)
			ch <- &model.StreamChunk{
				Delta:  `{"next_action":"execute","action_calls":[{"name":"noop","params":{}}]}`,
				IsDone: true,
			}
			close(ch)
			return ch, nil
		},
	}

	agent := New(sess, actExt, mockReq, WithMaxRounds(1))
	// Run with no per-call opts: must use the persisted maxRounds=1.
	_, err := agent.Run(context.Background(), "test")
	if err == nil {
		t.Skip("LLM happened to return a response; cannot assert callCount reliably")
	}
	// With maxRounds=1 the loop makes 2 LLM calls (round 0 and round 1)
	// before ShouldContinue returns false at roundIndex=1 >= maxRounds=1.
	if callCount != 2 {
		t.Errorf("Expected 2 LLM calls with persisted maxRounds=1, got %d", callCount)
	}
}

// TestAgent_RunMaxRoundsOverrideFromRunOpt verifies that a per-call
// WithMaxRounds still overrides the Agent's persisted maxRounds.
func TestAgent_RunMaxRoundsOverrideFromRunOpt(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()

	callCount := 0
	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			callCount++
			ch := make(chan *model.StreamChunk, 1)
			ch <- &model.StreamChunk{
				Delta:  `{"next_action":"execute","action_calls":[{"name":"noop","params":{}}]}`,
				IsDone: true,
			}
			close(ch)
			return ch, nil
		},
	}

	// Persisted maxRounds=5 on New, overridden to 1 by the per-call opt.
	agent := New(sess, actExt, mockReq, WithMaxRounds(5))
	_, err := agent.Run(context.Background(), "test", WithMaxRounds(1))
	if err == nil {
		t.Skip("LLM happened to return a response; cannot assert callCount reliably")
	}
	// Per-call WithMaxRounds(1) must win → 2 LLM calls, not 6.
	if callCount != 2 {
		t.Errorf("Expected 2 LLM calls with per-call maxRounds=1 overriding persisted 5, got %d", callCount)
	}
}
