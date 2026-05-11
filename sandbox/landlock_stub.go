//go:build !linux

package sandbox

// LandlockProvider is a stub on non-linux platforms.
type LandlockProvider struct{}

// NewLandlockProvider returns a stub LandlockProvider on non-linux platforms.
func NewLandlockProvider() *LandlockProvider {
	return &LandlockProvider{}
}

func (p *LandlockProvider) Name() string { return "landlock" }
func (p *LandlockProvider) Kind() string { return "local" }

// InspectAvailability reports landlock is unavailable on non-linux platforms.
func (p *LandlockProvider) InspectAvailability() (*AvailabilityResult, error) {
	return &AvailabilityResult{
		Available:    false,
		Platform:     string(OSLinux),
		ErrorMessage: "landlock not available on " + string(DetectOS()),
	}, nil
}

// CreateHandle always returns ErrProviderUnavailable on non-linux platforms.
func (p *LandlockProvider) CreateHandle(cfg map[string]any, policy *ExecutionPolicy) (Handle, error) {
	return nil, ErrProviderUnavailable
}

// RegisterLandlockProvider is a no-op stub on non-linux platforms.
func RegisterLandlockProvider(m *Manager) error {
	return ErrProviderUnavailable
}

// 编译期断言
var _ Provider = (*LandlockProvider)(nil)
