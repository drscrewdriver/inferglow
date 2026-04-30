//go:build !windows

package sandbox

// RegisterWindowsRuntimeProvider is a no-op stub on non-windows platforms.
func RegisterWindowsRuntimeProvider(m *Manager) error {
	return ErrProviderUnavailable
}
