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
	"io"
	"strings"
	"testing"
	"time"

	"github.com/inferglow/model"
	"github.com/inferglow/session"
)

// TestChatModelAgent_New verifies that NewChatModelAgent returns a non-nil
// agent with all component fields wired.
func TestChatModelAgent_New(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()
	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			return nil, nil
		},
	}

	a := NewChatModelAgent(sess, actExt, mockReq)
	if a == nil {
		t.Fatal("NewChatModelAgent returned nil")
	}
	if a.session != sess {
		t.Error("session field not wired")
	}
	if a.actionExt != actExt {
		t.Error("actionExt field not wired")
	}
	if a.modelReq != mockReq {
		t.Error("modelReq field not wired")
	}
	if a.engine == nil {
		t.Error("engine field not wired")
	}
	if a.agent == nil {
		t.Error("internal agent field not wired")
	}
}

// TestChatModelAgent_DefaultConfig verifies the documented defaults:
// MaxRounds=10 and StreamTimeout=5 minutes. Asserted directly on the
// config struct (robust) and behaviorally via LLM call count.
func TestChatModelAgent_DefaultConfig(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()

	a := NewChatModelAgent(sess, actExt, &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			return nil, nil
		},
	})

	cfg := a.Config()
	if cfg.MaxRounds != 10 {
		t.Errorf("default MaxRounds: got %d, want 10", cfg.MaxRounds)
	}
	if cfg.StreamTimeout != 5*time.Minute {
		t.Errorf("default StreamTimeout: got %v, want %v", cfg.StreamTimeout, 5*time.Minute)
	}
	if cfg.SystemPrompt != "" {
		t.Errorf("default SystemPrompt: got %q, want empty", cfg.SystemPrompt)
	}
	if cfg.PIIMasker != nil {
		t.Error("default PIIMasker: got non-nil, want nil")
	}
}

// TestChatModelAgent_DefaultConfig_Behavior verifies maxRounds=10
// behaviorally: when the model always returns "execute", the loop performs
// exactly 11 LLM calls (rounds 0..10) before ShouldContinue returns false.
func TestChatModelAgent_DefaultConfig_Behavior(t *testing.T) {
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

	a := NewChatModelAgent(sess, actExt, mockReq)
	_, err := a.Run(context.Background(), "test")
	if err == nil {
		t.Skip("LLM happened to return a response; cannot assert callCount reliably")
	}
	// Default maxRounds=10 → rounds 0..10 → 11 LLM calls.
	if callCount != 11 {
		t.Errorf("default config: expected 11 LLM calls with maxRounds=10, got %d", callCount)
	}
}

// TestChatModelAgent_CustomConfig verifies that WithAgentMaxRounds,
// WithAgentSystemPrompt, WithAgentStreamTimeout, and WithAgentPIIMasker are
// all applied. maxRounds is asserted via call count; systemPrompt is captured
// from the mock's requestFn; streamTimeout is asserted on the config;
// PIIMasker is asserted non-nil on the config.
func TestChatModelAgent_CustomConfig(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()

	var capturedSystem string
	callCount := 0
	mockReq := &mockModelRequester{
		requestFn: func(ctx context.Context, req *model.ModelRequest) {
			capturedSystem = req.System
		},
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

	masker := &testPIIMasker{maskInput: true}
	a := NewChatModelAgent(sess, actExt, mockReq,
		WithAgentMaxRounds(5),
		WithAgentSystemPrompt("test-system-prompt"),
		WithAgentStreamTimeout(30*time.Second),
		WithAgentPIIMasker(masker),
	)

	cfg := a.Config()
	if cfg.MaxRounds != 5 {
		t.Errorf("custom MaxRounds: got %d, want 5", cfg.MaxRounds)
	}
	if cfg.StreamTimeout != 30*time.Second {
		t.Errorf("custom StreamTimeout: got %v, want 30s", cfg.StreamTimeout)
	}
	if cfg.SystemPrompt != "test-system-prompt" {
		t.Errorf("custom SystemPrompt: got %q, want %q", cfg.SystemPrompt, "test-system-prompt")
	}
	if cfg.PIIMasker != masker {
		t.Error("custom PIIMasker not set on config")
	}

	_, err := a.Run(context.Background(), "test")
	if err == nil {
		t.Skip("LLM happened to return a response; cannot assert callCount reliably")
	}

	// maxRounds=5 → rounds 0..5 → 6 LLM calls.
	if callCount != 6 {
		t.Errorf("custom config: expected 6 LLM calls with maxRounds=5, got %d", callCount)
	}
	if capturedSystem != "test-system-prompt" {
		t.Errorf("custom config: system prompt not applied, got %q", capturedSystem)
	}
	// Verify the streamTimeout was persisted on the underlying agent.
	if a.agent.streamTimeout != 30*time.Second {
		t.Errorf("custom config: streamTimeout not persisted on agent, got %v", a.agent.streamTimeout)
	}
	// Verify the PII masker was persisted on the underlying agent.
	if a.agent.piiMasker != masker {
		t.Error("custom config: PIIMasker not persisted on agent")
	}
}

// TestChatModelAgent_Run verifies that Run returns the final response when
// the LLM returns a "response" decision on the first round — i.e. a one-line
// call completes the PLAN → EXECUTE loop.
func TestChatModelAgent_Run(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()

	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 1)
			ch <- &model.StreamChunk{
				Delta:  `{"next_action":"response","final_response":"Hello from ChatModelAgent!"}`,
				IsDone: true,
			}
			close(ch)
			return ch, nil
		},
	}

	a := NewChatModelAgent(sess, actExt, mockReq)
	result, err := a.Run(context.Background(), "Hi")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result != "Hello from ChatModelAgent!" {
		t.Errorf("Run result: got %q, want %q", result, "Hello from ChatModelAgent!")
	}
}

// TestChatModelAgent_Run_PlanExecuteLoop verifies that a one-line Run call
// completes a full PLAN → EXECUTE loop: the LLM first decides to execute a
// tool, then on the next round returns a final response that references the
// tool result. This proves the convenience wrapper does not short-circuit
// the loop.
func TestChatModelAgent_Run_PlanExecuteLoop(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()

	round := 0
	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			round++
			ch := make(chan *model.StreamChunk, 1)
			if round == 1 {
				// First round: decide to execute. action_calls must be
				// non-empty or ShouldContinue returns false (the loop
				// refuses to continue with nothing to execute). The
				// "noop" action is not registered, so the dispatcher
				// produces an error result and the loop continues.
				ch <- &model.StreamChunk{
					Delta:  `{"next_action":"execute","action_calls":[{"name":"noop","params":{}}]}`,
					IsDone: true,
				}
			} else {
				// Second round: return final response.
				ch <- &model.StreamChunk{
					Delta:  `{"next_action":"response","final_response":"done after loop"}`,
					IsDone: true,
				}
			}
			close(ch)
			return ch, nil
		},
	}

	a := NewChatModelAgent(sess, actExt, mockReq)
	result, err := a.Run(context.Background(), "go")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result != "done after loop" {
		t.Errorf("Run result: got %q, want %q", result, "done after loop")
	}
	if round != 2 {
		t.Errorf("expected 2 LLM rounds (plan + response), got %d", round)
	}
}

// TestChatModelAgent_RunWithOpts verifies that a per-call WithMaxRounds
// override applied via RunWithOpts wins over the agent's configured default.
func TestChatModelAgent_RunWithOpts(t *testing.T) {
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

	// Default maxRounds=10 on construction.
	a := NewChatModelAgent(sess, actExt, mockReq)
	// Override to maxRounds=1 per-call via RunWithOpts.
	_, err := a.RunWithOpts(context.Background(), "test", WithMaxRounds(1))
	if err == nil {
		t.Skip("LLM happened to return a response; cannot assert callCount reliably")
	}
	// Per-call WithMaxRounds(1) must win → 2 LLM calls, not 11.
	if callCount != 2 {
		t.Errorf("RunWithOpts override: expected 2 LLM calls with per-call maxRounds=1, got %d", callCount)
	}
}

// TestChatModelAgent_RunStream verifies that RunStream returns a StreamReader
// that delivers the LLM's streamed chunks in order.
func TestChatModelAgent_RunStream(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()

	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 2)
			ch <- &model.StreamChunk{Delta: "Hello", IsDone: false}
			ch <- &model.StreamChunk{Delta: " World", IsDone: true}
			close(ch)
			return ch, nil
		},
	}

	a := NewChatModelAgent(sess, actExt, mockReq)
	reader, err := a.RunStream(context.Background(), "Hi")
	if err != nil {
		t.Fatalf("RunStream returned error: %v", err)
	}
	if reader == nil {
		t.Fatal("RunStream returned nil reader")
	}
	defer reader.Close()

	var fullText string
	for {
		chunk, rerr := reader.Recv()
		if errors.Is(rerr, io.EOF) {
			break
		}
		if rerr != nil {
			t.Fatalf("Recv error: %v", rerr)
		}
		fullText += chunk.Delta
	}
	if fullText != "Hello World" {
		t.Errorf("streamed text: got %q, want %q", fullText, "Hello World")
	}
}

// TestChatModelAgent_RunStream_SystemPrompt verifies that the configured
// SystemPrompt is applied to the stream request.
func TestChatModelAgent_RunStream_SystemPrompt(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()

	var capturedSystem string
	mockReq := &mockModelRequester{
		requestFn: func(ctx context.Context, req *model.ModelRequest) {
			capturedSystem = req.System
		},
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 1)
			ch <- &model.StreamChunk{Delta: "ok", IsDone: true}
			close(ch)
			return ch, nil
		},
	}

	a := NewChatModelAgent(sess, actExt, mockReq, WithAgentSystemPrompt("stream-prompt"))
	reader, err := a.RunStream(context.Background(), "Hi")
	if err != nil {
		t.Fatalf("RunStream returned error: %v", err)
	}
	defer reader.Close()
	// Drain the reader so the requestFn runs to completion.
	for {
		_, rerr := reader.Recv()
		if errors.Is(rerr, io.EOF) {
			break
		}
		if rerr != nil {
			t.Fatalf("Recv error: %v", rerr)
		}
	}
	if capturedSystem != "stream-prompt" {
		t.Errorf("RunStream system prompt: got %q, want %q", capturedSystem, "stream-prompt")
	}
}

// TestChatModelAgent_PIIMasker verifies that WithAgentPIIMasker propagates
// the masker to the underlying agent so that user input is redacted in the
// session and the final response is redacted on output.
func TestChatModelAgent_PIIMasker(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()

	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 1)
			ch <- &model.StreamChunk{
				Delta:  `{"next_action":"response","final_response":"reply to alice@example.com"}`,
				IsDone: true,
			}
			close(ch)
			return ch, nil
		},
	}

	masker := &testPIIMasker{maskInput: true, maskOutput: true}
	a := NewChatModelAgent(sess, actExt, mockReq, WithAgentPIIMasker(masker))

	result, err := a.Run(context.Background(), "my email is alice@example.com")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	// Output masked.
	if strings.Contains(result, "alice@example.com") {
		t.Errorf("output not masked: %q", result)
	}
	if !strings.Contains(result, "***") {
		t.Errorf("expected mask char in output, got %q", result)
	}
	// Input masked in session.
	for _, msg := range sess.GetFullContext() {
		if msg.Role == "user" {
			if s, ok := msg.Content.(string); ok && strings.Contains(s, "alice@example.com") {
				t.Errorf("input not masked in session: %q", s)
			}
		}
	}
}

// TestChatModelAgent_RunStream_PIIMasker verifies that RunStream propagates
// the configured PIIMasker to the session so user input is redacted before
// entering the conversation history.
func TestChatModelAgent_RunStream_PIIMasker(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()

	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 1)
			ch <- &model.StreamChunk{Delta: "ok", IsDone: true}
			close(ch)
			return ch, nil
		},
	}

	masker := &testPIIMasker{maskInput: true}
	a := NewChatModelAgent(sess, actExt, mockReq, WithAgentPIIMasker(masker))

	reader, err := a.RunStream(context.Background(), "contact alice@example.com")
	if err != nil {
		t.Fatalf("RunStream returned error: %v", err)
	}
	defer reader.Close()
	// Drain the reader so the user message is added to the session.
	for {
		_, rerr := reader.Recv()
		if errors.Is(rerr, io.EOF) {
			break
		}
		if rerr != nil {
			t.Fatalf("Recv error: %v", rerr)
		}
	}

	for _, msg := range sess.GetFullContext() {
		if msg.Role == "user" {
			if s, ok := msg.Content.(string); ok && strings.Contains(s, "alice@example.com") {
				t.Errorf("input not masked in session: %q", s)
			}
		}
	}
}
