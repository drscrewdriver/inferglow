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
//
// It embeds *DockerHandle and therefore inherits all container lifecycle
// behaviour, including the network_access policy enforcement in
// DockerHandle.Start(): when the policy NetworkAccess.Level is
// NetworkAccessNone, the container is created with NetworkDisabled=true so
// the gVisor sandbox has no network stack.
type GVisorHandle struct {
	*DockerHandle
}
