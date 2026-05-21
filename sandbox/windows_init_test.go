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
