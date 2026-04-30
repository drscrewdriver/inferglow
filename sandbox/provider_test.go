package sandbox

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

// mockProvider implements Provider for testing.
type mockProvider struct {
	name string
	kind string
	avail *AvailabilityResult
	availErr error
	handle Handle
	handleErr error
}

func (m *mockProvider) Name() string { return m.name }
func (m *mockProvider) Kind() string { return m.kind }
func (m *mockProvider) InspectAvailability() (*AvailabilityResult, error) {
	return m.avail, m.availErr
}
func (m *mockProvider) CreateHandle(cfg map[string]any, policy *ExecutionPolicy) (Handle, error) {
	return m.handle, m.handleErr
}

// 编译期断言：mockProvider 满足 Provider 接口
var _ Provider = (*mockProvider)(nil)

// mockHandle implements Handle for testing.
type mockHandle struct {
	startErr error
	execResult *ExecutionResult
	execErr error
	stopErr error
	statusVal HandleStatus
}

func (h *mockHandle) Start(ctx context.Context) error { return h.startErr }
func (h *mockHandle) Execute(ctx context.Context, cmd *Command) (*ExecutionResult, error) {
	return h.execResult, h.execErr
}
func (h *mockHandle) Stop(ctx context.Context) error { return h.stopErr }
func (h *mockHandle) Status() HandleStatus { return h.statusVal }

// 编译期断言：mockHandle 满足 Handle 接口
var _ Handle = (*mockHandle)(nil)

func TestHandleStatusConstants(t *testing.T) {
	cases := []struct {
		name string
		got  HandleStatus
		want string
	}{
		{"StatusCreated", StatusCreated, "created"},
		{"StatusRunning", StatusRunning, "running"},
		{"StatusStopped", StatusStopped, "stopped"},
		{"StatusError", StatusError, "error"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if string(c.got) != c.want {
				t.Errorf("%s = %q, want %q", c.name, string(c.got), c.want)
			}
		})
	}
}

func TestProviderErrorVariables(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"ErrProviderNotFound", ErrProviderNotFound},
		{"ErrNoAvailableSandbox", ErrNoAvailableSandbox},
		{"ErrProviderUnavailable", ErrProviderUnavailable},
		{"ErrHandleNotRunning", ErrHandleNotRunning},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.err == nil {
				t.Fatalf("%s is nil", c.name)
			}
			if !errors.Is(c.err, c.err) {
				t.Errorf("%s does not satisfy errors.Is with itself", c.name)
			}
		})
	}
}

func TestAvailabilityResultStruct(t *testing.T) {
	r := AvailabilityResult{
		Available:    true,
		Platform:     "linux/amd64",
		BinaryPath:   "/usr/bin/docker",
		Version:      "24.0.7",
		ErrorMessage: "",
	}
	if !r.Available {
		t.Errorf("Available = false, want true")
	}
	if r.Platform != "linux/amd64" {
		t.Errorf("Platform = %q, want %q", r.Platform, "linux/amd64")
	}
	if r.BinaryPath != "/usr/bin/docker" {
		t.Errorf("BinaryPath = %q, want %q", r.BinaryPath, "/usr/bin/docker")
	}
	if r.Version != "24.0.7" {
		t.Errorf("Version = %q, want %q", r.Version, "24.0.7")
	}
	if r.ErrorMessage != "" {
		t.Errorf("ErrorMessage = %q, want empty", r.ErrorMessage)
	}
}

func TestCommandStruct(t *testing.T) {
	c := Command{
		Argv:    []string{"/bin/sh", "-c", "echo hi"},
		Env:     []string{"PATH=/usr/bin"},
		Workdir: "/tmp",
		Stdin:   strings.NewReader("input"),
	}
	if len(c.Argv) != 3 || c.Argv[0] != "/bin/sh" {
		t.Errorf("Argv = %+v, want [/bin/sh -c echo hi]", c.Argv)
	}
	if len(c.Env) != 1 || c.Env[0] != "PATH=/usr/bin" {
		t.Errorf("Env = %+v, want [PATH=/usr/bin]", c.Env)
	}
	if c.Workdir != "/tmp" {
		t.Errorf("Workdir = %q, want /tmp", c.Workdir)
	}
	if c.Stdin == nil {
		t.Errorf("Stdin is nil")
	}
	// 确保实现了 io.Reader 接口
	var _ io.Reader = c.Stdin
}

func TestExecutionResultStruct(t *testing.T) {
	r := ExecutionResult{
		ExitCode: 0,
		Stdout:   "hello\n",
		Stderr:   "",
		Duration: 100 * time.Millisecond,
	}
	if r.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", r.ExitCode)
	}
	if r.Stdout != "hello\n" {
		t.Errorf("Stdout = %q, want %q", r.Stdout, "hello\n")
	}
	if r.Stderr != "" {
		t.Errorf("Stderr = %q, want empty", r.Stderr)
	}
	if r.Duration != 100*time.Millisecond {
		t.Errorf("Duration = %v, want 100ms", r.Duration)
	}
}

func TestMockProviderImplementsProvider(t *testing.T) {
	// 运行时验证 mockProvider 实现 Provider 接口
	p := &mockProvider{name: "mock", kind: "mock"}
	var _ Provider = p
	if p.Name() != "mock" {
		t.Errorf("Name() = %q, want mock", p.Name())
	}
	if p.Kind() != "mock" {
		t.Errorf("Kind() = %q, want mock", p.Kind())
	}
}

func TestMockHandleImplementsHandle(t *testing.T) {
	h := &mockHandle{statusVal: StatusCreated}
	var _ Handle = h
	if h.Status() != StatusCreated {
		t.Errorf("Status() = %q, want %q", h.Status(), StatusCreated)
	}
}

func TestMockProviderCreateHandle(t *testing.T) {
	expectedHandle := &mockHandle{statusVal: StatusRunning}
	p := &mockProvider{
		name:   "docker",
		kind:   "docker",
		handle: expectedHandle,
	}
	policy := DefaultPolicy()
	h, err := p.CreateHandle(map[string]any{"image": "alpine"}, &policy)
	if err != nil {
		t.Fatalf("CreateHandle returned error: %v", err)
	}
	if h != expectedHandle {
		t.Errorf("CreateHandle returned wrong handle")
	}
	if h.Status() != StatusRunning {
		t.Errorf("handle.Status() = %q, want %q", h.Status(), StatusRunning)
	}
}

func TestMockProviderInspectAvailability(t *testing.T) {
	p := &mockProvider{
		name:  "docker",
		kind:  "docker",
		avail: &AvailabilityResult{Available: true, BinaryPath: "/usr/bin/docker"},
	}
	r, err := p.InspectAvailability()
	if err != nil {
		t.Fatalf("InspectAvailability returned error: %v", err)
	}
	if r == nil || !r.Available {
		t.Fatalf("InspectAvailability returned unavailable result: %+v", r)
	}
	if r.BinaryPath != "/usr/bin/docker" {
		t.Errorf("BinaryPath = %q, want /usr/bin/docker", r.BinaryPath)
	}
}
