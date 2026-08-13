// Copyright 2026 InferGlow Authors
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

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
//
// The backend may be given as an int (WindowsBackend value) or as a string
// ("restricted_token", "appcontainer", "windows_sandbox").
func (p *WindowsRuntimeProvider) CreateHandle(cfg map[string]any, policy *ExecutionPolicy) (Handle, error) {
	// Determine backend
	backend := BackendRestrictedToken // default
	if b, ok := cfg["backend"].(int); ok {
		backend = WindowsBackend(b)
	} else if s, ok := cfg["backend"].(string); ok {
		switch s {
		case "appcontainer":
			backend = BackendAppContainer
		case "windows_sandbox":
			backend = BackendWindowsSandbox
		default:
			backend = BackendRestrictedToken
		}
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

// selectStrongestAvailable returns the strongest available backend for
// auto-select. Windows Sandbox is intentionally excluded: it requires an
// enterprise edition, always opens a visible window (no headless startup),
// and its host communication model is not fully supported. Ordering:
// AppContainer > RestrictedToken.
func (p *WindowsRuntimeProvider) selectStrongestAvailable() WindowsBackend {
	// Check in strongest-first order.
	strongest := []WindowsBackend{BackendAppContainer, BackendRestrictedToken}

	for _, b := range strongest {
		for _, avail := range p.availableBackends {
			if avail == b {
				return b
			}
		}
	}

	// Fallback to first available.
	if len(p.availableBackends) > 0 {
		return p.availableBackends[0]
	}

	return BackendRestrictedToken
}
