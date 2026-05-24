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

import "strings"

// ResolveURL resolves the final request URL for a Provider.
//
// When fullURL is non-empty, it is returned as-is (no concatenation, no
// trailing-slash trimming) — this mirrors Agently's `full_url` override
// semantics and lets callers target proxy gateways, Azure special paths, or
// self-deployed non-standard endpoints.
//
// Otherwise the URL is built by concatenating `strings.TrimRight(baseURL, "/")`
// with defaultPath, preserving the legacy behavior of all three Providers
// (OpenAI / Anthropic / Ollama) when FullURL is empty.
//
// Spec: model-parity Phase 1, P0 — full_url 覆盖.
func ResolveURL(baseURL, defaultPath, fullURL string) string {
	if fullURL != "" {
		return fullURL
	}
	return strings.TrimRight(baseURL, "/") + defaultPath
}
