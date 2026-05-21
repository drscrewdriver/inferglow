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

package main

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/inferglow/action"
	"github.com/inferglow/audit"
	"github.com/inferglow/model"
	"github.com/inferglow/orchestrator/actionruntime"
	"github.com/inferglow/session"
)

// scriptedModelRequester returns pre-scripted LLM responses in order.
// Each RequestModel call emits the next script entry as a single
// StreamChunk. An optional failErrs slice (indexed by call number) makes
// the corresponding call return an error instead, simulating transient
// network failures for retry tests.
type scriptedModelRequester struct {
	mu        sync.Mutex
	responses []string
	failErrs  []error
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
	if idx < len(m.failErrs) && m.failErrs[idx] != nil {
		return nil, m.failErrs[idx]
	}
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

func (m *scriptedModelRequester) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

var _ model.ModelRequester = (*scriptedModelRequester)(nil)

// newCalculatorAction builds a real Action that evaluates arithmetic
// expressions using go/ast. Supported operators: + - * / and parentheses.
func newCalculatorAction() *action.Action {
	a, _ := action.New("calculator", "Evaluate a mathematical expression.",
		func(ctx context.Context, input map[string]any) (any, error) {
			expr, _ := input["expression"].(string)
			return evalArith(expr)
		})
	return a
}

func evalArith(expr string) (float64, error) {
	if expr == "" {
		return 0, errors.New("calculator: empty expression")
	}
	node, err := parser.ParseExpr(expr)
	if err != nil {
		return 0, fmt.Errorf("calculator: parse error: %w", err)
	}
	return walkArith(node)
}

func walkArith(node ast.Node) (float64, error) {
	switch n := node.(type) {
	case *ast.BasicLit:
		v, err := strconv.ParseFloat(n.Value, 64)
		if err != nil {
			return 0, fmt.Errorf("calculator: invalid literal %q: %w", n.Value, err)
		}
		return v, nil
	case *ast.ParenExpr:
		return walkArith(n.X)
	case *ast.UnaryExpr:
		v, err := walkArith(n.X)
		if err != nil {
			return 0, err
		}
		if n.Op == token.SUB {
			return -v, nil
		}
		return v, nil
	case *ast.BinaryExpr:
		lhs, err := walkArith(n.X)
		if err != nil {
			return 0, err
		}
		rhs, err := walkArith(n.Y)
		if err != nil {
			return 0, err
		}
		switch n.Op {
		case token.ADD:
			return lhs + rhs, nil
		case token.SUB:
			return lhs - rhs, nil
		case token.MUL:
			return lhs * rhs, nil
		case token.QUO:
			if rhs == 0 {
				return 0, errors.New("calculator: division by zero")
			}
			return lhs / rhs, nil
		default:
			return 0, fmt.Errorf("calculator: unsupported operator %q", n.Op)
		}
	default:
		return 0, fmt.Errorf("calculator: unsupported node %T", node)
	}
}

// toModelChatMessages converts session.ChatMessage to model.ChatMessage.
func toModelChatMessages(msgs []session.ChatMessage) []model.ChatMessage {
	out := make([]model.ChatMessage, len(msgs))
	for i, m := range msgs {
		content, _ := m.Content.(string)
		out[i] = model.ChatMessage{Role: m.Role, Content: content, Name: m.Name}
	}
	return out
}

// runCrossModuleLoop drives the LLM → Action → LLM loop using only
// exported APIs. It mirrors agent.Engine.executeLoop but lives in the
// test so we can inject a real AuditChain (the public Agent API does
// not expose audit-hook injection). Decision audit entries are appended
// here; action audit entries are appended by the ActionDispatcher.
func runCrossModuleLoop(ctx context.Context, mr model.ModelRequester, dispatcher *actionruntime.ActionDispatcher, hook audit.AuditHook, sess *session.Session, userMessage string, maxRounds int) (string, error) {
	sess.AddMessage("user", userMessage, "")

	round := 0
	for {
		req := &model.ModelRequest{
			System:      "You are a helpful assistant.",
			ChatHistory: toModelChatMessages(sess.PreparePrompt()),
		}

		data, err := mr.GenerateRequestData(ctx, req)
		if err != nil {
			return "", err
		}

		stream, err := mr.RequestModel(ctx, data)
		if err != nil {
			return "", err
		}

		var content strings.Builder
		for chunk := range stream {
			content.WriteString(chunk.Delta)
		}

		decision, err := actionruntime.ParseDecision(content.String())
		if err != nil {
			return "", err
		}

		if hook != nil && hook.IsEnabled() {
			_, _ = hook.Append(&audit.AuditEntry{
				Timestamp: time.Now(),
				Source:    "agent",
				Action:    "decision",
				Input:     userMessage,
				Output:    decision,
				Metadata:  map[string]string{"round": strconv.Itoa(round)},
			})
		}

		if decision.NextAction != "execute" || len(decision.ActionCalls) == 0 || round >= maxRounds {
			if decision.NextAction == "response" {
				sess.AddMessage("assistant", decision.FinalResponse, "")
			}
			return decision.FinalResponse, nil
		}

		results := dispatcher.Execute(ctx, decision.ActionCalls)

		for i, call := range decision.ActionCalls {
			if i < len(results) {
				r := results[i]
				msg := fmt.Sprintf("Action %q result: %v", call.Name, r.Result)
				if !r.OK {
					msg = fmt.Sprintf("Action %q failed: %s", call.Name, r.Error)
				}
				sess.AddMessage("system", msg, "")
			}
		}

		round++
	}
}

// TestCrossModuleIntegration_NormalFlow verifies the complete chain:
// user input → mock LLM returns action_calls → ActionDispatcher executes
// the calculator → result fed back → mock LLM returns final response.
func TestCrossModuleIntegration_NormalFlow(t *testing.T) {
	sess := session.NewSession("cross-module-normal", 10000)

	reg := action.NewRegistry()
	if err := reg.Register(newCalculatorAction()); err != nil {
		t.Fatalf("Register calculator: %v", err)
	}
	dispatcher := actionruntime.NewActionDispatcher(reg)

	mockReq := &scriptedModelRequester{
		responses: []string{
			`{"next_action":"execute","action_calls":[{"name":"calculator","params":{"expression":"1+2"}}]}`,
			`{"next_action":"response","final_response":"The result of 1+2 is 3"}`,
		},
	}

	result, err := runCrossModuleLoop(context.Background(), mockReq, dispatcher, nil, sess, "What is 1+2?", 5)
	if err != nil {
		t.Fatalf("runCrossModuleLoop error: %v", err)
	}

	if !strings.Contains(result, "3") {
		t.Errorf("Final response should contain calculator result '3', got %q", result)
	}

	if mockReq.CallCount() != 2 {
		t.Errorf("Expected 2 LLM calls, got %d", mockReq.CallCount())
	}

	history := sess.GetFullContext()
	hasUserMsg := false
	hasActionResult := false
	hasAssistantMsg := false
	for _, m := range history {
		content := fmt.Sprintf("%v", m.Content)
		if m.Role == "user" && strings.Contains(content, "1+2") {
			hasUserMsg = true
		}
		if m.Role == "system" && strings.Contains(content, "calculator") {
			hasActionResult = true
		}
		if m.Role == "assistant" && strings.Contains(content, "3") {
			hasAssistantMsg = true
		}
	}
	if !hasUserMsg {
		t.Error("Session should contain the user message")
	}
	if !hasActionResult {
		t.Error("Session should contain the calculator action result")
	}
	if !hasAssistantMsg {
		t.Error("Session should contain the assistant's final response")
	}
}

// TestCrossModuleIntegration_AuditChain verifies that a real AuditChain
// records the full LLM → Action → LLM → response chain with both
// decision entries (Source="agent") and action entries (Source="action"),
// and that VerifyChain passes (chain hash integrity).
func TestCrossModuleIntegration_AuditChain(t *testing.T) {
	sess := session.NewSession("cross-module-audit", 10000)

	reg := action.NewRegistry()
	if err := reg.Register(newCalculatorAction()); err != nil {
		t.Fatalf("Register calculator: %v", err)
	}

	chain, err := audit.NewAuditChain(audit.AuditConfig{Enabled: true, StorageBackend: "memory"})
	if err != nil {
		t.Fatalf("NewAuditChain: %v", err)
	}
	dispatcher := actionruntime.NewActionDispatcherWithAudit(reg, chain)

	mockReq := &scriptedModelRequester{
		responses: []string{
			`{"next_action":"execute","action_calls":[{"name":"calculator","params":{"expression":"4*5"}}]}`,
			`{"next_action":"response","final_response":"The result of 4*5 is 20"}`,
		},
	}

	result, err := runCrossModuleLoop(context.Background(), mockReq, dispatcher, chain, sess, "What is 4*5?", 5)
	if err != nil {
		t.Fatalf("runCrossModuleLoop error: %v", err)
	}

	if !strings.Contains(result, "20") {
		t.Errorf("Final response should contain '20', got %q", result)
	}

	entries, qErr := chain.Query(audit.QueryFilter{})
	if qErr != nil {
		t.Fatalf("chain.Query: %v", qErr)
	}

	decisionEntries := 0
	actionEntries := 0
	for _, e := range entries {
		switch {
		case e.Source == "agent" && e.Action == "decision":
			decisionEntries++
		case e.Source == "action" && e.Action == "execute":
			actionEntries++
		}
	}
	if decisionEntries < 2 {
		t.Errorf("Expected >=2 decision audit entries (one per LLM call), got %d", decisionEntries)
	}
	if actionEntries < 1 {
		t.Errorf("Expected >=1 action audit entry, got %d", actionEntries)
	}

	actionEntry := findActionEntry(entries)
	if actionEntry == nil {
		t.Fatal("Expected an action audit entry but found none")
	}
	if actionEntry.Metadata == nil {
		t.Fatal("Action audit entry should have metadata")
	}
	if actionEntry.Metadata["action_name"] != "calculator" {
		t.Errorf("action_name metadata = %q, want %q", actionEntry.Metadata["action_name"], "calculator")
	}
	if actionEntry.Error != "" {
		t.Errorf("Action audit entry should have no error, got %q", actionEntry.Error)
	}

	if err := chain.VerifyChain(); err != nil {
		t.Errorf("AuditChain.VerifyChain failed: %v", err)
	}

	if chain.Len() != len(entries) {
		t.Errorf("Len() = %d, want %d", chain.Len(), len(entries))
	}
}

func findActionEntry(entries []*audit.AuditEntry) *audit.AuditEntry {
	for _, e := range entries {
		if e.Source == "action" && e.Action == "execute" {
			return e
		}
	}
	return nil
}

// TestCrossModuleIntegration_ActionFailure verifies that when an Action
// returns an error, the error is fed back to the LLM which then produces
// a final response acknowledging the failure.
func TestCrossModuleIntegration_ActionFailure(t *testing.T) {
	sess := session.NewSession("cross-module-action-fail", 10000)

	reg := action.NewRegistry()
	if err := reg.Register(newCalculatorAction()); err != nil {
		t.Fatalf("Register calculator: %v", err)
	}
	dispatcher := actionruntime.NewActionDispatcher(reg)

	mockReq := &scriptedModelRequester{
		responses: []string{
			`{"next_action":"execute","action_calls":[{"name":"calculator","params":{"expression":"1/0"}}]}`,
			`{"next_action":"response","final_response":"The calculator reported a division by zero error"}`,
		},
	}

	result, err := runCrossModuleLoop(context.Background(), mockReq, dispatcher, nil, sess, "What is 1/0?", 5)
	if err != nil {
		t.Fatalf("runCrossModuleLoop error: %v", err)
	}

	if !strings.Contains(strings.ToLower(result), "error") && !strings.Contains(strings.ToLower(result), "division") {
		t.Errorf("Final response should mention the error, got %q", result)
	}

	history := sess.GetFullContext()
	hasFailureMsg := false
	for _, m := range history {
		content := fmt.Sprintf("%v", m.Content)
		if m.Role == "system" && strings.Contains(strings.ToLower(content), "failed") {
			hasFailureMsg = true
			break
		}
	}
	if !hasFailureMsg {
		t.Error("Session should contain a system message recording the action failure")
	}
}

// TestCrossModuleIntegration_LLMRetry verifies that model.AttemptRunner
// retries a failed LLM call (simulated network error) and the loop
// ultimately succeeds. The first RequestModel call returns a 503-style
// error (retryable); the second succeeds with a direct response.
func TestCrossModuleIntegration_LLMRetry(t *testing.T) {
	sess := session.NewSession("cross-module-llm-retry", 10000)

	reg := action.NewRegistry()
	dispatcher := actionruntime.NewActionDispatcher(reg)

	inner := &scriptedModelRequester{
		responses: []string{
			"",
			`{"next_action":"response","final_response":"Recovered after retry"}`,
		},
		failErrs: []error{
			errors.New("API error (status 503): service unavailable"),
		},
	}

	runner := model.NewAttemptRunner()
	runner.MaxAttempts = 3
	runner.BackoffBase = 1 * time.Millisecond
	runner.BackoffMax = 5 * time.Millisecond

	retryReq := &retryModelRequester{inner: inner, runner: runner}

	result, err := runCrossModuleLoop(context.Background(), retryReq, dispatcher, nil, sess, "Hi", 5)
	if err != nil {
		t.Fatalf("runCrossModuleLoop error: %v", err)
	}

	if result != "Recovered after retry" {
		t.Errorf("Final response = %q, want %q", result, "Recovered after retry")
	}

	if inner.CallCount() != 2 {
		t.Errorf("Expected 2 LLM calls (1 failed + 1 retried), got %d", inner.CallCount())
	}
}

// retryModelRequester wraps a scriptedModelRequester and routes
// RequestModel through an AttemptRunner so transient failures are
// retried transparently.
type retryModelRequester struct {
	inner  *scriptedModelRequester
	runner *model.AttemptRunner
}

func (r *retryModelRequester) Name() string { return "retry-wrapper" }

func (r *retryModelRequester) GenerateRequestData(ctx context.Context, req *model.ModelRequest) (*model.RequestData, error) {
	return r.inner.GenerateRequestData(ctx, req)
}

func (r *retryModelRequester) RequestModel(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
	return r.runner.Run(ctx, func(ctx context.Context) (<-chan *model.StreamChunk, error) {
		return r.inner.RequestModel(ctx, data)
	})
}

func (r *retryModelRequester) BroadcastResponse(ctx context.Context, stream <-chan *model.StreamChunk) (<-chan *model.ResultEvent, error) {
	return nil, nil
}

var _ model.ModelRequester = (*retryModelRequester)(nil)
