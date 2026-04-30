package sandbox

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"
)

// TestIntegration_ManagerWithBuiltinProviders verifies that all 4 builtin
// providers can be registered together and listed.
func TestIntegration_ManagerWithBuiltinProviders(t *testing.T) {
	m := NewManager()
	if err := m.Register(NewTrustedLocalProvider()); err != nil {
		t.Fatalf("register trusted_local: %v", err)
	}
	dockerProv, err := NewDockerProvider()
	if err != nil {
		t.Logf("docker not available: %v", err)
	} else if err := m.Register(dockerProv); err != nil {
		t.Fatalf("register docker: %v", err)
	}
	gvisorProv, err := NewGVisorProvider()
	if err != nil {
		t.Logf("gvisor not available: %v", err)
	} else if err := m.Register(gvisorProv); err != nil {
		t.Fatalf("register gvisor: %v", err)
	}
	if err := m.Register(NewLocalSandboxProvider()); err != nil {
		t.Fatalf("register local: %v", err)
	}
	names := m.List()
	// Expect at least 1 (trusted_local) and up to 4
	if len(names) < 1 {
		t.Fatalf("expected at least 1 provider, got %d: %v", len(names), names)
	}
	if len(names) > 4 {
		t.Fatalf("expected at most 4 providers, got %d: %v", len(names), names)
	}
}

// TestIntegration_AutoFallbackChain verifies that when only TrustedLocal is
// available, SelectSandbox(ModeAuto) returns TrustedLocal.
func TestIntegration_AutoFallbackChain(t *testing.T) {
	m := NewManager()
	// Register only TrustedLocal — Docker/GVisor/LocalSandbox not registered
	if err := m.Register(NewTrustedLocalProvider()); err != nil {
		t.Fatalf("register: %v", err)
	}
	p, err := m.SelectSandbox(ModeAuto)
	if err != nil {
		t.Fatalf("SelectSandbox(ModeAuto): %v", err)
	}
	if p.Name() != "trusted_local" {
		t.Errorf("got %q, want trusted_local", p.Name())
	}
}

// TestIntegration_AutoFallbackWithAllRegistered verifies that with all 4
// providers registered, ModeAuto picks the first available one (likely
// trusted_local since docker/runsc are usually not installed in test env).
func TestIntegration_AutoFallbackWithAllRegistered(t *testing.T) {
	m := NewManager()
	_ = m.Register(NewTrustedLocalProvider())
	dockerProv, _ := NewDockerProvider()
	if dockerProv != nil {
		_ = m.Register(dockerProv)
	}
	gvisorProv, _ := NewGVisorProvider()
	if gvisorProv != nil {
		_ = m.Register(gvisorProv)
	}
	_ = m.Register(NewLocalSandboxProvider())
	p, err := m.SelectSandbox(ModeAuto)
	if err != nil {
		t.Fatalf("SelectSandbox(ModeAuto): %v", err)
	}
	// In typical test env, docker/runsc not installed, local_sandbox is stub
	// → should fall back to trusted_local
	if p.Name() != "trusted_local" {
		t.Logf("Note: SelectSandbox picked %q (docker/runsc may be installed)", p.Name())
	}
}

// TestIntegration_TrustedLocalExecuteEcho verifies end-to-end: Manager →
// CreateHandle → Start → Execute(echo hello) → Stop.
func TestIntegration_TrustedLocalExecuteEcho(t *testing.T) {
	m := NewManager()
	if err := m.Register(NewTrustedLocalProvider()); err != nil {
		t.Fatalf("register: %v", err)
	}
	policy := DefaultPolicy()
	h, err := m.CreateHandle(ModeTrustedLocal, nil, &policy)
	if err != nil {
		t.Fatalf("CreateHandle: %v", err)
	}
	if err := h.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer h.Stop(context.Background())
	result, err := h.Execute(context.Background(), &Command{Argv: []string{"echo", "hello"}})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
	if !strings.Contains(result.Stdout, "hello") {
		t.Errorf("Stdout = %q, want contains %q", result.Stdout, "hello")
	}
}

// TestIntegration_DockerHandleWithMock tests Docker handle through mock client.
func TestIntegration_DockerHandleWithMock(t *testing.T) {
	mc := &mockDockerClient{
		createFn: func(config *ContainerConfig, hostConfig *HostConfig, networkingConfig *NetworkSettings, containerConfig *ContainerConfig, opts ...any) (*ContainerCreateResult, error) {
			return &ContainerCreateResult{ID: "mock123"}, nil
		},
		startFn: func(containerID string, opts ...any) error { return nil },
	}
	m := NewManager()
	prov := NewDockerProviderWithClient(mc)
	if err := m.Register(prov); err != nil {
		t.Fatalf("register docker: %v", err)
	}
	policy := DefaultPolicy()
	h, err := m.CreateHandle(ModeDocker, nil, &policy)
	if err != nil {
		t.Fatalf("CreateHandle: %v", err)
	}
	if err := h.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if h.Status() != StatusRunning {
		t.Errorf("Status = %q, want %q", h.Status(), StatusRunning)
	}
	if err := h.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if h.Status() != StatusStopped {
		t.Errorf("Status = %q, want %q", h.Status(), StatusStopped)
	}
}

// TestIntegration_OSDetectionConsistency verifies that DetectOS matches
// runtime.GOOS and AvailableProvidersOnOS(DetectOS()) includes at least
// TrustedLocal (which is supported on all OSes).
func TestIntegration_OSDetectionConsistency(t *testing.T) {
	detected := DetectOS()
	switch runtime.GOOS {
	case "darwin":
		if detected != OSDarwin {
			t.Errorf("DetectOS() = %q, want %q", detected, OSDarwin)
		}
	case "linux":
		if detected != OSLinux {
			t.Errorf("DetectOS() = %q, want %q", detected, OSLinux)
		}
	case "windows":
		if detected != OSWindows {
			t.Errorf("DetectOS() = %q, want %q", detected, OSWindows)
		}
	default:
		if detected != OSUnknown {
			t.Errorf("DetectOS() = %q, want %q", detected, OSUnknown)
		}
	}
	providers := AvailableProvidersOnOS(detected)
	if len(providers) == 0 {
		t.Fatalf("no providers available on %q", detected)
	}
	found := false
	for _, p := range providers {
		if p == ProviderTrustedLocal {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("ProviderTrustedLocal not in available providers for %q: %v", detected, providers)
	}
}

// TestIntegration_SelectSandboxExplicit verifies explicit mode selection works
// for all 4 registered providers.
func TestIntegration_SelectSandboxExplicit(t *testing.T) {
	m := NewManager()
	_ = m.Register(NewTrustedLocalProvider())
	dockerProv, _ := NewDockerProvider()
	if dockerProv != nil {
		_ = m.Register(dockerProv)
	}
	gvisorProv, _ := NewGVisorProvider()
	if gvisorProv != nil {
		_ = m.Register(gvisorProv)
	}
	_ = m.Register(NewLocalSandboxProvider())

	cases := []struct {
		mode     SandboxMode
		wantName string
		skip     bool
	}{
		{ModeTrustedLocal, "trusted_local", false},
		{ModeDocker, "docker", dockerProv == nil},
		{ModeGVisor, "gvisor", gvisorProv == nil},
		{ModeLocal, "local", false},
	}
	for _, c := range cases {
		if c.skip {
			t.Logf("skipping %q (provider not available)", c.mode)
			continue
		}
		p, err := m.SelectSandbox(c.mode)
		if err != nil {
			t.Errorf("SelectSandbox(%q): %v", c.mode, err)
			continue
		}
		if p.Name() != c.wantName {
			t.Errorf("SelectSandbox(%q) = %q, want %q", c.mode, p.Name(), c.wantName)
		}
	}
}

// TestIntegration_SelectSandboxUnregisteredMode verifies that selecting a mode
// for which no provider is registered returns ErrProviderNotFound.
func TestIntegration_SelectSandboxUnregisteredMode(t *testing.T) {
	m := NewManager()
	// No providers registered
	_, err := m.SelectSandbox(ModeTrustedLocal)
	if !errors.Is(err, ErrProviderNotFound) {
		t.Errorf("expected ErrProviderNotFound, got %v", err)
	}
}

// TestIntegration_AutoNoneAvailableWhenOnlyStubs verifies that when only stub
// providers (whose InspectAvailability returns false) are registered,
// SelectSandbox(ModeAuto) returns ErrNoAvailableSandbox.
func TestIntegration_AutoNoneAvailableWhenOnlyStubs(t *testing.T) {
	m := NewManager()
	// Register only LocalSandbox (always unavailable)
	_ = m.Register(NewLocalSandboxProvider())
	_, err := m.SelectSandbox(ModeAuto)
	// LocalSandbox is named "local" which is in the fallback chain, but
	// InspectAvailability returns false, so auto should fail.
	// However, the fallback chain is gvisor → docker → local → trusted_local.
	// Only "local" is registered and it's unavailable, so we get ErrNoAvailableSandbox.
	if !errors.Is(err, ErrNoAvailableSandbox) {
		t.Errorf("expected ErrNoAvailableSandbox, got %v", err)
	}
}

// TestIntegration_TrustedLocalHandleLifecycle verifies the full lifecycle of
// a TrustedLocalHandle: Created → Running → Stopped.
func TestIntegration_TrustedLocalHandleLifecycle(t *testing.T) {
	p := NewTrustedLocalProvider()
	policy := DefaultPolicy()
	h, err := p.CreateHandle(nil, &policy)
	if err != nil {
		t.Fatalf("CreateHandle: %v", err)
	}
	if h.Status() != StatusCreated {
		t.Errorf("initial Status = %q, want %q", h.Status(), StatusCreated)
	}
	if err := h.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if h.Status() != StatusRunning {
		t.Errorf("Status after Start = %q, want %q", h.Status(), StatusRunning)
	}
	if err := h.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if h.Status() != StatusStopped {
		t.Errorf("Status after Stop = %q, want %q", h.Status(), StatusStopped)
	}
}
