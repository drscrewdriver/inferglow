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

package sandbox

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// mgrMockProvider for testing manager - implements Provider interface.
// (Named differently from provider_test.go's mockProvider to avoid conflict.)
type mgrMockProvider struct {
	name      string
	kind      string
	available bool
	handle    Handle
	err       error
}

func (m *mgrMockProvider) Name() string { return m.name }
func (m *mgrMockProvider) Kind() string { return m.kind }
func (m *mgrMockProvider) InspectAvailability() (*AvailabilityResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &AvailabilityResult{Available: m.available}, nil
}
func (m *mgrMockProvider) CreateHandle(cfg map[string]any, policy *ExecutionPolicy) (Handle, error) {
	if m.handle == nil {
		return nil, fmt.Errorf("no handle")
	}
	return m.handle, nil
}

// 编译期断言：mgrMockProvider 满足 Provider 接口
var _ Provider = (*mgrMockProvider)(nil)

// mgrMockHandle for testing manager
type mgrMockHandle struct{}

func (h *mgrMockHandle) Start(ctx context.Context) error { return nil }
func (h *mgrMockHandle) Execute(ctx context.Context, cmd *Command) (*ExecutionResult, error) {
	return &ExecutionResult{ExitCode: 0}, nil
}
func (h *mgrMockHandle) Stop(ctx context.Context) error { return nil }
func (h *mgrMockHandle) Status() HandleStatus           { return StatusRunning }

// 编译期断言：mgrMockHandle 满足 Handle 接口
var _ Handle = (*mgrMockHandle)(nil)

func TestNewManager(t *testing.T) {
	m := NewManager()
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	if names := m.List(); len(names) != 0 {
		t.Fatalf("expected empty List, got %v", names)
	}
}

func TestManagerRegisterAndGet(t *testing.T) {
	m := NewManager()
	p := &mgrMockProvider{name: "test", kind: "test", available: true}
	if err := m.Register(p); err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	got, err := m.Get("test")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got != p {
		t.Fatal("Get returned different provider")
	}
}

func TestManagerRegisterDuplicate(t *testing.T) {
	m := NewManager()
	p1 := &mgrMockProvider{name: "dup", kind: "test", available: true}
	p2 := &mgrMockProvider{name: "dup", kind: "test", available: true}
	if err := m.Register(p1); err != nil {
		t.Fatalf("first Register failed: %v", err)
	}
	err := m.Register(p2)
	if !errors.Is(err, ErrProviderAlreadyRegistered) {
		t.Fatalf("expected ErrProviderAlreadyRegistered, got %v", err)
	}
}

func TestManagerGetNotFound(t *testing.T) {
	m := NewManager()
	_, err := m.Get("nonexistent")
	if !errors.Is(err, ErrProviderNotFound) {
		t.Fatalf("expected ErrProviderNotFound, got %v", err)
	}
}

func TestManagerList(t *testing.T) {
	m := NewManager()
	_ = m.Register(&mgrMockProvider{name: "charlie", kind: "test", available: true})
	_ = m.Register(&mgrMockProvider{name: "alpha", kind: "test", available: true})
	_ = m.Register(&mgrMockProvider{name: "bravo", kind: "test", available: true})
	names := m.List()
	expected := []string{"alpha", "bravo", "charlie"}
	if len(names) != 3 || names[0] != expected[0] || names[1] != expected[1] || names[2] != expected[2] {
		t.Fatalf("expected %v, got %v", expected, names)
	}
}

func TestManagerSelectSandboxExplicit(t *testing.T) {
	m := NewManager()
	p := &mgrMockProvider{name: "trusted_local", kind: "local", available: true}
	_ = m.Register(p)
	got, err := m.SelectSandbox(ModeTrustedLocal)
	if err != nil {
		t.Fatalf("SelectSandbox failed: %v", err)
	}
	if got != p {
		t.Fatal("SelectSandbox returned wrong provider")
	}
}

func TestManagerSelectSandboxNotFound(t *testing.T) {
	m := NewManager()
	_, err := m.SelectSandbox(ModeDocker)
	if !errors.Is(err, ErrProviderNotFound) {
		t.Fatalf("expected ErrProviderNotFound, got %v", err)
	}
}

func TestManagerSelectSandboxAutoOnlyTrustedLocal(t *testing.T) {
	m := NewManager()
	p := &mgrMockProvider{name: "trusted_local", kind: "local", available: true}
	_ = m.Register(p)
	got, err := m.SelectSandbox(ModeAuto)
	if err != nil {
		t.Fatalf("SelectSandbox(ModeAuto) failed: %v", err)
	}
	if got.Name() != "trusted_local" {
		t.Fatalf("expected trusted_local, got %s", got.Name())
	}
}

func TestManagerSelectSandboxAutoPrefersGVisor(t *testing.T) {
	m := NewManager()
	gvisor := &mgrMockProvider{name: "gvisor", kind: "sandbox", available: true}
	docker := &mgrMockProvider{name: "docker", kind: "container", available: true}
	trusted := &mgrMockProvider{name: "trusted_local", kind: "local", available: true}
	_ = m.Register(gvisor)
	_ = m.Register(docker)
	_ = m.Register(trusted)
	got, err := m.SelectSandbox(ModeAuto)
	if err != nil {
		t.Fatalf("SelectSandbox(ModeAuto) failed: %v", err)
	}
	if got.Name() != "gvisor" {
		t.Fatalf("expected gvisor, got %s", got.Name())
	}
}

func TestManagerSelectSandboxAutoFallsBackToDocker(t *testing.T) {
	m := NewManager()
	// gVisor not available, docker available
	gvisor := &mgrMockProvider{name: "gvisor", kind: "sandbox", available: false}
	docker := &mgrMockProvider{name: "docker", kind: "container", available: true}
	trusted := &mgrMockProvider{name: "trusted_local", kind: "local", available: true}
	_ = m.Register(gvisor)
	_ = m.Register(docker)
	_ = m.Register(trusted)
	got, err := m.SelectSandbox(ModeAuto)
	if err != nil {
		t.Fatalf("SelectSandbox(ModeAuto) failed: %v", err)
	}
	if got.Name() != "docker" {
		t.Fatalf("expected docker (gVisor unavailable), got %s", got.Name())
	}
}

func TestManagerSelectSandboxAutoFallsBackToTrustedLocal(t *testing.T) {
	m := NewManager()
	gvisor := &mgrMockProvider{name: "gvisor", kind: "sandbox", available: false}
	docker := &mgrMockProvider{name: "docker", kind: "container", available: false}
	trusted := &mgrMockProvider{name: "trusted_local", kind: "local", available: true}
	_ = m.Register(gvisor)
	_ = m.Register(docker)
	_ = m.Register(trusted)
	got, err := m.SelectSandbox(ModeAuto)
	if err != nil {
		t.Fatalf("SelectSandbox(ModeAuto) failed: %v", err)
	}
	if got.Name() != "trusted_local" {
		t.Fatalf("expected trusted_local, got %s", got.Name())
	}
}

func TestManagerSelectSandboxAutoNoneAvailable(t *testing.T) {
	m := NewManager()
	gvisor := &mgrMockProvider{name: "gvisor", kind: "sandbox", available: false}
	docker := &mgrMockProvider{name: "docker", kind: "container", available: false}
	trusted := &mgrMockProvider{name: "trusted_local", kind: "local", available: false}
	_ = m.Register(gvisor)
	_ = m.Register(docker)
	_ = m.Register(trusted)
	_, err := m.SelectSandbox(ModeAuto)
	if !errors.Is(err, ErrNoAvailableSandbox) {
		t.Fatalf("expected ErrNoAvailableSandbox, got %v", err)
	}
}

func TestManagerSelectSandboxAutoSkipsInspectError(t *testing.T) {
	// Provider whose InspectAvailability returns error should be skipped
	m := NewManager()
	gvisor := &mgrMockProvider{name: "gvisor", kind: "sandbox", err: errors.New("inspect failed")}
	trusted := &mgrMockProvider{name: "trusted_local", kind: "local", available: true}
	_ = m.Register(gvisor)
	_ = m.Register(trusted)
	got, err := m.SelectSandbox(ModeAuto)
	if err != nil {
		t.Fatalf("SelectSandbox(ModeAuto) failed: %v", err)
	}
	if got.Name() != "trusted_local" {
		t.Fatalf("expected trusted_local (gVisor errored), got %s", got.Name())
	}
}

func TestManagerCreateHandle(t *testing.T) {
	m := NewManager()
	h := &mgrMockHandle{}
	p := &mgrMockProvider{name: "trusted_local", kind: "local", available: true, handle: h}
	_ = m.Register(p)
	policy := DefaultPolicy()
	got, err := m.CreateHandle(ModeTrustedLocal, nil, &policy)
	if err != nil {
		t.Fatalf("CreateHandle failed: %v", err)
	}
	if got != h {
		t.Fatal("CreateHandle returned wrong handle")
	}
}

func TestManagerConcurrent(t *testing.T) {
	m := NewManager()
	var wg sync.WaitGroup
	// Concurrent Register
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = m.Register(&mgrMockProvider{
				name:      fmt.Sprintf("p%d", i),
				kind:      "test",
				available: true,
			})
		}(i)
	}
	// Concurrent SelectSandbox
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = m.SelectSandbox(ModeAuto)
		}()
	}
	// Concurrent List
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = m.List()
		}()
	}
	wg.Wait()
	// Just verify no panic/race
	_ = time.Now()
}
