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
