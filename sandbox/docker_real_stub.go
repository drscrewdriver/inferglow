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
