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

package resource

import (
	"context"
	"errors"
	"testing"
)

// --- ResourceHandle tests ---

func TestHandleLifecycle(t *testing.T) {
	res := NewNoopResource("bash")
	h := newHandle("h1", res, "scope-1")

	if h.State() != StateCreating {
		t.Fatalf("expected creating, got %s", h.State())
	}

	h.MarkReady()
	if h.State() != StateReady {
		t.Fatalf("expected ready, got %s", h.State())
	}

	ctx := context.Background()
	result, err := h.Execute(ctx, "echo hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.OK {
		t.Fatal("expected OK result")
	}
	if h.State() != StateIdle {
		t.Fatalf("expected idle after execute, got %s", h.State())
	}
	if h.UseCount() != 1 {
		t.Fatalf("expected use count 1, got %d", h.UseCount())
	}

	if err := h.HealthCheck(ctx); err != nil {
		t.Fatalf("health check failed: %v", err)
	}

	if err := h.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
	if h.State() != StateClosed {
		t.Fatalf("expected closed, got %s", h.State())
	}

	// Execute after close must fail.
	_, err = h.Execute(ctx, "echo hello")
	if !errors.Is(err, ErrResourceClosed) {
		t.Fatalf("expected ErrResourceClosed, got %v", err)
	}
}

func TestHandleFailedState(t *testing.T) {
	res := &failingResource{resType: "bash"}
	h := newHandle("h2", res, "")
	h.MarkReady()

	ctx := context.Background()
	_, err := h.Execute(ctx, "bad")
	if err == nil {
		t.Fatal("expected error from failing resource")
	}
	if h.State() != StateFailed {
		t.Fatalf("expected failed state, got %s", h.State())
	}

	// Further Execute must return ErrResourceFailed.
	_, err = h.Execute(ctx, "bad")
	if !errors.Is(err, ErrResourceFailed) {
		t.Fatalf("expected ErrResourceFailed, got %v", err)
	}
}

func TestHandleDoubleClose(t *testing.T) {
	res := NewNoopResource("bash")
	h := newHandle("h3", res, "")
	if err := h.Close(); err != nil {
		t.Fatalf("first close failed: %v", err)
	}
	// Second close is a no-op.
	if err := h.Close(); err != nil {
		t.Fatalf("second close should be no-op, got %v", err)
	}
}

func TestHandleStatus(t *testing.T) {
	res := NewNoopResource("python")
	h := newHandle("h4", res, "exec-1")
	h.MarkReady()

	status := h.Status()
	if status.HandleID != "h4" {
		t.Fatalf("expected h4, got %s", status.HandleID)
	}
	if status.ResourceType != "python" {
		t.Fatalf("expected python, got %s", status.ResourceType)
	}
	if status.Scope != "exec-1" {
		t.Fatalf("expected exec-1, got %s", status.Scope)
	}
	if !status.Healthy {
		t.Fatal("expected healthy")
	}
}

// --- ResourceManager tests ---

func TestManagerRegisterAndEnsure(t *testing.T) {
	m := NewResourceManager()
	p := NewNoopProvider("bash", "filesystem")

	if err := m.RegisterProvider(p, false); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	// Duplicate registration without replace should fail.
	if err := m.RegisterProvider(p, false); err == nil {
		t.Fatal("expected ErrProviderExists")
	}

	// With replace=true it should succeed.
	if err := m.RegisterProvider(p, true); err != nil {
		t.Fatalf("replace register failed: %v", err)
	}

	ctx := context.Background()
	req := Requirement{Type: "bash", Scope: "test-scope"}
	h, err := m.Ensure(ctx, req)
	if err != nil {
		t.Fatalf("ensure failed: %v", err)
	}
	if h == nil {
		t.Fatal("expected non-nil handle")
	}
	if h.Scope() != "test-scope" {
		t.Fatalf("expected test-scope, got %s", h.Scope())
	}

	result, err := h.Execute(ctx, "ls")
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if !result.OK {
		t.Fatal("expected OK")
	}
}

func TestManagerCapabilityCheck(t *testing.T) {
	m := NewResourceManager()
	p := NewNoopProvider("bash") // no capabilities
	_ = m.RegisterProvider(p, false)

	ctx := context.Background()
	req := Requirement{
		Type:         "bash",
		Capabilities: []string{"gpu"},
	}
	_, err := m.Ensure(ctx, req)
	if !errors.Is(err, ErrCapabilityMismatch) {
		t.Fatalf("expected ErrCapabilityMismatch, got %v", err)
	}
}

func TestManagerProviderNotFound(t *testing.T) {
	m := NewResourceManager()
	ctx := context.Background()
	_, err := m.Ensure(ctx, Requirement{Type: "nonexistent"})
	if !errors.Is(err, ErrProviderNotFound) {
		t.Fatalf("expected ErrProviderNotFound, got %v", err)
	}
}

func TestManagerRelease(t *testing.T) {
	m := NewResourceManager()
	p := NewNoopProvider("bash")
	_ = m.RegisterProvider(p, false)

	ctx := context.Background()
	h, _ := m.Ensure(ctx, Requirement{Type: "bash", Scope: "s1"})

	if err := m.Release(h); err != nil {
		t.Fatalf("release failed: %v", err)
	}

	// Handle should be closed.
	if h.State() != StateClosed {
		t.Fatalf("expected closed after release, got %s", h.State())
	}

	// List should be empty.
	if len(m.List()) != 0 {
		t.Fatalf("expected 0 handles after release, got %d", len(m.List()))
	}
}

func TestManagerReleaseScope(t *testing.T) {
	m := NewResourceManager()
	p := NewNoopProvider("bash")
	_ = m.RegisterProvider(p, false)

	ctx := context.Background()
	h1, _ := m.Ensure(ctx, Requirement{Type: "bash", Scope: "s1", Properties: map[string]string{"id": "1"}})
	h2, _ := m.Ensure(ctx, Requirement{Type: "bash", Scope: "s1", Properties: map[string]string{"id": "2"}})
	h3, _ := m.Ensure(ctx, Requirement{Type: "bash", Scope: "s2", Properties: map[string]string{"id": "3"}})

	if err := m.ReleaseScope("s1"); err != nil {
		t.Fatalf("release scope failed: %v", err)
	}

	if h1.State() != StateClosed || h2.State() != StateClosed {
		t.Fatal("s1 handles should be closed")
	}
	if h3.State() == StateClosed {
		t.Fatal("s2 handle should not be closed")
	}
	if len(m.List()) != 1 {
		t.Fatalf("expected 1 handle remaining, got %d", len(m.List()))
	}
}

func TestManagerHandleReuse(t *testing.T) {
	m := NewResourceManager()
	p := NewNoopProvider("bash")
	_ = m.RegisterProvider(p, false)

	ctx := context.Background()
	req := Requirement{Type: "bash"}

	h1, _ := m.Ensure(ctx, req)
	h2, _ := m.Ensure(ctx, req)

	// Same handle should be reused.
	if h1.ID() != h2.ID() {
		t.Fatal("expected handle reuse for same requirement")
	}
}

func TestManagerCloseAll(t *testing.T) {
	m := NewResourceManager()
	p := NewNoopProvider("bash")
	_ = m.RegisterProvider(p, false)

	ctx := context.Background()
	m.Ensure(ctx, Requirement{Type: "bash"})
	m.Ensure(ctx, Requirement{Type: "bash", Scope: "x"})

	if err := m.CloseAll(); err != nil {
		t.Fatalf("close all failed: %v", err)
	}
	if len(m.List()) != 0 {
		t.Fatalf("expected 0 handles after close all, got %d", len(m.List()))
	}
}

func TestManagerInspect(t *testing.T) {
	m := NewResourceManager()
	status := m.Inspect(nil)
	if status.HandleID != "" {
		t.Fatal("expected empty status for nil handle")
	}

	p := NewNoopProvider("bash")
	_ = m.RegisterProvider(p, false)

	ctx := context.Background()
	h, _ := m.Ensure(ctx, Requirement{Type: "bash"})
	status = m.Inspect(h)
	if status.HandleID != h.ID() {
		t.Fatalf("expected %s, got %s", h.ID(), status.HandleID)
	}
}

// --- NoopProvider tests ---

func TestNoopProvider(t *testing.T) {
	p := NewNoopProvider("python", "filesystem", "network")
	if p.Type() != "python" {
		t.Fatalf("expected python, got %s", p.Type())
	}
	caps := p.Capabilities()
	if len(caps) != 2 {
		t.Fatalf("expected 2 capabilities, got %d", len(caps))
	}

	ctx := context.Background()
	if err := p.Probe(ctx); err != nil {
		t.Fatalf("probe failed: %v", err)
	}

	res, err := p.Create(ctx, ResourceConfig{})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if res.Type() != "python" {
		t.Fatalf("expected python, got %s", res.Type())
	}
}

// --- failingResource helper ---

type failingResource struct {
	resType string
}

func (r *failingResource) ID() string   { return "fail-1" }
func (r *failingResource) Type() string { return r.resType }
func (r *failingResource) Execute(_ context.Context, _ any) (*ResourceResult, error) {
	return nil, errors.New("simulated failure")
}
func (r *failingResource) HealthCheck(_ context.Context) error { return nil }
func (r *failingResource) Close() error                        { return nil }
