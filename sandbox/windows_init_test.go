//go:build windows

package sandbox

import (
	"testing"
)

// TestWindowsRuntimeProviderAvailableOnWindows verifies that the
// Windows Runtime Provider is available on Windows.
func TestWindowsRuntimeProviderAvailableOnWindows(t *testing.T) {
	p := NewWindowsRuntimeProvider()
	if p == nil {
		t.Fatal("NewWindowsRuntimeProvider returned nil")
	}
	avail, err := p.InspectAvailability()
	if err != nil {
		t.Fatalf("InspectAvailability returned unexpected error: %v", err)
	}
	if avail == nil || !avail.Available {
		t.Fatal("Windows Runtime should be available on this platform")
	}
}

// TestWindowsRuntimeInitRegistered verifies that RegisterWindowsRuntimeProvider
// on Windows does not return an error.
func TestWindowsRuntimeInitRegistered(t *testing.T) {
	mgr := NewManager()
	err := RegisterWindowsRuntimeProvider(mgr)
	if err != nil {
		t.Fatalf("expected no error registering windows_runtime on Windows, got: %v", err)
	}
	names := mgr.List()
	found := false
	for _, n := range names {
		if n == "windows_runtime" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("windows_runtime not found in registered providers")
	}
}
