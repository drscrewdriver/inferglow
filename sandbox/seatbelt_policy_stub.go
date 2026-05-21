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

// SeatbeltConfig is a stub on non-darwin platforms.
type SeatbeltConfig struct{}

// buildSBPLProfile returns an empty string on non-darwin platforms.
func buildSBPLProfile(cfg SeatbeltConfig, policy *ExecutionPolicy) string {
	return ""
}

// realPath returns an empty string on non-darwin platforms.
func realPath(path string) string {
	return ""
}

// parseSeatbeltConfig returns an empty config on non-darwin platforms.
func parseSeatbeltConfig(m map[string]any) SeatbeltConfig {
	return SeatbeltConfig{}
}

// writeSBPLProfile returns an error on non-darwin platforms.
func writeSBPLProfile(profile string) (string, func(), error) {
	return "", nil, ErrProviderUnavailable
}
