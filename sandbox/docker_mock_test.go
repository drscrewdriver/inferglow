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
	"testing"
	"time"
)

// mockDockerClient implements DockerClient for testing.
type mockDockerClient struct {
	createFn      func(config *ContainerConfig, hostConfig *HostConfig, networkingConfig *NetworkSettings, containerConfig *ContainerConfig, opts ...any) (*ContainerCreateResult, error)
	startFn       func(containerID string, opts ...any) error
	createExecFn  func(containerID string, opts CreateExecOptions) (*ExecCreateResult, error)
	startExecFn   func(execID string, opts StartExecOptions) error
	stopFn        func(containerID string, timeout uint) error
	removeFn      func(opts ...any) error
	inspectContFn func(containerID string) (*ContainerInspectResult, error)
	inspectExecFn func(execID string) (*ExecInspectResult, error)
}

func (m *mockDockerClient) ContainerCreate(config *ContainerConfig, hostConfig *HostConfig, networkingConfig *NetworkSettings, containerConfig *ContainerConfig, opts ...any) (*ContainerCreateResult, error) {
	if m.createFn != nil {
		return m.createFn(config, hostConfig, networkingConfig, containerConfig, opts...)
	}
	return nil, nil
}
func (m *mockDockerClient) ContainerStart(containerID string, opts ...any) error {
	if m.startFn != nil {
		return m.startFn(containerID, opts...)
	}
	return nil
}
func (m *mockDockerClient) CreateExec(containerID string, opts CreateExecOptions) (*ExecCreateResult, error) {
	if m.createExecFn != nil {
		return m.createExecFn(containerID, opts)
	}
	return nil, nil
}
func (m *mockDockerClient) StartExec(execID string, opts StartExecOptions) error {
	if m.startExecFn != nil {
		return m.startExecFn(execID, opts)
	}
	return nil
}
func (m *mockDockerClient) StopContainer(containerID string, timeout uint) error {
	if m.stopFn != nil {
		return m.stopFn(containerID, timeout)
	}
	return nil
}
func (m *mockDockerClient) RemoveContainer(opts ...any) error {
	if m.removeFn != nil {
		return m.removeFn(opts...)
	}
	return nil
}
func (m *mockDockerClient) InspectContainer(containerID string) (*ContainerInspectResult, error) {
	if m.inspectContFn != nil {
		return m.inspectContFn(containerID)
	}
	return nil, nil
}
func (m *mockDockerClient) InspectExec(execID string) (*ExecInspectResult, error) {
	if m.inspectExecFn != nil {
		return m.inspectExecFn(execID)
	}
	return nil, nil
}

var _ DockerClient = (*mockDockerClient)(nil)

func TestNewDockerProviderWithClient(t *testing.T) {
	mc := &mockDockerClient{}
	p := NewDockerProviderWithClient(mc)
	if p == nil {
		t.Fatal("NewDockerProviderWithClient returned nil")
	}
	if p.Name() != "docker" {
		t.Errorf("Name() = %q, want %q", p.Name(), "docker")
	}
	if p.Kind() != "sandbox" {
		t.Errorf("Kind() = %q, want %q", p.Kind(), "sandbox")
	}
	if p.defaultNetwork != "bridge" {
		t.Errorf("defaultNetwork = %q, want %q", p.defaultNetwork, "bridge")
	}
}

func TestDockerProviderCreateHandle_Defaults(t *testing.T) {
	mc := &mockDockerClient{}
	p := NewDockerProviderWithClient(mc)
	policy := DefaultPolicy()
	h, err := p.CreateHandle(nil, &policy)
	if err != nil {
		t.Fatalf("CreateHandle returned error: %v", err)
	}
	dh, ok := h.(*DockerHandle)
	if !ok {
		t.Fatalf("expected *DockerHandle, got %T", h)
	}
	if dh.image != "alpine:latest" {
		t.Errorf("image = %q, want %q", dh.image, "alpine:latest")
	}
	if dh.network != "bridge" {
		t.Errorf("network = %q, want %q", dh.network, "bridge")
	}
	if dh.status != StatusCreated {
		t.Errorf("status = %q, want %q", dh.status, StatusCreated)
	}
}

func TestDockerProviderCreateHandle_CustomCfg(t *testing.T) {
	mc := &mockDockerClient{}
	p := NewDockerProviderWithClient(mc)
	policy := DefaultPolicy()
	cfg := map[string]any{
		"image":   "ubuntu:22.04",
		"network": "mynetwork",
		"ports":   []int{8080, 8443},
		"volumes": []string{"/host/data:/container/data"},
	}
	h, err := p.CreateHandle(cfg, &policy)
	if err != nil {
		t.Fatalf("CreateHandle returned error: %v", err)
	}
	dh := h.(*DockerHandle)
	if dh.image != "ubuntu:22.04" {
		t.Errorf("image = %q, want %q", dh.image, "ubuntu:22.04")
	}
	if dh.network != "mynetwork" {
		t.Errorf("network = %q, want %q", dh.network, "mynetwork")
	}
	if dh.ports == nil {
		t.Error("ports should not be nil")
	}
	if dh.volumes == nil {
		t.Error("volumes should not be nil")
	}
}

func TestDockerHandleStart_Success(t *testing.T) {
	mc := &mockDockerClient{
		createFn: func(config *ContainerConfig, hostConfig *HostConfig, networkingConfig *NetworkSettings, containerConfig *ContainerConfig, opts ...any) (*ContainerCreateResult, error) {
			if config.Image != "alpine:latest" {
				t.Errorf("create config.Image = %q, want %q", config.Image, "alpine:latest")
			}
			if hostConfig.NetworkMode != "bridge" {
				t.Errorf("hostConfig.NetworkMode = %q, want %q", hostConfig.NetworkMode, "bridge")
			}
			return &ContainerCreateResult{ID: "abc123"}, nil
		},
		startFn: func(containerID string, opts ...any) error {
			if containerID != "abc123" {
				t.Errorf("start containerID = %q, want %q", containerID, "abc123")
			}
			return nil
		},
	}
	p := NewDockerProviderWithClient(mc)
	policy := DefaultPolicy()
	h, _ := p.CreateHandle(nil, &policy)
	dh := h.(*DockerHandle)

	err := dh.Start(context.Background())
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if dh.containerID != "abc123" {
		t.Errorf("containerID = %q, want %q", dh.containerID, "abc123")
	}
	if dh.status != StatusRunning {
		t.Errorf("status = %q, want %q", dh.status, StatusRunning)
	}
}

func TestDockerHandleStart_CreateError(t *testing.T) {
	wantErr := errors.New("create failed")
	mc := &mockDockerClient{
		createFn: func(config *ContainerConfig, hostConfig *HostConfig, networkingConfig *NetworkSettings, containerConfig *ContainerConfig, opts ...any) (*ContainerCreateResult, error) {
			return nil, wantErr
		},
	}
	p := NewDockerProviderWithClient(mc)
	policy := DefaultPolicy()
	h, _ := p.CreateHandle(nil, &policy)
	dh := h.(*DockerHandle)

	err := dh.Start(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected error wrapping %v, got %v", wantErr, err)
	}
}

func TestDockerHandleStart_StartError(t *testing.T) {
	mc := &mockDockerClient{
		createFn: func(config *ContainerConfig, hostConfig *HostConfig, networkingConfig *NetworkSettings, containerConfig *ContainerConfig, opts ...any) (*ContainerCreateResult, error) {
			return &ContainerCreateResult{ID: "abc123"}, nil
		},
		startFn: func(containerID string, opts ...any) error {
			return errors.New("start failed")
		},
	}
	p := NewDockerProviderWithClient(mc)
	policy := DefaultPolicy()
	h, _ := p.CreateHandle(nil, &policy)
	dh := h.(*DockerHandle)

	err := dh.Start(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestDockerHandleStart_AlreadyStarted(t *testing.T) {
	mc := &mockDockerClient{
		createFn: func(config *ContainerConfig, hostConfig *HostConfig, networkingConfig *NetworkSettings, containerConfig *ContainerConfig, opts ...any) (*ContainerCreateResult, error) {
			return &ContainerCreateResult{ID: "abc123"}, nil
		},
		startFn: func(containerID string, opts ...any) error {
			return nil
		},
	}
	p := NewDockerProviderWithClient(mc)
	policy := DefaultPolicy()
	h, _ := p.CreateHandle(nil, &policy)
	dh := h.(*DockerHandle)

	if err := dh.Start(context.Background()); err != nil {
		t.Fatalf("first Start returned error: %v", err)
	}
	if err := dh.Start(context.Background()); err != nil {
		t.Fatalf("second Start returned error: %v", err)
	}
}

func TestDockerHandleExecute_NotRunning(t *testing.T) {
	mc := &mockDockerClient{}
	p := NewDockerProviderWithClient(mc)
	policy := DefaultPolicy()
	h, _ := p.CreateHandle(nil, &policy)
	dh := h.(*DockerHandle)

	_, err := dh.Execute(context.Background(), &Command{Argv: []string{"echo", "hi"}})
	if !errors.Is(err, ErrHandleNotRunning) {
		t.Fatalf("expected ErrHandleNotRunning, got %v", err)
	}
}

func TestDockerHandleExecute_Success(t *testing.T) {
	mc := &mockDockerClient{
		createFn: func(config *ContainerConfig, hostConfig *HostConfig, networkingConfig *NetworkSettings, containerConfig *ContainerConfig, opts ...any) (*ContainerCreateResult, error) {
			return &ContainerCreateResult{ID: "abc123"}, nil
		},
		startFn: func(containerID string, opts ...any) error { return nil },
		createExecFn: func(containerID string, opts CreateExecOptions) (*ExecCreateResult, error) {
			if containerID != "abc123" {
				t.Errorf("createExec containerID = %q, want %q", containerID, "abc123")
			}
			wantCmd := []string{"sh", "-c", "echo hi"}
			if len(opts.Cmd) != len(wantCmd) {
				t.Errorf("createExec Cmd = %v, want %v", opts.Cmd, wantCmd)
			}
			return &ExecCreateResult{ID: "exec1"}, nil
		},
		startExecFn: func(execID string, opts StartExecOptions) error { return nil },
		inspectExecFn: func(execID string) (*ExecInspectResult, error) {
			return &ExecInspectResult{ExitCode: 0}, nil
		},
	}
	p := NewDockerProviderWithClient(mc)
	policy := DefaultPolicy()
	h, _ := p.CreateHandle(nil, &policy)
	dh := h.(*DockerHandle)

	if err := dh.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	result, err := dh.Execute(context.Background(), &Command{Argv: []string{"echo", "hi"}})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
}

func TestDockerHandleExecute_PolicyTimeout(t *testing.T) {
	mc := &mockDockerClient{
		createFn: func(config *ContainerConfig, hostConfig *HostConfig, networkingConfig *NetworkSettings, containerConfig *ContainerConfig, opts ...any) (*ContainerCreateResult, error) {
			return &ContainerCreateResult{ID: "abc123"}, nil
		},
		startFn: func(containerID string, opts ...any) error { return nil },
		createExecFn: func(containerID string, opts CreateExecOptions) (*ExecCreateResult, error) {
			return &ExecCreateResult{ID: "exec1"}, nil
		},
		startExecFn: func(execID string, opts StartExecOptions) error { return nil },
		inspectExecFn: func(execID string) (*ExecInspectResult, error) {
			return &ExecInspectResult{ExitCode: 0}, nil
		},
	}
	p := NewDockerProviderWithClient(mc)
	policy := ExecutionPolicy{Timeout: 5 * time.Second}
	h, _ := p.CreateHandle(nil, &policy)
	dh := h.(*DockerHandle)

	if err := dh.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := dh.Execute(ctx, &Command{Argv: []string{"echo", "hi"}})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
}

func TestDockerHandleStop_Success(t *testing.T) {
	stopped := false
	mc := &mockDockerClient{
		createFn: func(config *ContainerConfig, hostConfig *HostConfig, networkingConfig *NetworkSettings, containerConfig *ContainerConfig, opts ...any) (*ContainerCreateResult, error) {
			return &ContainerCreateResult{ID: "abc123"}, nil
		},
		startFn: func(containerID string, opts ...any) error { return nil },
		stopFn: func(containerID string, timeout uint) error {
			stopped = true
			return nil
		},
		removeFn: func(opts ...any) error { return nil },
	}
	p := NewDockerProviderWithClient(mc)
	policy := DefaultPolicy()
	h, _ := p.CreateHandle(nil, &policy)
	dh := h.(*DockerHandle)

	if err := dh.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	err := dh.Stop(context.Background())
	if err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
	if !stopped {
		t.Error("StopContainer was not called")
	}
	if dh.containerID != "" {
		t.Errorf("containerID = %q, want empty", dh.containerID)
	}
	if dh.status != StatusStopped {
		t.Errorf("status = %q, want %q", dh.status, StatusStopped)
	}
}

func TestDockerHandleStop_NotStarted(t *testing.T) {
	mc := &mockDockerClient{}
	p := NewDockerProviderWithClient(mc)
	policy := DefaultPolicy()
	h, _ := p.CreateHandle(nil, &policy)
	dh := h.(*DockerHandle)

	err := dh.Stop(context.Background())
	if err != nil {
		t.Fatalf("Stop on not-started handle returned error: %v", err)
	}
	if dh.status != StatusStopped {
		t.Errorf("status = %q, want %q", dh.status, StatusStopped)
	}
}

func TestDockerHandleStatus(t *testing.T) {
	mc := &mockDockerClient{}
	p := NewDockerProviderWithClient(mc)
	policy := DefaultPolicy()
	h, _ := p.CreateHandle(nil, &policy)
	dh := h.(*DockerHandle)

	if dh.Status() != StatusCreated {
		t.Errorf("initial Status = %q, want %q", dh.Status(), StatusCreated)
	}
}

func TestDockerProviderImplementsProvider(t *testing.T) {
	mc := &mockDockerClient{}
	p := NewDockerProviderWithClient(mc)
	var _ Provider = p
}
