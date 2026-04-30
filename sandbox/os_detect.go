package sandbox

import (
	"runtime"
	"sort"
)

// OS identifies a host operating system that the sandbox framework can run on.
type OS string

const (
	OSDarwin  OS = "darwin"
	OSLinux   OS = "linux"
	OSWindows OS = "windows"
	OSUnknown OS = "unknown"
)

// ProviderKind identifies a concrete sandbox backend implementation.
type ProviderKind string

const (
	ProviderTrustedLocal   ProviderKind = "trusted_local"
	ProviderDocker         ProviderKind = "docker"
	ProviderGVisor         ProviderKind = "gvisor"
	ProviderSeatbelt       ProviderKind = "seatbelt"
	ProviderBubblewrap     ProviderKind = "bubblewrap"
	ProviderLandlock       ProviderKind = "landlock"
	ProviderWindowsRuntime ProviderKind = "windows_runtime"
	ProviderE2B            ProviderKind = "e2b"
)

// DetectOS returns the OS the current process is running on.
func DetectOS() OS {
	switch runtime.GOOS {
	case "darwin":
		return OSDarwin
	case "linux":
		return OSLinux
	case "windows":
		return OSWindows
	default:
		return OSUnknown
	}
}

// ProviderMatrix describes which ProviderKinds are supported on which OSes.
type ProviderMatrix struct {
	Supported map[OS]map[ProviderKind]bool
}

// NewProviderMatrix builds the default cross-platform provider support matrix.
//
// Cross-platform providers (TrustedLocal, Docker, GVisor, E2B) are available
// on all three desktop/server OSes; OS-specific providers (Seatbelt on darwin,
// Bubblewrap and Landlock on linux, WindowsRuntime on windows) are only
// available on their respective platforms.
func NewProviderMatrix() *ProviderMatrix {
	m := &ProviderMatrix{
		Supported: map[OS]map[ProviderKind]bool{},
	}
	allOSes := []OS{OSDarwin, OSLinux, OSWindows}
	setAll := func(kind ProviderKind) {
		for _, os := range allOSes {
			if m.Supported[os] == nil {
				m.Supported[os] = map[ProviderKind]bool{}
			}
			m.Supported[os][kind] = true
		}
	}
	setOnly := func(kind ProviderKind, os OS) {
		if m.Supported[os] == nil {
			m.Supported[os] = map[ProviderKind]bool{}
		}
		m.Supported[os][kind] = true
	}
	setAll(ProviderTrustedLocal)
	setAll(ProviderDocker)
	setAll(ProviderGVisor)
	setAll(ProviderE2B)
	setOnly(ProviderSeatbelt, OSDarwin)
	setOnly(ProviderBubblewrap, OSLinux)
	setOnly(ProviderLandlock, OSLinux)
	setOnly(ProviderWindowsRuntime, OSWindows)
	return m
}

// DefaultProviderMatrix is the package-level shared provider support matrix.
var DefaultProviderMatrix = NewProviderMatrix()

// IsSupported reports whether kind is supported on os.
// Safe to call on a nil receiver (returns false).
func (m *ProviderMatrix) IsSupported(kind ProviderKind, os OS) bool {
	if m == nil || m.Supported == nil {
		return false
	}
	return m.Supported[os][kind]
}

// AvailableProviders returns the ProviderKinds available on os, sorted in
// ascending lexicographic order by their string form. Safe to call on a nil
// receiver (returns an empty, non-nil slice).
func (m *ProviderMatrix) AvailableProviders(os OS) []ProviderKind {
	if m == nil || m.Supported == nil {
		return []ProviderKind{}
	}
	kinds := m.Supported[os]
	result := make([]ProviderKind, 0, len(kinds))
	for k := range kinds {
		result = append(result, k)
	}
	sort.Slice(result, func(i, j int) bool {
		return string(result[i]) < string(result[j])
	})
	return result
}

// IsProviderSupportedOnOS is a package-level convenience wrapper around
// DefaultProviderMatrix.IsSupported.
func IsProviderSupportedOnOS(kind ProviderKind, os OS) bool {
	return DefaultProviderMatrix.IsSupported(kind, os)
}

// AvailableProvidersOnOS is a package-level convenience wrapper around
// DefaultProviderMatrix.AvailableProviders.
func AvailableProvidersOnOS(os OS) []ProviderKind {
	return DefaultProviderMatrix.AvailableProviders(os)
}
