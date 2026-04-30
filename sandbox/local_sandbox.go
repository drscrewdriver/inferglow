package sandbox

import (
	"fmt"
)

// LocalSandboxProvider is a stub Provider for OS-native sandboxes
// (Seatbelt on macOS, Bubblewrap/Landlock on Linux, Windows Runtime
// on Windows). It is not implemented in P2/P3 initial — InspectAvailability
// always returns Available=false and CreateHandle returns
// ErrProviderUnavailable. Real platform-specific providers will be added
// in later specs.
type LocalSandboxProvider struct{}

// NewLocalSandboxProvider constructs a new LocalSandboxProvider.
func NewLocalSandboxProvider() *LocalSandboxProvider { return &LocalSandboxProvider{} }

// Name returns "local".
func (p *LocalSandboxProvider) Name() string { return "local" }

// Kind returns "local".
func (p *LocalSandboxProvider) Kind() string { return "local" }

// InspectAvailability always returns Available=false because the
// platform-specific local sandbox is not yet implemented.
func (p *LocalSandboxProvider) InspectAvailability() (*AvailabilityResult, error) {
	return &AvailabilityResult{
		Available:    false,
		Platform:     string(DetectOS()),
		ErrorMessage: "local sandbox (seatbelt/bwrap/windows) not implemented in P2/P3 initial",
	}, nil
}

// CreateHandle is a stub that always returns ErrProviderUnavailable.
func (p *LocalSandboxProvider) CreateHandle(cfg map[string]any, policy *ExecutionPolicy) (Handle, error) {
	return nil, fmt.Errorf("%w: local sandbox handle not implemented in P2/P3 initial", ErrProviderUnavailable)
}
