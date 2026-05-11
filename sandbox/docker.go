package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

// DockerClient abstracts the Docker API for testability.
type DockerClient interface {
	ContainerCreate(config *ContainerConfig, hostConfig *HostConfig, networkingConfig *NetworkSettings, containerConfig *ContainerConfig, opts ...any) (*ContainerCreateResult, error)
	ContainerStart(containerID string, opts ...any) error
	CreateExec(containerID string, opts CreateExecOptions) (*ExecCreateResult, error)
	StartExec(execID string, opts StartExecOptions) error
	StopContainer(containerID string, timeout uint) error
	RemoveContainer(opts ...any) error
	InspectContainer(containerID string) (*ContainerInspectResult, error)
	InspectExec(execID string) (*ExecInspectResult, error)
}

// ContainerConfig holds container creation parameters.
type ContainerConfig struct {
	Image      string
	Hostname   string
	Env        []string
	Cmd        []string
	WorkingDir string
	NetworkDisabled bool
}

// HostConfig holds host-specific container parameters.
type HostConfig struct {
	NetworkMode string
	PortBindings map[string][]PortMapping
	Binds        []string
	Runtime      string
}

// NetworkSettings holds network configuration.
type NetworkSettings struct {
	Networks map[string]NetworkEndpoint
}

// NetworkEndpoint holds endpoint-specific network config.
type NetworkEndpoint struct {
	IPAddress string
}

// ContainerCreateResult is the result of creating a container.
type ContainerCreateResult struct {
	ID string
	Warnings []string
}

// CreateExecOptions holds parameters for creating an exec instance.
type CreateExecOptions struct {
	Container    string
	Cmd          []string
	Stdin        bool
	Stdout       bool
	Stderr       bool
	Tty          bool
	WorkingDir   string
}

// ExecCreateResult is the result of creating an exec instance.
type ExecCreateResult struct {
	ID string
}

// StartExecOptions holds parameters for starting an exec instance.
type StartExecOptions struct {
	OutputStream io.Writer
	ErrorStream  io.Writer
	Stdout       bool
	Stderr       bool
	Stdin        bool
	Tty          bool
}

// ContainerInspectResult holds container inspection data.
type ContainerInspectResult struct {
	ID         string
	State      ContainerState
	Config     ContainerConfig
	NetworkSettings NetworkSettings
}

// ContainerState holds the state of a container.
type ContainerState struct {
	Running    bool
	ExitCode   int
	Status     string
}

// ExecInspectResult holds exec inspection data.
type ExecInspectResult struct {
	ID         string
	Running    bool
	ExitCode   int
	ContainerID string
}

// dockerClientAdapter wraps a real docker.Client for use with real Docker.
type dockerClientAdapter struct {
	impl DockerClient
}

func newDockerClientAdapter(impl DockerClient) *dockerClientAdapter {
	return &dockerClientAdapter{impl: impl}
}

// DockerProvider is a sandbox Provider backed by Docker containers.
type DockerProvider struct {
	client           DockerClient
	defaultNetwork   string
}

// NewDockerProvider creates a DockerProvider using the real Docker API via
// docker.FromEnv. If Docker is not available, it returns ErrProviderUnavailable.
func NewDockerProvider() (*DockerProvider, error) {
	client, err := newRealDockerClient()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProviderUnavailable, err)
	}
	return &DockerProvider{
		client:         client,
		defaultNetwork: "bridge",
	}, nil
}

// NewDockerProviderWithClient creates a DockerProvider with a mock DockerClient for testing.
func NewDockerProviderWithClient(client DockerClient) *DockerProvider {
	return &DockerProvider{
		client:         client,
		defaultNetwork: "bridge",
	}
}

// Name returns "docker".
func (p *DockerProvider) Name() string { return "docker" }

// Kind returns "sandbox".
func (p *DockerProvider) Kind() string { return "sandbox" }

// InspectAvailability checks whether the docker binary is on PATH.
func (p *DockerProvider) InspectAvailability() (*AvailabilityResult, error) {
	path, err := exec.LookPath("docker")
	if err != nil {
		return &AvailabilityResult{
			Available:    false,
			Platform:     string(DetectOS()),
			ErrorMessage: "docker not found in PATH",
		}, nil
	}
	return &AvailabilityResult{
		Available:  true,
		Platform:   string(DetectOS()),
		BinaryPath: path,
	}, nil
}

// CreateHandle parses the configuration and returns a DockerHandle.
func (p *DockerProvider) CreateHandle(cfg map[string]any, policy *ExecutionPolicy) (Handle, error) {
	image, _ := cfg["image"].(string)
	if image == "" {
		image = "alpine:latest"
	}
	network, _ := cfg["network"].(string)
	if network == "" {
		network = p.defaultNetwork
	}
	return &DockerHandle{
		client:     p.client,
		config:     cfg,
		policy:     policy,
		image:      image,
		network:    network,
		ports:      cfg["ports"],
		volumes:    cfg["volumes"],
		env:        cfg["env"],
		status:     StatusCreated,
	}, nil
}

// DockerHandle is a Handle backed by a Docker container.
type DockerHandle struct {
	client      DockerClient
	config      map[string]any
	policy      *ExecutionPolicy
	image       string
	network     string
	ports       any
	volumes     any
	env         any
	containerID string
	runtime     string
	mu          sync.Mutex
	status      HandleStatus
}

// Start creates and starts the Docker container.
func (h *DockerHandle) Start(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.containerID != "" {
		return nil // Already started
	}

	config := &ContainerConfig{
		Image:      h.image,
		NetworkDisabled: false,
		// Keep the container alive so that Execute can run commands in it.
		// Without an explicit long-running command, alpine's default /bin/sh
		// exits immediately when there is no TTY, causing the container to
		// stop before Execute can be invoked.
		Cmd: []string{"sleep", "infinity"},
	}
	if h.env != nil {
		switch e := h.env.(type) {
		case []string:
			config.Env = e
		case []any:
			for _, v := range e {
				if s, ok := v.(string); ok {
					config.Env = append(config.Env, s)
				}
			}
		}
	}

	hostConfig := &HostConfig{
		NetworkMode: h.network,
		Runtime:     h.runtime,
	}
	if h.ports != nil {
		switch ps := h.ports.(type) {
		case []int:
			hostConfig.PortBindings = make(map[string][]PortMapping)
			for _, port := range ps {
				hostConfig.PortBindings[fmt.Sprintf("%d/tcp", port)] = []PortMapping{{HostPort: fmt.Sprintf("%d", port)}}
			}
		case map[string]any:
			hostConfig.PortBindings = make(map[string][]PortMapping)
			for portStr, v := range ps {
				switch pv := v.(type) {
				case string:
					hostConfig.PortBindings[fmt.Sprintf("%s/tcp", portStr)] = []PortMapping{{HostPort: pv}}
				case int:
					hostConfig.PortBindings[fmt.Sprintf("%s/tcp", portStr)] = []PortMapping{{HostPort: fmt.Sprintf("%d", pv)}}
				}
			}
		}
	}
	if h.volumes != nil {
		switch vs := h.volumes.(type) {
		case []string:
			hostConfig.Binds = vs
		case []any:
			for _, v := range vs {
				if s, ok := v.(string); ok {
					hostConfig.Binds = append(hostConfig.Binds, s)
				}
			}
		}
	}

	resp, err := h.client.ContainerCreate(config, hostConfig, &NetworkSettings{Networks: map[string]NetworkEndpoint{h.network: {}}}, config)
	if err != nil {
		return fmt.Errorf("create container: %w", err)
	}
	h.containerID = resp.ID

	if err := h.client.ContainerStart(h.containerID); err != nil {
		return fmt.Errorf("start container: %w", err)
	}

	h.status = StatusRunning
	return nil
}

// Execute runs a command inside the running container.
func (h *DockerHandle) Execute(ctx context.Context, cmd *Command) (*ExecutionResult, error) {
	h.mu.Lock()
	if h.status != StatusRunning {
		h.mu.Unlock()
		return nil, ErrHandleNotRunning
	}
	h.mu.Unlock()

	// Apply policy timeout
	if h.policy != nil && h.policy.Timeout != 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, h.policy.Timeout)
		defer cancel()
	}

	argv := cmd.Argv
	if len(argv) == 0 {
		argv = []string{"sh", "-c", "true"}
	}

	// If only one arg, wrap as sh -c
	var execCmd []string
	if len(argv) == 1 {
		execCmd = []string{"sh", "-c", argv[0]}
	} else {
		// Join with spaces for sh -c
		execCmd = []string{"sh", "-c", joinArgv(argv)}
	}

	execConfig := CreateExecOptions{
		Container: h.containerID,
		Cmd:       execCmd,
		Stdin:     cmd.Stdin != nil,
		Stdout:    true,
		Stderr:    true,
		Tty:       false,
	}
	if cmd.Workdir != "" {
		execConfig.WorkingDir = cmd.Workdir
	}

	execResult, err := h.client.CreateExec(h.containerID, execConfig)
	if err != nil {
		return nil, fmt.Errorf("create exec: %w", err)
	}

	var stdout, stderr bytes.Buffer
	startOpts := StartExecOptions{
		OutputStream: &stdout,
		ErrorStream:  &stderr,
		Stdout:       true,
		Stderr:       true,
	}

	startErr := h.client.StartExec(execResult.ID, startOpts)

	// Get exit code from exec inspect
	var exitCode int
	inspect, inspectErr := h.client.InspectExec(execResult.ID)
	if inspectErr == nil {
		exitCode = inspect.ExitCode
	}

	result := &ExecutionResult{
		ExitCode: exitCode,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}

	if startErr != nil {
		// Check if it's a timeout
		if ctx.Err() == context.DeadlineExceeded {
			result.Stderr = "exec timed out"
			result.ExitCode = -1
			return result, ctx.Err()
		}
		return result, fmt.Errorf("start exec: %w", startErr)
	}

	return result, nil
}

// Stop stops and removes the container.
func (h *DockerHandle) Stop(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.containerID == "" {
		h.status = StatusStopped
		return nil
	}

	stopTimeout := uint(30)
	err := h.client.StopContainer(h.containerID, stopTimeout)

	// Force remove regardless of stop result
	removeErr := h.client.RemoveContainer()
	if removeErr != nil {
		h.status = StatusError
		return fmt.Errorf("stop container: %w; remove container: %w", err, removeErr)
	}

	h.containerID = ""
	h.status = StatusStopped
	return err
}

// Status returns the current handle status.
func (h *DockerHandle) Status() HandleStatus {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.status
}

// PortMapping represents a port binding.
type PortMapping struct {
	HostIP   string
	HostPort string
}

// newRealDockerClient creates a real Docker client.
func newRealDockerClient() (DockerClient, error) {
	return newRealDockerClientImpl()
}

// joinArgv joins command arguments into a shell-safe string.
func joinArgv(argv []string) string {
	if len(argv) == 0 {
		return "true"
	}
	result := argv[0]
	for _, a := range argv[1:] {
		// Simple escaping for shell safety
		result += " " + escapeArg(a)
	}
	return result
}

// escapeArg escapes a single argument for shell safety.
func escapeArg(arg string) string {
	// Use single quotes; escape existing single quotes
	result := "'"
	for _, c := range arg {
		if c == '\'' {
			result += `'\''`
		} else {
			result += string(c)
		}
	}
	result += "'"
	return result
}
