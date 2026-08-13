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
	"syscall"
	"unsafe"
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

// osVersionInfoEx mirrors the Win32 OSVERSIONINFOEXW structure used by
// RtlGetVersion to report the real OS version (GetVersionEx lies under
// manifest-based version probing).
type osVersionInfoEx struct {
	osVersionInfoSize uint32
	majorVersion      uint32
	minorVersion      uint32
	buildNumber       uint32
	platformID        uint32
	csdVersion        [128]uint16
	servicePackMajor  uint16
	servicePackMinor  uint16
	suiteMask         uint16
	productType       byte
	reserved          byte
}

// win10OrLater reports whether the current OS is Windows 10 or later, which
// is the floor for AppContainer APIs. It queries RtlGetVersion from
// ntdll.dll; on failure it fails open (modern OS) so availability checks do
// not spuriously disable the backend on exotic systems.
func win10OrLater() bool {
	var info osVersionInfoEx
	info.osVersionInfoSize = uint32(unsafe.Sizeof(info))
	ntdll := syscall.NewLazyDLL("ntdll.dll")
	r1, _, _ := ntdll.NewProc("RtlGetVersion").Call(uintptr(unsafe.Pointer(&info)))
	if r1 != 0 {
		return true
	}
	return info.majorVersion >= 10
}

// windowsSandboxAvailable checks if Windows Sandbox is installed and available.
func windowsSandboxAvailable() bool {
	// Check if Microsoft.WindowsSandbox.exe exists in PATH
	_, err := exec.LookPath("Microsoft.WindowsSandbox.exe")
	return err == nil
}
