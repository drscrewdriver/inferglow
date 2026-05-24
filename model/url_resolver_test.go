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

package model

import "testing"

// TestResolveURL covers the four contractual scenarios of ResolveURL:
//  1. full_url 非空时直接返回 full_url（不拼接、不去尾部斜杠）
//  2. full_url 空且 base_url 无尾部斜杠时拼接 defaultPath
//  3. full_url 空且 base_url 有尾部斜杠时去除后拼接 defaultPath
//  4. full_url 与 base_url 均空时返回 defaultPath（兼容现状）
func TestResolveURL(t *testing.T) {
	t.Run("full_url_non_empty_returns_full_url", func(t *testing.T) {
		got := ResolveURL("https://api.openai.com/v1", "/chat/completions", "https://gateway.example.com/v1/proxy/chat")
		want := "https://gateway.example.com/v1/proxy/chat"
		if got != want {
			t.Errorf("ResolveURL(...) = %q, want %q", got, want)
		}
	})

	t.Run("full_url_empty_no_trailing_slash", func(t *testing.T) {
		got := ResolveURL("https://api.openai.com/v1", "/chat/completions", "")
		want := "https://api.openai.com/v1/chat/completions"
		if got != want {
			t.Errorf("ResolveURL(...) = %q, want %q", got, want)
		}
	})

	t.Run("full_url_empty_trailing_slash_stripped", func(t *testing.T) {
		got := ResolveURL("https://open.bigmodel.cn/api/paas/v4/", "/chat/completions", "")
		want := "https://open.bigmodel.cn/api/paas/v4/chat/completions"
		if got != want {
			t.Errorf("ResolveURL(...) = %q, want %q", got, want)
		}
	})

	t.Run("both_empty_returns_default_path", func(t *testing.T) {
		got := ResolveURL("", "/chat/completions", "")
		want := "/chat/completions"
		if got != want {
			t.Errorf("ResolveURL(...) = %q, want %q", got, want)
		}
	})

	t.Run("full_url_does_not_strip_trailing_slash", func(t *testing.T) {
		// full_url 直接返回，不去尾部斜杠（spec 明确要求）
		got := ResolveURL("https://api.openai.com/v1", "/chat/completions", "https://proxy.example.com/chat/")
		want := "https://proxy.example.com/chat/"
		if got != want {
			t.Errorf("ResolveURL(...) = %q, want %q", got, want)
		}
	})
}
