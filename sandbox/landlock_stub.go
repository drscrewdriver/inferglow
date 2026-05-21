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

//go:build !linux

package sandbox

// LandlockProvider is a stub on non-linux platforms.
type LandlockProvider struct{}

// NewLandlockProvider returns a stub LandlockProvider on non-linux platforms.
func NewLandlockProvider() *LandlockProvider {
	return &LandlockProvider{}
}

func (p *LandlockProvider) Name() string { return "landlock" }
func (p *LandlockProvider) Kind() string { return "local" }

// InspectAvailability reports landlock is unavailable on non-linux platforms.
func (p *LandlockProvider) InspectAvailability() (*AvailabilityResult, error) {
	return &AvailabilityResult{
		Available:    false,
		Platform:     string(OSLinux),
		ErrorMessage: "landlock not available on " + string(DetectOS()),
	}, nil
}

// CreateHandle always returns ErrProviderUnavailable on non-linux platforms.
func (p *LandlockProvider) CreateHandle(cfg map[string]any, policy *ExecutionPolicy) (Handle, error) {
	return nil, ErrProviderUnavailable
}

// RegisterLandlockProvider is a no-op stub on non-linux platforms.
func RegisterLandlockProvider(m *Manager) error {
	return ErrProviderUnavailable
}

// 编译期断言
var _ Provider = (*LandlockProvider)(nil)
