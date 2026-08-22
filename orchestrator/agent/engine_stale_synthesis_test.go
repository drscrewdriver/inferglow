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
	"strings"
	"testing"

	"github.com/inferglow/action"
	"github.com/inferglow/model"
	"github.com/inferglow/session"
)

// TestEngine_StaleDetectedReturnsEmptyExecuteDecision verifies the LG-1
// "card dead → synthesis" contract at the executeLoop level: when the model
// calls the same tool with the same arguments toolCallStaleThreshold (3)
// times in a row, executeLoop returns an empty execute decision
// (NextAction="execute", FinalResponse="") instead of raising
// ErrLoopDetected, and it does so well before maxToolCallRounds.
//
// With cap=20 and always-identical execute(noop):
//   - call 1: stale=1, tool executed, toolCallRounds=1
//   - call 2: stale=2, tool executed, toolCallRounds=2
//   - call 3: stale=3 → recognized as stuck → empty execute decision (no exec)
//
// So callCount must be 3 (<< 20) and the loop must end with an empty execute
// decision that RunLoop/Agent.Run turns into a synthesis summary.
func TestEngine_StaleDetectedReturnsEmptyExecuteDecision(t *testing.T) {
	sess := NewSessionExtension(session.NewSession("test", 10000))

	actionInst, _ := action.New("noop", "noop", func(ctx context.Context, input map[string]any) (any, error) {
		return nil, nil
	})
	actExt := NewActionExtension()
	if err := actExt.Register(actionInst); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	callCount := 0
	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 1)
			callCount++
			ch <- &model.StreamChunk{
				Delta:  `{"next_action":"execute","action_calls":[{"name":"noop","params":{}}]}`,
				IsDone: true,
			}
			close(ch)
			return ch, nil
		},
	}

	// No loopGuard: the stale detector is the only stuck-loop protection.
	engine := &Engine{
		session:           sess,
		actionExt:         actExt,
		modelReq:          mockReq,
		maxToolCallRounds: 20, // large so the stale path fires well before cap
	}

	decision, err := engine.executeLoop(context.Background(), "compute", 10, "")
	if err != nil {
		t.Fatalf("executeLoop returned error: %v", err)
	}
	if decision == nil {
		t.Fatal("expected non-nil decision")
	}
	// LG-1: must be the empty execute decision that triggers synthesis, not
	// a response and not an ErrLoopDetected error.
	if decision.NextAction != "execute" {
		t.Errorf("Expected NextAction=execute (synthesis trigger), got %q", decision.NextAction)
	}
	if decision.FinalResponse != "" {
		t.Errorf("Expected empty FinalResponse (RunLoop will synth), got %q", decision.FinalResponse)
	}
	// Must fire on the 3rd identical call, well before maxToolCallRounds=20.
	if callCount != toolCallStaleThreshold {
		t.Errorf("Expected stale detection on call %d, got %d calls", toolCallStaleThreshold, callCount)
	}
}

// TestEngine_StaleStillProducesSynthesisThroughAgentRun verifies the full
// Agent.Run path: a stuck model that keeps calling the same tool ends with a
// synthesis summary, not an error, and without waiting for maxToolCallRounds.
func TestEngine_StaleStillProducesSynthesisThroughAgentRun(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()

	callCount := 0
	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 1)
			callCount++
			// Identical execute for the first 3 calls (stale trips on call 3);
			// the synthesis call is the 4th and must return a plain summary.
			if callCount <= toolCallStaleThreshold {
				ch <- &model.StreamChunk{
					Delta:  `{"next_action":"execute","action_calls":[{"name":"test","params":{}}]}`,
					IsDone: true,
				}
			} else {
				ch <- &model.StreamChunk{Delta: "stuck-loop summary", IsDone: true}
			}
			close(ch)
			return ch, nil
		},
	}

	agent := New(sess, actExt, mockReq)
	// Large cap so the stale path (not the cap) forces the synthesis.
	agent.engine.maxToolCallRounds = 30

	result, err := agent.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if result != "stuck-loop summary" {
		t.Errorf("Expected synthesis summary, got %q", result)
	}
	// 3 identical execute + 1 synthesis call; must not reach the 30-round cap.
	if callCount != toolCallStaleThreshold+1 {
		t.Errorf("Expected %d model calls, got %d", toolCallStaleThreshold+1, callCount)
	}
}

// TestEngine_LoopGuardStillBreaksIsolation verifies LG-2: when a loopGuard
// is configured, its VerdictBreak still surfaces as ErrLoopDetected (the
// policy-level hard stop is independent of the stale→synthesis path). This
// mirrors TestEngine_LoopGuardBreaksOnRepeat but is kept here to pin the
// interplay between the two stuck-loop defenses.
func TestEngine_LoopGuardStillBreaksIsolation(t *testing.T) {
	sess := NewSessionExtension(session.NewSession("test", 10000))

	actionInst, _ := action.New("calc", "calc", func(ctx context.Context, input map[string]any) (any, error) {
		return 1, nil
	})
	actExt := NewActionExtension()
	if err := actExt.Register(actionInst); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	mockReq := &scriptedModelRequester{
		responses: []string{
			`{"next_action":"execute","action_calls":[{"name":"calc","params":{}}]}`,
			`{"next_action":"execute","action_calls":[{"name":"calc","params":{}}]}`,
			`{"next_action":"response","final_response":"should-not-reach"}`,
		},
	}

	// RepeatActionWindow=2 makes the guard break before stale can synthesize.
	guard := NewLoopGuard(LoopGuardConfig{RepeatActionWindow: 2})
	engine := NewEngineWithLoopGuard(sess, actExt, mockReq, guard)

	_, err := engine.executeLoop(context.Background(), "compute", 10, "")
	if err == nil {
		t.Fatalf("Expected ErrLoopDetected from loopGuard, got nil")
	}
	if !errors.Is(err, ErrLoopDetected) {
		t.Errorf("Expected errors.Is(err, ErrLoopDetected)=true, got false (err: %v)", err)
	}
	if !strings.Contains(err.Error(), ErrLoopDetected.Error()) {
		t.Errorf("Expected error message to contain %q, got %q", ErrLoopDetected.Error(), err.Error())
	}
}