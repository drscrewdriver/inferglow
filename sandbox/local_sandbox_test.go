package sandbox

import (
	"errors"
	"strings"
	"testing"
)

// ============================================================================
// 基础测试
// ============================================================================

func TestNewLocalSandboxProvider(t *testing.T) {
	p := NewLocalSandboxProvider()
	if p == nil {
		t.Fatal("NewLocalSandboxProvider returned nil")
	}
}

func TestLocalSandboxProviderImplementsProvider(t *testing.T) {
	var _ Provider = (*LocalSandboxProvider)(nil)
}

func TestLocalSandboxProviderNameKind(t *testing.T) {
	p := NewLocalSandboxProvider()
	if p.Name() != "local" {
		t.Errorf("Name() = %q, want %q", p.Name(), "local")
	}
	if p.Kind() != "local" {
		t.Errorf("Kind() = %q, want %q", p.Kind(), "local")
	}
}

// ============================================================================
// 调度逻辑测试（使用 provider_test.go 中已有的 mockProvider / mockHandle）
// ============================================================================

func TestLocalSandboxEmptyBackendsUnavailable(t *testing.T) {
	p := NewLocalSandboxProvider().WithBackends()
	avail, err := p.InspectAvailability()
	if err != nil {
		t.Fatalf("InspectAvailability error: %v", err)
	}
	if avail.Available {
		t.Error("expected Available=false with empty backends")
	}
	if !strings.Contains(avail.ErrorMessage, "no local backend available") {
		t.Errorf("ErrorMessage = %q, want contains 'no local backend available'", avail.ErrorMessage)
	}
}

func TestLocalSandboxEmptyBackendsCreateHandleFails(t *testing.T) {
	p := NewLocalSandboxProvider().WithBackends()
	policy := DefaultPolicy()
	_, err := p.CreateHandle(nil, &policy)
	if !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("expected ErrProviderUnavailable, got %v", err)
	}
}

func TestLocalSandboxSelectsFirstAvailableBackend(t *testing.T) {
	backend1 := &mockProvider{
		name:  "backend1",
		kind:  "local",
		avail: &AvailabilityResult{Available: false, ErrorMessage: "backend1 not available"},
	}
	backend2 := &mockProvider{
		name:  "backend2",
		kind:  "local",
		avail: &AvailabilityResult{Available: true},
		handle: &mockHandle{statusVal: StatusRunning},
	}
	backend3 := &mockProvider{
		name:  "backend3",
		kind:  "local",
		avail: &AvailabilityResult{Available: true},
		handle: &mockHandle{statusVal: StatusRunning},
	}

	p := NewLocalSandboxProvider().WithBackends(backend1, backend2, backend3)

	selected, err := p.SelectBackend()
	if err != nil {
		t.Fatalf("SelectBackend error: %v", err)
	}
	if selected.Name() != "backend2" {
		t.Errorf("selected = %q, want backend2 (first available)", selected.Name())
	}
}

func TestLocalSandboxInspectAvailabilityDelegatesToSelected(t *testing.T) {
	backend := &mockProvider{
		name:  "real_backend",
		kind:  "local",
		avail: &AvailabilityResult{Available: true, BinaryPath: "/usr/bin/real"},
	}
	p := NewLocalSandboxProvider().WithBackends(backend)

	avail, err := p.InspectAvailability()
	if err != nil {
		t.Fatalf("InspectAvailability error: %v", err)
	}
	if !avail.Available {
		t.Error("expected Available=true when backend is available")
	}
	if avail.BinaryPath != "/usr/bin/real" {
		t.Errorf("BinaryPath = %q, want /usr/bin/real", avail.BinaryPath)
	}
}

func TestLocalSandboxCreateHandleDelegatesToSelected(t *testing.T) {
	wantHandle := &mockHandle{statusVal: StatusRunning}
	backend := &mockProvider{
		name:   "real_backend",
		kind:   "local",
		avail:  &AvailabilityResult{Available: true},
		handle: wantHandle,
	}
	p := NewLocalSandboxProvider().WithBackends(backend)

	policy := DefaultPolicy()
	h, err := p.CreateHandle(nil, &policy)
	if err != nil {
		t.Fatalf("CreateHandle error: %v", err)
	}
	if h != wantHandle {
		t.Error("returned handle is not the backend's handle")
	}
}

func TestLocalSandboxSkipsErroringBackends(t *testing.T) {
	// 第一个后端 InspectAvailability 返回 error，应跳过。
	errBackend := &mockProvider{
		name:     "err_backend",
		kind:     "local",
		availErr: errors.New("probe failed"),
	}
	goodBackend := &mockProvider{
		name:   "good_backend",
		kind:   "local",
		avail:  &AvailabilityResult{Available: true},
		handle: &mockHandle{statusVal: StatusRunning},
	}
	p := NewLocalSandboxProvider().WithBackends(errBackend, goodBackend)

	selected, err := p.SelectBackend()
	if err != nil {
		t.Fatalf("SelectBackend error: %v", err)
	}
	if selected.Name() != "good_backend" {
		t.Errorf("selected = %q, want good_backend", selected.Name())
	}
}

func TestLocalSandboxAllBackendsUnavailable(t *testing.T) {
	b1 := &mockProvider{name: "b1", kind: "local", avail: &AvailabilityResult{Available: false, ErrorMessage: "b1 unavailable"}}
	b2 := &mockProvider{name: "b2", kind: "local", avail: &AvailabilityResult{Available: false, ErrorMessage: "b2 unavailable"}}
	p := NewLocalSandboxProvider().WithBackends(b1, b2)

	_, err := p.SelectBackend()
	if !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("expected ErrProviderUnavailable, got %v", err)
	}
	if !strings.Contains(err.Error(), "b2 unavailable") {
		t.Errorf("error should include last backend's error message, got: %v", err)
	}
}

func TestLocalSandboxAllBackendsErroring(t *testing.T) {
	b1 := &mockProvider{name: "b1", kind: "local", availErr: errors.New("b1 probe error")}
	b2 := &mockProvider{name: "b2", kind: "local", availErr: errors.New("b2 probe error")}
	p := NewLocalSandboxProvider().WithBackends(b1, b2)

	_, err := p.SelectBackend()
	if !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("expected ErrProviderUnavailable, got %v", err)
	}
	if !strings.Contains(err.Error(), "b2 probe error") {
		t.Errorf("error should include last backend's error, got: %v", err)
	}
}

func TestLocalSandboxBackendsReturnsCopy(t *testing.T) {
	b1 := &mockProvider{name: "b1", kind: "local", avail: &AvailabilityResult{Available: true}}
	p := NewLocalSandboxProvider().WithBackends(b1)

	original := p.Backends()
	if len(original) != 1 || original[0] != b1 {
		t.Fatalf("Backends() = %v, want [b1]", original)
	}
	// 修改返回的切片不应影响内部状态。
	original[0] = &mockProvider{name: "injected", kind: "local"}
	again := p.Backends()
	if again[0].Name() != "b1" {
		t.Errorf("Backends() internal state mutated via returned slice")
	}
}

func TestLocalSandboxWithBackendsNilClears(t *testing.T) {
	b1 := &mockProvider{name: "b1", kind: "local", avail: &AvailabilityResult{Available: true}}
	p := NewLocalSandboxProvider().WithBackends(b1)
	// WithBackends(nil) 应清空。
	p.WithBackends(nil)
	if len(p.Backends()) != 0 {
		t.Errorf("WithBackends(nil) should clear, got %d backends", len(p.Backends()))
	}
}

func TestDefaultLocalBackendsMatchesOS(t *testing.T) {
	backends := DefaultLocalBackends()
	switch DetectOS() {
	case OSDarwin:
		if len(backends) != 1 || backends[0].Name() != "seatbelt" {
			t.Errorf("darwin backends = %v, want [seatbelt]", backends)
		}
	case OSLinux:
		// Bubblewrap + Landlock 已实现并按优先级加入。
		if len(backends) != 2 {
			t.Errorf("linux backends = %v, want [bubblewrap, landlock]", backends)
		} else {
			if backends[0].Name() != "bubblewrap" {
				t.Errorf("linux backends[0] = %q, want bubblewrap", backends[0].Name())
			}
			if backends[1].Name() != "landlock" {
				t.Errorf("linux backends[1] = %q, want landlock", backends[1].Name())
			}
		}
	case OSWindows:
		if len(backends) != 1 || backends[0].Name() != "windows_runtime" {
			t.Errorf("windows backends = %v, want [windows_runtime]", backends)
		}
	}
}

// ============================================================================
// 并发安全测试
// ============================================================================

func TestLocalSandboxConcurrentAccess(t *testing.T) {
	p := NewLocalSandboxProvider().WithBackends()
	// 并发读 Backends() 和 InspectAvailability()。
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			_ = p.Backends()
			_, _ = p.InspectAvailability()
		}
	}()
	// 主 goroutine 并发写。
	for i := 0; i < 100; i++ {
		p.WithBackends(&mockProvider{name: "b", kind: "local", avail: &AvailabilityResult{Available: false}})
	}
	<-done
}
