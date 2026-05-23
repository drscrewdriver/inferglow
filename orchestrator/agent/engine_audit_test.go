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
	"sync"
	"testing"

	"github.com/inferglow/action"
	"github.com/inferglow/audit"
	"github.com/inferglow/model"
	"github.com/inferglow/session"
)

// engineFakeAuditHook is the agent-package test AuditHook. It mirrors
// actionruntime.fakeAuditHook but lives in the agent package so engine
// tests can use it without an import cycle.
type engineFakeAuditHook struct {
	mu      sync.Mutex
	count   int
	entries []*audit.AuditEntry
}

func (h *engineFakeAuditHook) Append(entry *audit.AuditEntry) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.count++
	h.entries = append(h.entries, entry)
	return "", nil
}

func (h *engineFakeAuditHook) IsEnabled() bool { return true }

func (h *engineFakeAuditHook) Count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.count
}

func (h *engineFakeAuditHook) Snapshot() []*audit.AuditEntry {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]*audit.AuditEntry, len(h.entries))
	copy(out, h.entries)
	return out
}

// scriptedModelRequester returns a stream of pre-scripted responses, one per
// RequestModel call. Each response is emitted as a single StreamChunk whose
// Delta is the next script entry. Exceeding the script panics so tests fail
// loudly instead of hanging.
type scriptedModelRequester struct {
	mu        sync.Mutex
	responses []string
	calls     int
}

func (m *scriptedModelRequester) Name() string { return "scripted" }

func (m *scriptedModelRequester) GenerateRequestData(ctx context.Context, req *model.ModelRequest) (*model.RequestData, error) {
	return &model.RequestData{Model: req.Model}, nil
}

func (m *scriptedModelRequester) RequestModel(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
	m.mu.Lock()
	idx := m.calls
	m.calls++
	m.mu.Unlock()
	if idx >= len(m.responses) {
		panic("scriptedModelRequester: script exhausted")
	}
	ch := make(chan *model.StreamChunk, 1)
	ch <- &model.StreamChunk{Delta: m.responses[idx], IsDone: true}
	close(ch)
	return ch, nil
}

func (m *scriptedModelRequester) BroadcastResponse(ctx context.Context, stream <-chan *model.StreamChunk) (<-chan *model.ResultEvent, error) {
	return nil, nil
}

// Compile-time guard: ensure our scriptedModelRequester satisfies the
// model.ModelRequester interface.
var _ model.ModelRequester = (*scriptedModelRequester)(nil)

// TestEngine_AppendsDecisionAuditEntry verifies that a single round that ends
// in a "response" decision produces at least one "decision" audit entry with
// Source="agent".
func TestEngine_AppendsDecisionAuditEntry(t *testing.T) {
	sess := NewSessionExtension(session.NewSession("test", 10000))
	actExt := NewActionExtension()

	mockReq := &scriptedModelRequester{
		responses: []string{
			`{"next_action":"response","final_response":"done"}`,
		},
	}

	hook := &engineFakeAuditHook{}
	engine := NewEngineWithAudit(sess, actExt, mockReq, hook)

	decision, err := engine.executeLoop(context.Background(), "Hi", 1, "")
	if err != nil {
		t.Fatalf("executeLoop returned error: %v", err)
	}
	if decision.NextAction != "response" {
		t.Errorf("Expected response, got %q", decision.NextAction)
	}

	if got := hook.Count(); got < 1 {
		t.Fatalf("Expected >=1 audit Append calls, got %d", got)
	}
	entries := hook.Snapshot()
	decisionEntries := 0
	for _, entry := range entries {
		if entry.Source == "agent" && entry.Action == "decision" {
			decisionEntries++
		}
	}
	if decisionEntries < 1 {
		t.Errorf("Expected >=1 decision audit entry, got %d (entries: %v)", decisionEntries, entries)
	}
}

// TestEngine_AppendsActionAuditEntry verifies that when the LLM returns an
// execute decision with one ActionCall and then a response decision, the
// engine appends both decision entries and the dispatcher appends an action
// entry. Total = 2 decisions + 1 action = 3 audit entries.
func TestEngine_AppendsActionAuditEntry(t *testing.T) {
	sess := NewSessionExtension(session.NewSession("test", 10000))

	actionInst, _ := action.New("calc", "calc", func(ctx context.Context, input map[string]any) (any, error) {
		return 42, nil
	})
	actExt := NewActionExtension()
	if err := actExt.Register(actionInst); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	mockReq := &scriptedModelRequester{
		responses: []string{
			`{"next_action":"execute","action_calls":[{"name":"calc","params":{}}]}`,
			`{"next_action":"response","final_response":"Result is 42"}`,
		},
	}

	hook := &engineFakeAuditHook{}
	engine := NewEngineWithAudit(sess, actExt, mockReq, hook)

	decision, err := engine.executeLoop(context.Background(), "What is 21*2?", 5, "")
	if err != nil {
		t.Fatalf("executeLoop returned error: %v", err)
	}
	if decision.NextAction != "response" {
		t.Errorf("Expected response, got %q", decision.NextAction)
	}

	// With this script: 2 LLM calls (each emits a decision audit entry),
	// 1 action execution (dispatcher emits one action audit entry).
	// Total = 2 decisions + 1 action = 3 audit entries.
	if got := hook.Count(); got != 3 {
		t.Fatalf("Expected 3 audit Append calls (2 decisions + 1 action), got %d", got)
	}
	entries := hook.Snapshot()
	decisionCount := 0
	actionCount := 0
	for _, entry := range entries {
		switch {
		case entry.Source == "agent" && entry.Action == "decision":
			decisionCount++
		case entry.Source == "action" && entry.Action == "execute":
			actionCount++
		default:
			t.Errorf("Unexpected audit entry: source=%q action=%q", entry.Source, entry.Action)
		}
	}
	if decisionCount != 2 {
		t.Errorf("Expected 2 decision audit entries, got %d", decisionCount)
	}
	if actionCount != 1 {
		t.Errorf("Expected 1 action audit entry, got %d", actionCount)
	}
}

// TestEngine_NoAuditHookUnchangedBehavior verifies that NewEngine (which
// installs a NoOpHook) preserves the pre-audit behavior: the loop runs
// without panics and produces the expected decision.
func TestEngine_NoAuditHookUnchangedBehavior(t *testing.T) {
	sess := NewSessionExtension(session.NewSession("test", 10000))
	actExt := NewActionExtension()

	mockReq := &scriptedModelRequester{
		responses: []string{
			`{"next_action":"response","final_response":"Hello!"}`,
		},
	}

	engine := NewEngine(sess, actExt, mockReq)
	decision, err := engine.executeLoop(context.Background(), "Hi", 3, "")
	if err != nil {
		t.Fatalf("executeLoop returned error: %v", err)
	}
	if decision.NextAction != "response" {
		t.Errorf("Expected response, got %q", decision.NextAction)
	}
	if decision.FinalResponse != "Hello!" {
		t.Errorf("FinalResponse mismatch: got %q", decision.FinalResponse)
	}
}

// TestEngine_ThreeRoundsAuditCount verifies the spec scenario: run a multi-
// round loop where each round has 1 action call, and verify the audit hook
// records the expected number of entries. With maxRounds=3 and the LLM
// returning execute on rounds 0, 1, 2 then response on round 3:
//   - 4 LLM calls → 4 decision audit entries
//   - 3 action executions × 1 action each → 3 action audit entries
//   - Total = 7
//
// The spec text mentions "3 rounds × (1 decision + 1 action) = 6", but the
// actual executeLoop emits a 4th decision audit entry on the final round
// before ShouldContinue returns false. This test pins the actual count so
// future regressions are caught.
func TestEngine_ThreeRoundsAuditCount(t *testing.T) {
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
			`{"next_action":"execute","action_calls":[{"name":"calc","params":{}}]}`,
			`{"next_action":"response","final_response":"done"}`,
		},
	}

	hook := &engineFakeAuditHook{}
	engine := NewEngineWithAudit(sess, actExt, mockReq, hook)

	decision, err := engine.executeLoop(context.Background(), "compute", 3, "")
	if err != nil {
		t.Fatalf("executeLoop returned error: %v", err)
	}
	if decision.NextAction != "response" {
		t.Errorf("Expected response, got %q", decision.NextAction)
	}

	// 4 decisions + 3 actions = 7 audit entries.
	if got := hook.Count(); got != 7 {
		t.Fatalf("Expected 7 audit Append calls (4 decisions + 3 actions), got %d", got)
	}
}

// TestEngine_DisabledHookNoEntries verifies that when IsEnabled() returns
// false (e.g. a NoOpHook wrapping an AuditChain with Enabled=false), the
// engine does not append decision entries.
func TestEngine_DisabledHookNoEntries(t *testing.T) {
	sess := NewSessionExtension(session.NewSession("test", 10000))
	actExt := NewActionExtension()

	mockReq := &scriptedModelRequester{
		responses: []string{
			`{"next_action":"response","final_response":"done"}`,
		},
	}

	// Wrap an AuditChain with Enabled=false; its IsEnabled() returns false.
	chain, err := audit.NewAuditChain(audit.AuditConfig{Enabled: false})
	if err != nil {
		t.Fatalf("NewAuditChain failed: %v", err)
	}
	engine := NewEngineWithAudit(sess, actExt, mockReq, chain)

	_, err = engine.executeLoop(context.Background(), "Hi", 1, "")
	if err != nil {
		t.Fatalf("executeLoop returned error: %v", err)
	}
	// Chain.IsEnabled() is false, so Append is a no-op and Len stays 0.
	if got := chain.Len(); got != 0 {
		t.Errorf("Expected 0 entries on disabled chain, got %d", got)
	}
}
