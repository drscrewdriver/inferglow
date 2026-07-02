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

package flow

import (
	"context"
	"errors"
	"testing"
)

// ---------------------------------------------------------------------------
// AuditHook tests
// ---------------------------------------------------------------------------

func TestAuditHookFrom_NoInject_ReturnsNoop(t *testing.T) {
	ctx := context.Background()
	h := AuditHookFrom(ctx)
	if h == nil {
		t.Fatal("AuditHookFrom should never return nil")
	}
	// noop should not panic
	h.AuditAppend("src", "act", "in", "out")
}

type recordingAuditHook struct {
	calls []string
}

func (r *recordingAuditHook) AuditAppend(source, action string, input, output any) {
	r.calls = append(r.calls, source+":"+action)
}

func TestAuditHookFrom_WithInject_ReturnsInjected(t *testing.T) {
	rec := &recordingAuditHook{}
	ctx := WithAuditHook(context.Background(), rec)
	h := AuditHookFrom(ctx)
	h.AuditAppend("flow", "execute", nil, nil)
	if len(rec.calls) != 1 || rec.calls[0] != "flow:execute" {
		t.Fatalf("expected [flow:execute], got %v", rec.calls)
	}
}

// ---------------------------------------------------------------------------
// SecurityHook tests
// ---------------------------------------------------------------------------

func TestSecurityHookFrom_NoInject_ReturnsNoop(t *testing.T) {
	ctx := context.Background()
	h := SecurityHookFrom(ctx)
	if h == nil {
		t.Fatal("SecurityHookFrom should never return nil")
	}
	// noop MaskInput returns input unchanged
	if got := h.MaskInput("hello"); got != "hello" {
		t.Errorf("noop MaskInput: expected %q, got %q", "hello", got)
	}
	// noop CheckOutput returns nil
	if err := h.CheckOutput("output"); err != nil {
		t.Errorf("noop CheckOutput: expected nil, got %v", err)
	}
}

type blockingSecurityHook struct{}

func (blockingSecurityHook) MaskInput(input string) string { return "[MASKED]" }
func (blockingSecurityHook) CheckOutput(string) error      { return errors.New("blocked") }

func TestSecurityHookFrom_WithInject_ReturnsInjected(t *testing.T) {
	hook := blockingSecurityHook{}
	ctx := WithSecurityHook(context.Background(), hook)
	h := SecurityHookFrom(ctx)
	if got := h.MaskInput("secret"); got != "[MASKED]" {
		t.Errorf("expected [MASKED], got %q", got)
	}
	if err := h.CheckOutput("bad"); err == nil || err.Error() != "blocked" {
		t.Errorf("expected 'blocked' error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// SpanStarterHook tests
// ---------------------------------------------------------------------------

func TestSpanStarterHookFrom_NoInject_ReturnsNoop(t *testing.T) {
	ctx := context.Background()
	s := SpanStarterHookFrom(ctx)
	if s == nil {
		t.Fatal("SpanStarterHookFrom should never return nil")
	}
	newCtx, span := s.StartSpan(ctx, SpanKindStep, "test")
	if newCtx != ctx {
		t.Error("noop StartSpan should return original ctx")
	}
	if span == nil {
		t.Fatal("noop StartSpan should return non-nil span")
	}
	// should not panic
	span.End()
}

type recordingSpanStarter struct {
	called bool
}

func (r *recordingSpanStarter) StartSpan(ctx context.Context, kind SpanKind, name string) (context.Context, Span) {
	r.called = true
	return ctx, NoopSpan()
}

func TestSpanStarterHookFrom_WithInject_ReturnsInjected(t *testing.T) {
	rec := &recordingSpanStarter{}
	ctx := WithSpanStarterHook(context.Background(), rec)
	s := SpanStarterHookFrom(ctx)
	_, span := s.StartSpan(ctx, SpanKindTool, "action")
	span.End()
	if !rec.called {
		t.Error("expected injected SpanStarterHook to be called")
	}
}

// ---------------------------------------------------------------------------
// KVStore tests
// ---------------------------------------------------------------------------

func TestKVStoreFrom_NoInject_ReturnsNoop(t *testing.T) {
	ctx := context.Background()
	kv := KVStoreFrom(ctx)
	if kv == nil {
		t.Fatal("KVStoreFrom should never return nil")
	}
	// noop SetValue should not panic
	kv.SetValue("key", "value")
	// noop GetValue returns (nil, false)
	v, ok := kv.GetValue("key")
	if ok {
		t.Error("noop GetValue should return false")
	}
	if v != nil {
		t.Errorf("noop GetValue should return nil, got %v", v)
	}
}

func TestKVStoreFrom_WithInject_ReturnsInjected(t *testing.T) {
	// Use a simple map-backed KVStore for testing
	store := &mapKVStore{m: make(map[string]any)}
	ctx := WithKVStore(context.Background(), store)
	kv := KVStoreFrom(ctx)
	kv.SetValue("foo", "bar")
	v, ok := kv.GetValue("foo")
	if !ok || v != "bar" {
		t.Errorf("expected (bar, true), got (%v, %v)", v, ok)
	}
}

// mapKVStore is a test-only KVStore backed by a map.
type mapKVStore struct {
	m map[string]any
}

func (s *mapKVStore) SetValue(key string, value any) {
	s.m[key] = value
}

func (s *mapKVStore) GetValue(key string) (any, bool) {
	v, ok := s.m[key]
	return v, ok
}
