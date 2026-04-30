//go:build !windows

package sandbox

import (
	"testing"
)

// TestSeatbeltProviderNotAvailableOnThisPlatform verifies that the Seatbelt
// Provider stub returns ErrProviderUnavailable on non-darwin platforms.
func TestSeatbeltProviderNotAvailableOnThisPlatform(t *testing.T) {
	// seatbelt.go.stub is compiled on !darwin
	p := NewSeatbeltProvider()
	if p == nil {
		t.Fatal("NewSeatbeltProvider returned nil")
	}
	avail, err := p.InspectAvailability()
	if err != nil && err != ErrProviderUnavailable {
		t.Fatalf("InspectAvailability returned unexpected error: %v", err)
	}
	if avail != nil && avail.Available {
		t.Fatal("Seatbelt should not be available on this platform")
	}
}

// TestSeatbeltInitStubReturnsError verifies that RegisterSeatbeltProvider
// on non-darwin returns ErrProviderUnavailable.
func TestSeatbeltInitStubReturnsError(t *testing.T) {
	mgr := NewManager()
	err := RegisterSeatbeltProvider(mgr)
	if err == nil {
		t.Fatal("expected error registering seatbelt on non-darwin")
	}
	if err != ErrProviderUnavailable {
		t.Fatalf("expected ErrProviderUnavailable, got %v", err)
	}
}

// TestWindowsRuntimeProviderNotAvailableOnThisPlatform verifies that the
// Windows Runtime Provider stub returns ErrProviderUnavailable on non-windows.
func TestWindowsRuntimeProviderNotAvailableOnThisPlatform(t *testing.T) {
	p := NewWindowsRuntimeProvider()
	if p == nil {
		t.Fatal("NewWindowsRuntimeProvider returned nil")
	}
	avail, err := p.InspectAvailability()
	if err != nil && err != ErrProviderUnavailable {
		t.Fatalf("InspectAvailability returned unexpected error: %v", err)
	}
	if avail != nil && avail.Available {
		t.Fatal("Windows Runtime should not be available on this platform")
	}
}

// TestWindowsRuntimeInitStubReturnsError verifies that
// RegisterWindowsRuntimeProvider on non-windows returns ErrProviderUnavailable.
func TestWindowsRuntimeInitStubReturnsError(t *testing.T) {
	mgr := NewManager()
	err := RegisterWindowsRuntimeProvider(mgr)
	if err == nil {
		t.Fatal("expected error registering windows_runtime on non-windows")
	}
	if err != ErrProviderUnavailable {
		t.Fatalf("expected ErrProviderUnavailable, got %v", err)
	}
}
