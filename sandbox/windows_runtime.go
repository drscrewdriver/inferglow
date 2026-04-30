//go:build windows

package sandbox

import (
	"fmt"
)

// WindowsRuntimeProvider implements Provider for Windows runtime isolation.
// It supports three backends: RestrictedToken, AppContainer, and WindowsSandbox.
type WindowsRuntimeProvider struct {
	availableBackends []WindowsBackend
}

// NewWindowsRuntimeProvider creates a provider and probes which backends are available.
func NewWindowsRuntimeProvider() *WindowsRuntimeProvider {
	var backends []WindowsBackend

	// RestrictedToken is always available on Windows NT
	backends = append(backends, BackendRestrictedToken)

	// AppContainer is available on Windows 8.1+
	if win10OrLater() {
		backends = append(backends, BackendAppContainer)
	}

	// Windows Sandbox is available only if the feature is installed
	if windowsSandboxAvailable() {
		backends = append(backends, BackendWindowsSandbox)
	}

	return &WindowsRuntimeProvider{
		availableBackends: backends,
	}
}

// Name returns "windows_runtime".
func (p *WindowsRuntimeProvider) Name() string {
	return "windows_runtime"
}

// Kind returns "local".
func (p *WindowsRuntimeProvider) Kind() string {
	return "local"
}

// InspectAvailability returns availability information for all backends.
func (p *WindowsRuntimeProvider) InspectAvailability() (*AvailabilityResult, error) {
	platform := "windows"
	available := len(p.availableBackends) > 0

	var backendNames []string
	for _, b := range p.availableBackends {
		backendNames = append(backendNames, b.String())
	}

	return &AvailabilityResult{
		Available:    available,
		Platform:     platform,
		ErrorMessage: "",
	}, nil
}

// CreateHandle parses the configuration and returns the appropriate Handle
// based on the backend specified in the config.
func (p *WindowsRuntimeProvider) CreateHandle(cfg map[string]any, policy *ExecutionPolicy) (Handle, error) {
	// Determine backend
	backend := BackendRestrictedToken // default
	if b, ok := cfg["backend"].(int); ok {
		backend = WindowsBackend(b)
	}

	autoSelect, _ := cfg["auto_select"].(bool)
	if autoSelect {
		backend = p.selectStrongestAvailable()
	}

	sandboxDir, _ := cfg["sandbox_directory"].(string)
	networkIsolation, _ := cfg["network_isolation"].(bool)

	switch backend {
	case BackendAppContainer:
		return &AppContainerHandle{
			config:           cfg,
			policy:           policy,
			status:           StatusCreated,
			sandboxDir:       sandboxDir,
			networkIsolation: networkIsolation,
		}, nil
	case BackendRestrictedToken:
		return &RestrictedTokenHandle{
			config:           cfg,
			policy:           policy,
			status:           StatusCreated,
			sandboxDir:       sandboxDir,
			networkIsolation: networkIsolation,
		}, nil
	case BackendWindowsSandbox:
		if !windowsSandboxAvailable() {
			return nil, fmt.Errorf("windows sandbox not available: %w", ErrProviderUnavailable)
		}
		return &WindowsSandboxHandle{
			config:           cfg,
			policy:           policy,
			status:           StatusCreated,
			sandboxDir:       sandboxDir,
			networkIsolation: networkIsolation,
		}, nil
	default:
		return nil, fmt.Errorf("unknown backend %d: %w", backend, ErrProviderUnavailable)
	}
}

// selectStrongestAvailable returns the strongest available backend for auto-select.
// Ordering: WindowsSandbox > AppContainer > RestrictedToken
func (p *WindowsRuntimeProvider) selectStrongestAvailable() WindowsBackend {
	// Check in strongest-first order
	strongest := []WindowsBackend{BackendWindowsSandbox, BackendAppContainer, BackendRestrictedToken}

	for _, b := range strongest {
		for _, avail := range p.availableBackends {
			if avail == b {
				return b
			}
		}
	}

	// Fallback to first available
	if len(p.availableBackends) > 0 {
		return p.availableBackends[0]
	}

	return BackendRestrictedToken
}
