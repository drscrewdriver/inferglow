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

// BubblewrapProvider is a stub on non-linux platforms.
type BubblewrapProvider struct{}

// NewBubblewrapProvider returns a stub BubblewrapProvider on non-linux platforms.
func NewBubblewrapProvider() *BubblewrapProvider {
	return &BubblewrapProvider{}
}

func (p *BubblewrapProvider) Name() string { return "bubblewrap" }
func (p *BubblewrapProvider) Kind() string { return "local" }

// InspectAvailability reports bubblewrap is unavailable on non-linux platforms.
func (p *BubblewrapProvider) InspectAvailability() (*AvailabilityResult, error) {
	return &AvailabilityResult{
		Available:    false,
		Platform:     string(OSLinux),
		ErrorMessage: "bubblewrap not available on " + string(DetectOS()),
	}, nil
}

// CreateHandle always returns ErrProviderUnavailable on non-linux platforms.
func (p *BubblewrapProvider) CreateHandle(cfg map[string]any, policy *ExecutionPolicy) (Handle, error) {
	return nil, ErrProviderUnavailable
}

// RegisterBubblewrapProvider is a no-op stub on non-linux platforms.
func RegisterBubblewrapProvider(m *Manager) error {
	return ErrProviderUnavailable
}

// 编译期断言：BubblewrapProvider 满足 Provider 接口
var _ Provider = (*BubblewrapProvider)(nil)
