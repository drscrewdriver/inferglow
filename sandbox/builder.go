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

// ProviderBuilder assembles and registers sandbox Providers
// according to the current platform. It guarantees that at least
// trusted_local is always registered.
type ProviderBuilder struct {
	manager *Manager
}

// NewProviderBuilder creates a new ProviderBuilder with an empty Manager.
func NewProviderBuilder() *ProviderBuilder {
	return &ProviderBuilder{
		manager: NewManager(),
	}
}

// Build registers all available Providers (trusted_local + Docker +
// GVisor + platform-specific) and returns the Manager. It never fails
// — at minimum trusted_local is always available.
func (b *ProviderBuilder) Build() (*Manager, error) {
	// Always register trusted_local (minimum guarantee)
	_ = b.manager.Register(NewTrustedLocalProvider())

	// Try Docker (skip if unavailable)
	if dp, err := NewDockerProvider(); err == nil {
		_ = b.manager.Register(dp)
	}

	// Try GVisor (skip if unavailable)
	if gp, err := NewGVisorProvider(); err == nil {
		_ = b.manager.Register(gp)
	}

	// Register platform-specific providers (stub on non-target platforms)
	_ = RegisterSeatbeltProvider(b.manager)
	_ = RegisterWindowsRuntimeProvider(b.manager)

	return b.manager, nil
}

// BuildMinimal registers only trusted_local and Docker (if available),
// skipping all platform-specific Providers.
func (b *ProviderBuilder) BuildMinimal() *Manager {
	_ = b.manager.Register(NewTrustedLocalProvider())
	if dp, err := NewDockerProvider(); err == nil {
		_ = b.manager.Register(dp)
	}
	return b.manager
}
