package sandbox

// ProviderBuilder assembles and registers sandbox Providers
// according to the current platform. It guarantees that at least
// trusted_local is always registered.
type ProviderBuilder struct {
	manager *Manager
}

// NewProviderBuilder creates a new ProviderBuilder with an empty Manager.
func NewProviderBuilder() *ProviderBuilder {
	return &ProviderBuilder{
		manager: NewManager(),
	}
}

// Build registers all available Providers (trusted_local + Docker +
// GVisor + platform-specific) and returns the Manager. It never fails
// — at minimum trusted_local is always available.
func (b *ProviderBuilder) Build() (*Manager, error) {
	// Always register trusted_local (minimum guarantee)
	_ = b.manager.Register(NewTrustedLocalProvider())

	// Try Docker (skip if unavailable)
	if dp, err := NewDockerProvider(); err == nil {
		_ = b.manager.Register(dp)
	}

	// Try GVisor (skip if unavailable)
	if gp, err := NewGVisorProvider(); err == nil {
		_ = b.manager.Register(gp)
	}

	// Register platform-specific providers (stub on non-target platforms)
	_ = RegisterSeatbeltProvider(b.manager)
	_ = RegisterWindowsRuntimeProvider(b.manager)

	return b.manager, nil
}

// BuildMinimal registers only trusted_local and Docker (if available),
// skipping all platform-specific Providers.
func (b *ProviderBuilder) BuildMinimal() *Manager {
	_ = b.manager.Register(NewTrustedLocalProvider())
	if dp, err := NewDockerProvider(); err == nil {
		_ = b.manager.Register(dp)
	}
	return b.manager
}
