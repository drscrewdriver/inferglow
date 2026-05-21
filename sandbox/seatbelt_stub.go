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

//go:build !darwin

package sandbox

// SeatbeltProvider is a stub on non-darwin platforms.
type SeatbeltProvider struct{}

// NewSeatbeltProvider returns a stub SeatbeltProvider on non-darwin platforms.
func NewSeatbeltProvider() *SeatbeltProvider {
	return &SeatbeltProvider{}
}

// Name returns the provider identifier.
func (p *SeatbeltProvider) Name() string { return "seatbelt" }

// Kind returns the provider kind.
func (p *SeatbeltProvider) Kind() string { return "local" }

// InspectAvailability reports whether the seatbelt backend is usable.
func (p *SeatbeltProvider) InspectAvailability() (*AvailabilityResult, error) {
	return &AvailabilityResult{Available: false, Platform: "darwin", ErrorMessage: "seatbelt not available on " + string(DetectOS())}, nil
}

// CreateHandle returns an error on non-darwin platforms.
func (p *SeatbeltProvider) CreateHandle(cfg map[string]any, policy *ExecutionPolicy) (Handle, error) {
	return nil, ErrProviderUnavailable
}

// 编译期断言：SeatbeltProvider 满足 Provider 接口
var _ Provider = (*SeatbeltProvider)(nil)
