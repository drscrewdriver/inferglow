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
	"os/exec"
)

// WindowsBackend represents one of the three supported Windows isolation backends.
type WindowsBackend int

const (
	// BackendRestrictedToken uses a restricted access token for process-level isolation.
	// Lowest overhead, weakest isolation.
	BackendRestrictedToken WindowsBackend = iota

	// BackendAppContainer uses the Windows AppContainer API (StartAppContainerOperation)
	// for application-level isolation (UWP style).
	// Medium overhead, medium isolation.
	BackendAppContainer

	// BackendWindowsSandbox uses the Windows Sandbox API to launch an independent VM.
	// Highest overhead, strongest isolation.
	BackendWindowsSandbox
)

// String returns a human-readable name for the backend.
func (b WindowsBackend) String() string {
	switch b {
	case BackendRestrictedToken:
		return "restricted_token"
	case BackendAppContainer:
		return "appcontainer"
	case BackendWindowsSandbox:
		return "windows_sandbox"
	default:
		return "unknown"
	}
}

// WindowsRuntimeConfig holds configuration for the Windows Runtime Provider.
type WindowsRuntimeConfig struct {
	Backend          WindowsBackend `json:"backend"`
	AutoSelect       bool           `json:"auto_select"`
	SandboxDirectory string         `json:"sandbox_directory"`
	NetworkIsolation bool           `json:"network_isolation"`
	SharedFolders    []SharedFolder `json:"shared_folders"`
	Timeout          int            `json:"timeout"`
}

// SharedFolder defines a host-to-sandbox folder mapping.
type SharedFolder struct {
	HostPath    string `json:"host_path"`
	SandboxPath string `json:"sandbox_path"`
	ReadOnly    bool   `json:"read_only"`
}

// WindowsSandboxCapabilities holds the capability enumeration for AppContainer.
type WindowsSandboxCapabilities int

const (
	// CapFile is a placeholder for file system capability identifier.
	CapFile WindowsSandboxCapabilities = iota
	// CapRegistry is a placeholder for registry capability identifier.
	CapRegistry
	// CapDevice is a placeholder for device capability identifier.
	CapDevice
)

// WindowsCapability holds a unique identifier for an AppContainer capability.
type WindowsCapability struct {
	LUID uint64
}

// winAPIAvailable checks whether the Windows sandbox-related APIs are available.
func winAPIAvailable() bool {
	// Check if we're on Windows 8.1+ where AppContainer is available
	return win10OrLater()
}

// win10OrLater checks if the current system is Windows 10 or later.
func win10OrLater() bool {
	// On Windows 10+, all sandbox APIs are available.
	// The real implementation would use RtlGetVersion from ntdll.dll.
	return true
}

// windowsSandboxAvailable checks if Windows Sandbox is installed and available.
func windowsSandboxAvailable() bool {
	// Check if Microsoft.WindowsSandbox.exe exists in PATH
	_, err := exec.LookPath("Microsoft.WindowsSandbox.exe")
	return err == nil
}
