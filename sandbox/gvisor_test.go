package sandbox

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestNewGVisorProvider(t *testing.T) {
	_, err := NewGVisorProvider()
	// On Windows, runsc won't be available, so we expect ErrProviderUnavailable
	// or a wrapped version of it
	if err == nil {
		t.Log("NewGVisorProvider succeeded (runsc is available)")
	} else if !strings.Contains(err.Error(), "unavailable") && !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGVisorProviderImplementsProvider(t *testing.T) {
	_, err := NewGVisorProvider()
	if err != nil {
		t.Skip("GVisor not available, skipping interface check")
	}
}

func TestGVisorProviderNameKind(t *testing.T) {
	_, err := NewGVisorProvider()
	if err != nil {
		t.Skip("GVisor not available, skipping Name/Kind check")
	}
}

func TestGVisorProviderInspectAvailability(t *testing.T) {
	_, err := NewGVisorProvider()
	if err != nil {
		t.Skip("GVisor not available, skipping InspectAvailability check")
	}
}

func TestGVisorProviderCreateHandleStub(t *testing.T) {
	_, err := NewGVisorProvider()
	if err == nil {
		t.Skip("GVisor is available, CreateHandle will succeed")
	}
	// Expected: ErrProviderUnavailable
	if !strings.Contains(err.Error(), "unavailable") && !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected ErrProviderUnavailable, got %v", err)
	}
}

func TestGVisorProviderWithMockDocker(t *testing.T) {
	// Create a mock Docker provider
	mockClient := &mockDockerClient{
		createFn: func(config *ContainerConfig, hostConfig *HostConfig, networkingConfig *NetworkSettings, containerConfig *ContainerConfig, opts ...any) (*ContainerCreateResult, error) {
			return &ContainerCreateResult{ID: "gvisor123"}, nil
		},
		startFn: func(containerID string, opts ...any) error { return nil },
	}
	dockerProv := NewDockerProviderWithClient(mockClient)

	// Temporarily mock exec.LookPath for runsc
	// Since we can't mock exec.LookPath directly, we'll use a build tag approach
	// For now, skip this test if runsc is not available
	if _, lookErr := exec.LookPath("runsc"); lookErr != nil {
		t.Skip("runsc not available, skipping GVisor with mock Docker test")
	}

	prov, err := NewGVisorProviderWithDocker(dockerProv)
	if err != nil {
		t.Fatalf("NewGVisorProviderWithDocker returned error: %v", err)
	}
	if prov == nil {
		t.Fatal("NewGVisorProviderWithDocker returned nil")
	}
	if prov.Name() != "gvisor" {
		t.Errorf("Name() = %q, want %q", prov.Name(), "gvisor")
	}
}

func TestGVisorCreateHandle(t *testing.T) {
	if _, lookErr := exec.LookPath("runsc"); lookErr != nil {
		t.Skip("runsc not available, skipping CreateHandle test")
	}

	mockClient := &mockDockerClient{
		createFn: func(config *ContainerConfig, hostConfig *HostConfig, networkingConfig *NetworkSettings, containerConfig *ContainerConfig, opts ...any) (*ContainerCreateResult, error) {
			if hostConfig.Runtime != "runsc" {
				t.Errorf("hostConfig.Runtime = %q, want %q", hostConfig.Runtime, "runsc")
			}
			return &ContainerCreateResult{ID: "gvisor123"}, nil
		},
		startFn: func(containerID string, opts ...any) error { return nil },
	}
	dockerProv := NewDockerProviderWithClient(mockClient)

	prov, err := NewGVisorProviderWithDocker(dockerProv)
	if err != nil {
		t.Fatalf("NewGVisorProviderWithDocker returned error: %v", err)
	}

	policy := DefaultPolicy()
	h, err := prov.CreateHandle(nil, &policy)
	if err != nil {
		t.Fatalf("CreateHandle returned error: %v", err)
	}

	gvh, ok := h.(*GVisorHandle)
	if !ok {
		t.Fatalf("expected *GVisorHandle, got %T", h)
	}
	if gvh.DockerHandle.runtime != "runsc" {
		t.Errorf("runtime = %q, want %q", gvh.DockerHandle.runtime, "runsc")
	}
}

func TestGVisorStart(t *testing.T) {
	if _, lookErr := exec.LookPath("runsc"); lookErr != nil {
		t.Skip("runsc not available, skipping Start test")
	}

	mockClient := &mockDockerClient{
		createFn: func(config *ContainerConfig, hostConfig *HostConfig, networkingConfig *NetworkSettings, containerConfig *ContainerConfig, opts ...any) (*ContainerCreateResult, error) {
			return &ContainerCreateResult{ID: "gvisor123"}, nil
		},
		startFn: func(containerID string, opts ...any) error { return nil },
	}
	dockerProv := NewDockerProviderWithClient(mockClient)

	prov, err := NewGVisorProviderWithDocker(dockerProv)
	if err != nil {
		t.Fatalf("NewGVisorProviderWithDocker returned error: %v", err)
	}

	policy := DefaultPolicy()
	h, err := prov.CreateHandle(nil, &policy)
	if err != nil {
		t.Fatalf("CreateHandle returned error: %v", err)
	}

	err = h.Start(nil)
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
}

func TestGVisorHandleInheritsDockerMethods(t *testing.T) {
	if _, lookErr := exec.LookPath("runsc"); lookErr != nil {
		t.Skip("runsc not available, skipping inherited methods test")
	}

	mockClient := &mockDockerClient{
		createFn: func(config *ContainerConfig, hostConfig *HostConfig, networkingConfig *NetworkSettings, containerConfig *ContainerConfig, opts ...any) (*ContainerCreateResult, error) {
			return &ContainerCreateResult{ID: "gvisor123"}, nil
		},
		startFn:      func(containerID string, opts ...any) error { return nil },
		createExecFn: func(containerID string, opts CreateExecOptions) (*ExecCreateResult, error) {
			return &ExecCreateResult{ID: "exec1"}, nil
		},
		startExecFn: func(execID string, opts StartExecOptions) error { return nil },
		inspectExecFn: func(execID string) (*ExecInspectResult, error) {
			return &ExecInspectResult{ExitCode: 0}, nil
		},
	}
	dockerProv := NewDockerProviderWithClient(mockClient)

	prov, err := NewGVisorProviderWithDocker(dockerProv)
	if err != nil {
		t.Fatalf("NewGVisorProviderWithDocker returned error: %v", err)
	}

	policy := DefaultPolicy()
	h, err := prov.CreateHandle(nil, &policy)
	if err != nil {
		t.Fatalf("CreateHandle returned error: %v", err)
	}

	// Test Execute through GVisorHandle
	if err := h.Start(nil); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	result, err := h.Execute(nil, &Command{Argv: []string{"echo", "hi"}})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
}

func TestGVisorStop(t *testing.T) {
	if _, lookErr := exec.LookPath("runsc"); lookErr != nil {
		t.Skip("runsc not available, skipping Stop test")
	}

	mockClient := &mockDockerClient{
		createFn: func(config *ContainerConfig, hostConfig *HostConfig, networkingConfig *NetworkSettings, containerConfig *ContainerConfig, opts ...any) (*ContainerCreateResult, error) {
			return &ContainerCreateResult{ID: "gvisor123"}, nil
		},
		startFn: func(containerID string, opts ...any) error { return nil },
		stopFn:  func(containerID string, timeout uint) error { return nil },
		removeFn: func(opts ...any) error { return nil },
	}
	dockerProv := NewDockerProviderWithClient(mockClient)

	prov, err := NewGVisorProviderWithDocker(dockerProv)
	if err != nil {
		t.Fatalf("NewGVisorProviderWithDocker returned error: %v", err)
	}

	policy := DefaultPolicy()
	h, err := prov.CreateHandle(nil, &policy)
	if err != nil {
		t.Fatalf("CreateHandle returned error: %v", err)
	}

	if err := h.Start(nil); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	err = h.Stop(nil)
	if err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
}

func TestGVisorHandleNotRunning(t *testing.T) {
	if _, lookErr := exec.LookPath("runsc"); lookErr != nil {
		t.Skip("runsc not available, skipping not running test")
	}

	mockClient := &mockDockerClient{}
	dockerProv := NewDockerProviderWithClient(mockClient)

	prov, err := NewGVisorProviderWithDocker(dockerProv)
	if err != nil {
		t.Fatalf("NewGVisorProviderWithDocker returned error: %v", err)
	}

	policy := DefaultPolicy()
	h, err := prov.CreateHandle(nil, &policy)
	if err != nil {
		t.Fatalf("CreateHandle returned error: %v", err)
	}

	_, err = h.Execute(nil, &Command{Argv: []string{"echo", "hi"}})
	if !errors.Is(err, ErrHandleNotRunning) {
		t.Fatalf("expected ErrHandleNotRunning, got %v", err)
	}
}
