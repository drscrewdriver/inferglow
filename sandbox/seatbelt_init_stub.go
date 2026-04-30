//go:build !darwin

package sandbox

// RegisterSeatbeltProvider is a no-op stub on non-darwin platforms.
func RegisterSeatbeltProvider(m *Manager) error {
	return ErrProviderUnavailable
}
