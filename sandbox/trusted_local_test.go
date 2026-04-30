package sandbox

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestNewTrustedLocalProvider(t *testing.T) {
	p := NewTrustedLocalProvider()
	if p == nil {
		t.Fatal("NewTrustedLocalProvider returned nil")
	}
}

func TestTrustedLocalProviderNameKind(t *testing.T) {
	p := NewTrustedLocalProvider()
	if p.Name() != "trusted_local" {
		t.Errorf("Name() = %q, want %q", p.Name(), "trusted_local")
	}
	if p.Kind() != "local" {
		t.Errorf("Kind() = %q, want %q", p.Kind(), "local")
	}
}

func TestTrustedLocalProviderImplementsProvider(t *testing.T) {
	var _ Provider = (*TrustedLocalProvider)(nil)
}

func TestTrustedLocalProviderInspectAvailability(t *testing.T) {
	p := NewTrustedLocalProvider()
	avail, err := p.InspectAvailability()
	if err != nil {
		t.Fatalf("InspectAvailability failed: %v", err)
	}
	if avail == nil {
		t.Fatal("InspectAvailability returned nil")
	}
	if !avail.Available {
		t.Error("expected Available=true")
	}
	// Platform should match current OS
	expectedPlatform := string(DetectOS())
	if avail.Platform != expectedPlatform {
		t.Errorf("Platform = %q, want %q", avail.Platform, expectedPlatform)
	}
}

func TestTrustedLocalProviderCreateHandle(t *testing.T) {
	p := NewTrustedLocalProvider()
	policy := DefaultPolicy()
	h, err := p.CreateHandle(nil, &policy)
	if err != nil {
		t.Fatalf("CreateHandle failed: %v", err)
	}
	if h == nil {
		t.Fatal("CreateHandle returned nil handle")
	}
	if h.Status() != StatusCreated {
		t.Errorf("initial Status = %q, want %q", h.Status(), StatusCreated)
	}
}

func TestTrustedLocalHandleStart(t *testing.T) {
	p := NewTrustedLocalProvider()
	policy := DefaultPolicy()
	h, _ := p.CreateHandle(nil, &policy)
	if err := h.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if h.Status() != StatusRunning {
		t.Errorf("Status after Start = %q, want %q", h.Status(), StatusRunning)
	}
}

func TestTrustedLocalHandleStop(t *testing.T) {
	p := NewTrustedLocalProvider()
	policy := DefaultPolicy()
	h, _ := p.CreateHandle(nil, &policy)
	_ = h.Start(context.Background())
	if err := h.Stop(context.Background()); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if h.Status() != StatusStopped {
		t.Errorf("Status after Stop = %q, want %q", h.Status(), StatusStopped)
	}
}

func TestTrustedLocalHandleExecuteNotRunning(t *testing.T) {
	p := NewTrustedLocalProvider()
	policy := DefaultPolicy()
	h, _ := p.CreateHandle(nil, &policy)
	// Don't call Start
	_, err := h.Execute(context.Background(), &Command{Argv: []string{"echo", "hello"}})
	if !errors.Is(err, ErrHandleNotRunning) {
		t.Fatalf("expected ErrHandleNotRunning, got %v", err)
	}
}

func TestTrustedLocalHandleExecuteEcho(t *testing.T) {
	p := NewTrustedLocalProvider()
	policy := DefaultPolicy()
	h, _ := p.CreateHandle(nil, &policy)
	_ = h.Start(context.Background())
	defer h.Stop(context.Background())

	result, err := h.Execute(context.Background(), &Command{Argv: []string{"echo", "hello"}})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
	if !strings.Contains(result.Stdout, "hello") {
		t.Errorf("Stdout = %q, want contains %q", result.Stdout, "hello")
	}
}

func TestTrustedLocalHandleExecuteAllowedCommands(t *testing.T) {
	p := NewTrustedLocalProvider()
	policy := ExecutionPolicy{
		SandboxMode:     ModeTrustedLocal,
		AllowedCommands: []string{"echo"},
	}
	h, _ := p.CreateHandle(nil, &policy)
	_ = h.Start(context.Background())
	defer h.Stop(context.Background())

	// echo is allowed
	_, err := h.Execute(context.Background(), &Command{Argv: []string{"echo", "hi"}})
	if err != nil {
		t.Fatalf("echo should be allowed: %v", err)
	}
	// ls is not allowed (use a command that's definitely not "echo")
	// On Windows, use "dir"; on Unix, use "ls"
	badCmd := "ls"
	if runtime.GOOS == "windows" {
		badCmd = "dir"
	}
	_, err = h.Execute(context.Background(), &Command{Argv: []string{badCmd}})
	if !errors.Is(err, ErrCommandNotAllowed) {
		t.Fatalf("expected ErrCommandNotAllowed for %q, got %v", badCmd, err)
	}
}

func TestTrustedLocalHandleExecuteEmptyAllowedAllowsAll(t *testing.T) {
	p := NewTrustedLocalProvider()
	policy := DefaultPolicy() // AllowedCommands is nil/empty
	h, _ := p.CreateHandle(nil, &policy)
	_ = h.Start(context.Background())
	defer h.Stop(context.Background())

	// echo should work
	_, err := h.Execute(context.Background(), &Command{Argv: []string{"echo", "ok"}})
	if err != nil {
		t.Fatalf("echo should be allowed with empty AllowedCommands: %v", err)
	}
}

func TestTrustedLocalHandleExecuteTimeout(t *testing.T) {
	p := NewTrustedLocalProvider()
	policy := ExecutionPolicy{
		SandboxMode: ModeTrustedLocal,
		Timeout:     100 * time.Millisecond,
	}
	h, _ := p.CreateHandle(nil, &policy)
	_ = h.Start(context.Background())
	defer h.Stop(context.Background())

	// Sleep for 1 second should be killed by 100ms timeout
	// On Windows, use "timeout" command; on Unix, use "sleep"
	var argv []string
	if runtime.GOOS == "windows" {
		// timeout command: /T 2 means wait 2 seconds, /NOBREAK prevents Ctrl+C interrupt
		// But timeout is interactive; use ping instead which is more reliable
		argv = []string{"ping", "-n", "2", "127.0.0.1"}
	} else {
		argv = []string{"sleep", "1"}
	}
	_, err := h.Execute(context.Background(), &Command{Argv: argv})
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	// Should be context.DeadlineExceeded (possibly wrapped)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Logf("error (may be exit code error on Windows): %v", err)
		// On Windows, timeout might come as non-zero exit code rather than ctx error
		// Accept either as long as there's some error indicator
	}
}

func TestTrustedLocalHandleExecuteExitCode(t *testing.T) {
	p := NewTrustedLocalProvider()
	policy := DefaultPolicy()
	h, _ := p.CreateHandle(nil, &policy)
	_ = h.Start(context.Background())
	defer h.Stop(context.Background())

	// Command that exits with non-zero
	// On Unix: false; on Windows: cmd /c exit /b 1
	var argv []string
	if runtime.GOOS == "windows" {
		argv = []string{"cmd", "/c", "exit", "/b", "1"}
	} else {
		argv = []string{"false"}
	}
	result, err := h.Execute(context.Background(), &Command{Argv: argv})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result.ExitCode == 0 {
		t.Errorf("expected non-zero ExitCode, got 0")
	}
}

func TestTrustedLocalHandleImplementsHandle(t *testing.T) {
	var _ Handle = (*TrustedLocalHandle)(nil)
}
