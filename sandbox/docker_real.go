//go:build !skip_docker_real

package sandbox

import (
	"fmt"
	"io"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
)

// realDockerClient implements DockerClient using the real Docker SDK.
type realDockerClient struct {
	*client.Client
}

// ContainerCreate wraps the Docker SDK call.
func (r *realDockerClient) ContainerCreate(config *ContainerConfig, hostConfig *HostConfig, networkingConfig *NetworkSettings, containerConfig *ContainerConfig, opts ...any) (*ContainerCreateResult, error) {
	dockerConfig := &container.Config{
		Image:     config.Image,
		Env:       config.Env,
		WorkingDir: config.WorkingDir,
	}

	dockerHostConfig := &container.HostConfig{
		NetworkMode: container.NetworkMode(hostConfig.NetworkMode),
		Runtime:     hostConfig.Runtime,
	}

	// Add port bindings
	for portStr, mappings := range hostConfig.PortBindings {
		for _, m := range mappings {
			dockerHostConfig.PortBindings = map[string][]container.PortBinding{}
			if dockerHostConfig.PortBindings == nil {
				dockerHostConfig.PortBindings = map[string][]container.PortBinding{}
			}
			dockerHostConfig.PortBindings[portStr] = append(
				dockerHostConfig.PortBindings[portStr],
				container.PortBinding{HostIP: m.HostIP, HostPort: m.HostPort},
			)
		}
	}

	// Add bind mounts
	for _, bind := range hostConfig.Binds {
		parts := strings.SplitN(bind, ":", 2)
		if len(parts) == 2 {
			dockerHostConfig.Mounts = append(dockerHostConfig.Mounts, mount.Mount{
				Type:        mount.TypeBind,
				Source:      parts[0],
				Target:      parts[1],
				ReadOnly:    len(parts) > 2 && parts[2] == "ro",
			})
		}
	}

	resp, err := r.Client.ContainerCreate(nil, dockerConfig, dockerHostConfig, nil, nil, "")
	if err != nil {
		return nil, err
	}
	return &ContainerCreateResult{ID: resp.ID}, nil
}

// ContainerStart wraps the Docker SDK call.
func (r *realDockerClient) ContainerStart(containerID string, opts ...any) error {
	return r.Client.ContainerStart(nil, containerID, container.StartOptions{})
}

// CreateExec wraps the Docker SDK call.
func (r *realDockerClient) CreateExec(containerID string, opts CreateExecOptions) (*ExecCreateResult, error) {
	execConfig := container.ExecOptions{
		Cmd:          opts.Cmd,
		AttachStdout: opts.Stdout,
		AttachStderr: opts.Stderr,
		Stdin:        opts.Stdin,
		Tty:          opts.Tty,
		WorkingDir:   opts.WorkingDir,
	}
	resp, err := r.Client.ContainerExecCreate(nil, containerID, execConfig)
	if err != nil {
		return nil, err
	}
	return &ExecCreateResult{ID: resp.ID}, nil
}

// StartExec wraps the Docker SDK call.
func (r *realDockerClient) StartExec(execID string, opts StartExecOptions) error {
	return r.Client.ContainerExecStart(nil, execID, container.ExecStartOptions{
		Detach: !opts.Stdout && !opts.Stderr,
		Tty:    opts.Tty,
	})
}

// StopContainer wraps the Docker SDK call.
func (r *realDockerClient) StopContainer(containerID string, timeout uint) error {
	timeoutSec := int(timeout)
	return r.Client.ContainerStop(nil, containerID, container.StopOptions{
		Timeout: &timeoutSec,
	})
}

// RemoveContainer wraps the Docker SDK call.
func (r *realDockerClient) RemoveContainer(opts ...any) error {
	return r.Client.ContainerRemove(nil, "", container.RemoveOptions{Force: true})
}

// InspectContainer wraps the Docker SDK call.
func (r *realDockerClient) InspectContainer(containerID string) (*ContainerInspectResult, error) {
	info, err := r.Client.ContainerInspect(nil, containerID)
	if err != nil {
		return nil, err
	}
	return &ContainerInspectResult{
		ID:     info.ID,
		State:  ContainerState{Running: info.State.Running, ExitCode: info.State.ExitCode, Status: info.State.Status},
		Config: ContainerConfig{Image: info.Config.Image, Env: info.Config.Env},
	}, nil
}

// InspectExec wraps the Docker SDK call.
func (r *realDockerClient) InspectExec(execID string) (*ExecInspectResult, error) {
	info, err := r.Client.ContainerExecInspect(nil, execID)
	if err != nil {
		return nil, err
	}
	return &ExecInspectResult{
		ID:          execID,
		Running:     info.Running,
		ExitCode:    int(info.ExitCode),
		ContainerID: info.ContainerID,
	}, nil
}

// newRealDockerClientImpl creates a real Docker client from environment variables.
func newRealDockerClientImpl() (DockerClient, error) {
	dc, err := client.NewClientWithOpts(client.FromEnv, client.WithVersion("auto"))
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}
	// Verify connectivity
	ping, err := dc.Ping(nil)
	if err != nil {
		return nil, fmt.Errorf("docker daemon not reachable: %w", err)
	}
	_ = ping
	return &realDockerClient{Client: dc}, nil
}

// Ensure realDockerClient implements DockerClient.
var _ DockerClient = (*realDockerClient)(nil)

// ReadAll is a helper to read all bytes from an io.Reader.
func ReadAll(r io.Reader) ([]byte, error) {
	return io.ReadAll(r)
}
