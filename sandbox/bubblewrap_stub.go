//go:build !linux

package sandbox

// BubblewrapProvider is a stub on non-linux platforms.
type BubblewrapProvider struct{}

// NewBubblewrapProvider returns a stub BubblewrapProvider on non-linux platforms.
func NewBubblewrapProvider() *BubblewrapProvider {
	return &BubblewrapProvider{}
}

func (p *BubblewrapProvider) Name() string { return "bubblewrap" }
func (p *BubblewrapProvider) Kind() string { return "local" }

// InspectAvailability reports bubblewrap is unavailable on non-linux platforms.
func (p *BubblewrapProvider) InspectAvailability() (*AvailabilityResult, error) {
	return &AvailabilityResult{
		Available:    false,
		Platform:     string(OSLinux),
		ErrorMessage: "bubblewrap not available on " + string(DetectOS()),
	}, nil
}

// CreateHandle always returns ErrProviderUnavailable on non-linux platforms.
func (p *BubblewrapProvider) CreateHandle(cfg map[string]any, policy *ExecutionPolicy) (Handle, error) {
	return nil, ErrProviderUnavailable
}

// RegisterBubblewrapProvider is a no-op stub on non-linux platforms.
func RegisterBubblewrapProvider(m *Manager) error {
	return ErrProviderUnavailable
}

// 编译期断言：BubblewrapProvider 满足 Provider 接口
var _ Provider = (*BubblewrapProvider)(nil)
