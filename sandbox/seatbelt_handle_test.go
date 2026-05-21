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

// ---------- stub 层测试（跨平台） ----------

func TestSeatbeltHandleStubCreateReturnsError(t *testing.T) {
	provider := &SeatbeltProvider{}
	_, err := provider.CreateHandle(nil, nil)
	if err == nil {
		t.Error("expected error from SeatbeltProvider.CreateHandle on non-darwin")
	}
}

func TestSeatbeltHandleStubStatusIsCreated(t *testing.T) {
	// 在非 darwin 上无法创建 handle，所以只测试 stub provider
	provider := &SeatbeltProvider{}
	h, err := provider.CreateHandle(nil, nil)
	if err == nil {
		t.Fatal("expected error, got handle")
	}
	if h != nil {
		t.Error("expected nil handle on error")
	}
}
