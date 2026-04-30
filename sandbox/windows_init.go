//go:build windows

package sandbox

// RegisterWindowsRuntimeProvider registers the Windows Runtime Provider.
func RegisterWindowsRuntimeProvider(m *Manager) error {
	p := NewWindowsRuntimeProvider()
	return m.Register(p)
}
