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

//go:build skip_docker_real

package sandbox

import "fmt"

// skipDockerClient is a no-op implementation when real Docker is disabled.
type skipDockerClient struct{}

func (s *skipDockerClient) ContainerCreate(config *ContainerConfig, hostConfig *HostConfig, networkingConfig *NetworkSettings, containerConfig *ContainerConfig, opts ...any) (*ContainerCreateResult, error) {
	return nil, fmt.Errorf("docker not available (built with skip_docker_real)")
}
func (s *skipDockerClient) ContainerStart(containerID string, opts ...any) error {
	return fmt.Errorf("docker not available (built with skip_docker_real)")
}
func (s *skipDockerClient) CreateExec(containerID string, opts CreateExecOptions) (*ExecCreateResult, error) {
	return nil, fmt.Errorf("docker not available (built with skip_docker_real)")
}
func (s *skipDockerClient) StartExec(execID string, opts StartExecOptions) error {
	return fmt.Errorf("docker not available (built with skip_docker_real)")
}
func (s *skipDockerClient) StopContainer(containerID string, timeout uint) error {
	return fmt.Errorf("docker not available (built with skip_docker_real)")
}
func (s *skipDockerClient) RemoveContainer(opts ...any) error {
	return fmt.Errorf("docker not available (built with skip_docker_real)")
}
func (s *skipDockerClient) InspectContainer(containerID string) (*ContainerInspectResult, error) {
	return nil, fmt.Errorf("docker not available (built with skip_docker_real)")
}
func (s *skipDockerClient) InspectExec(execID string) (*ExecInspectResult, error) {
	return nil, fmt.Errorf("docker not available (built with skip_docker_real)")
}

// Ensure skipDockerClient implements DockerClient.
var _ DockerClient = (*skipDockerClient)(nil)

// newRealDockerClientImpl returns a no-op client when Docker is disabled.
func newRealDockerClientImpl() (DockerClient, error) {
	return nil, fmt.Errorf("docker not available (built with skip_docker_real)")
}
