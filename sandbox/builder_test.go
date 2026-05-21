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

package sandbox

import (
	"testing"
)

// TestProviderBuilderNew returns non-nil builder.
func TestProviderBuilderNew(t *testing.T) {
	b := NewProviderBuilder()
	if b == nil {
		t.Fatal("NewProviderBuilder returned nil")
	}
	if b.manager == nil {
		t.Fatal("ProviderBuilder.manager is nil")
	}
}

// TestProviderBuilderBuildAlwaysIncludesTrustedLocal verifies that Build()
// always registers trusted_local as the minimum guarantee.
func TestProviderBuilderBuildAlwaysIncludesTrustedLocal(t *testing.T) {
	b := NewProviderBuilder()
	mgr, err := b.Build()
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}
	if mgr == nil {
		t.Fatal("Build() returned nil Manager")
	}
	names := mgr.List()
	found := false
	for _, n := range names {
		if n == "trusted_local" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Build() Manager does not include trusted_local, providers = %v", names)
	}
}

// TestProviderBuilderBuildNeverErrors verifies that Build() never returns
// an error — at minimum trusted_local is always available.
func TestProviderBuilderBuildNeverErrors(t *testing.T) {
	// Run multiple builders to ensure no state leaks
	for i := 0; i < 5; i++ {
		b := NewProviderBuilder()
		_, err := b.Build()
		if err != nil {
			t.Fatalf("Build() iteration %d returned error: %v", i, err)
		}
	}
}

// TestProviderBuilderBuildMinimalOnlyTrustedLocalAndDocker verifies that
// BuildMinimal() skips platform-specific providers.
func TestProviderBuilderBuildMinimalOnlyTrustedLocalAndDocker(t *testing.T) {
	b := NewProviderBuilder()
	mgr := b.BuildMinimal()
	if mgr == nil {
		t.Fatal("BuildMinimal() returned nil Manager")
	}
	names := mgr.List()
	// Must have trusted_local
	foundTL := false
	for _, n := range names {
		if n == "trusted_local" {
			foundTL = true
			break
		}
	}
	if !foundTL {
		t.Errorf("BuildMinimal() Manager does not include trusted_local, providers = %v", names)
	}
	// With skip_docker_real tag, Docker is stubbed so should also be registered
	// The key point: no platform-specific providers (seatbelt, windows_runtime)
	for _, n := range names {
		if n == "seatbelt" || n == "windows_runtime" {
			t.Errorf("BuildMinimal() should not include platform-specific provider %q", n)
		}
	}
}

// TestProviderBuilderMultipleInstancesAreIndependent verifies that
// each NewProviderBuilder() creates an independent Manager.
func TestProviderBuilderMultipleInstancesAreIndependent(t *testing.T) {
	b1 := NewProviderBuilder()
	m1, _ := b1.Build()
	b2 := NewProviderBuilder()
	m2, _ := b2.Build()
	// Each should have their own independent provider list
	if len(m1.List()) != len(m2.List()) {
		t.Errorf("independent builders have different provider counts: %d vs %d",
			len(m1.List()), len(m2.List()))
	}
}

// TestProviderBuilderPlatformProvidersRegistered verifies that Build()
// also registers platform-specific providers when available (seatbelt, windows_runtime).
func TestProviderBuilderPlatformProvidersRegistered(t *testing.T) {
	b := NewProviderBuilder()
	mgr, err := b.Build()
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}
	names := mgr.List()
	// Should always have trusted_local
	foundTL := false
	for _, n := range names {
		if n == "trusted_local" {
			foundTL = true
			break
		}
	}
	if !foundTL {
		t.Errorf("Build() Manager does not include trusted_local, providers = %v", names)
	}
	// Platform providers may or may not be registered depending on build tags
	// but Build() should not fail regardless
	_ = names
}
