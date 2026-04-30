package sandbox

import (
	"fmt"
	"os/exec"
)

// GVisorProvider is a sandbox Provider backed by gVisor (runsc) on top of Docker.
type GVisorProvider struct {
	dockerProvider *DockerProvider
}

// NewGVisorProvider creates a GVisorProvider.
// It returns ErrProviderUnavailable if Docker is not available or runsc is not on PATH.
func NewGVisorProvider() (*GVisorProvider, error) {
	dockerProv, err := NewDockerProvider()
	if err != nil {
		return nil, err
	}
	// Check runsc availability
	if _, err := exec.LookPath("runsc"); err != nil {
		return nil, fmt.Errorf("%w: runsc not found in PATH", ErrProviderUnavailable)
	}
	return &GVisorProvider{
		dockerProvider: dockerProv,
	}, nil
}

// NewGVisorProviderWithDocker creates a GVisorProvider using an existing DockerProvider.
// Useful for testing with a mock DockerProvider.
func NewGVisorProviderWithDocker(dockerProv *DockerProvider) (*GVisorProvider, error) {
	if _, err := exec.LookPath("runsc"); err != nil {
		return nil, fmt.Errorf("%w: runsc not found in PATH", ErrProviderUnavailable)
	}
	return &GVisorProvider{
		dockerProvider: dockerProv,
	}, nil
}

// Name returns "gvisor".
func (p *GVisorProvider) Name() string { return "gvisor" }

// Kind returns "sandbox".
func (p *GVisorProvider) Kind() string { return "sandbox" }

// InspectAvailability checks whether both docker and runsc are available.
func (p *GVisorProvider) InspectAvailability() (*AvailabilityResult, error) {
	dockerAvail, err := p.dockerProvider.InspectAvailability()
	if err != nil || !dockerAvail.Available {
		return &AvailabilityResult{
			Available:    false,
			Platform:     string(DetectOS()),
			ErrorMessage: "docker not available",
		}, nil
	}
	path, err := exec.LookPath("runsc")
	if err != nil {
		return &AvailabilityResult{
			Available:    false,
			Platform:     string(DetectOS()),
			ErrorMessage: "runsc not found in PATH",
		}, nil
	}
	return &AvailabilityResult{
		Available:  true,
		Platform:   string(DetectOS()),
		BinaryPath: path,
	}, nil
}

// CreateHandle creates a GVisorHandle that wraps a DockerHandle with runsc runtime.
func (p *GVisorProvider) CreateHandle(cfg map[string]any, policy *ExecutionPolicy) (Handle, error) {
	dockerHandle, err := p.dockerProvider.CreateHandle(cfg, policy)
	if err != nil {
		return nil, err
	}
	// Cast to DockerHandle to set the runtime
	dh, ok := dockerHandle.(*DockerHandle)
	if !ok {
		return nil, fmt.Errorf("expected *DockerHandle, got %T", dockerHandle)
	}
	dh.runtime = "runsc"
	return &GVisorHandle{
		DockerHandle: dh,
	}, nil
}

// GVisorHandle is a Handle backed by a Docker container running with the runsc (gVisor) runtime.
type GVisorHandle struct {
	*DockerHandle
}
