//go:build !windows

package sandbox

// WindowsBackend is a stub type on non-windows platforms.
type WindowsBackend int

const (
	BackendRestrictedToken WindowsBackend = iota
	BackendAppContainer
	BackendWindowsSandbox
)

// WindowsRuntimeProvider is a stub on non-windows platforms.
type WindowsRuntimeProvider struct{}

// WindowsRuntimeConfig is a stub type on non-windows platforms.
type WindowsRuntimeConfig struct{}

// SharedFolder is a stub type on non-windows platforms.
type SharedFolder struct {
	HostPath    string
	SandboxPath string
	ReadOnly    bool
}

// NewWindowsRuntimeProvider returns a stub WindowsRuntimeProvider on non-windows platforms.
func NewWindowsRuntimeProvider() *WindowsRuntimeProvider {
	return &WindowsRuntimeProvider{}
}

func (p *WindowsRuntimeProvider) Name() string    { return "windows_runtime" }
func (p *WindowsRuntimeProvider) Kind() string    { return "local" }
func (p *WindowsRuntimeProvider) InspectAvailability() (*AvailabilityResult, error) {
	return &AvailabilityResult{Available: false, Platform: "windows", ErrorMessage: "windows runtime not available on " + string(DetectOS())}, nil
}
func (p *WindowsRuntimeProvider) CreateHandle(cfg map[string]any, policy *ExecutionPolicy) (Handle, error) {
	return nil, ErrProviderUnavailable
}

// 编译期断言：WindowsRuntimeProvider 满足 Provider 接口
var _ Provider = (*WindowsRuntimeProvider)(nil)
