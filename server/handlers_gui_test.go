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

package server

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// TestGUI_ServesShell verifies GET /gui/ returns the React app HTML.
func TestGUI_ServesShell(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore())

	req := httptest.NewRequest("GET", "/gui/", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("Content-Type = %q, want text/html", ct)
	}
	if !strings.Contains(w.Body.String(), "<title>InferGlow") {
		t.Fatal("body missing <title>InferGlow")
	}
}

// TestGUI_Redirect verifies GET /gui redirects to /gui/.
func TestGUI_Redirect(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore())

	req := httptest.NewRequest("GET", "/gui", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusMovedPermanently {
		t.Fatalf("want 301, got %d", w.Code)
	}
}

// TestGUI_ServesAssets verifies the hashed asset referenced by the shell HTML
// is served successfully (embed integrity).
func TestGUI_ServesAssets(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore())

	req := httptest.NewRequest("GET", "/gui/", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("shell: want 200, got %d", w.Code)
	}
	asset := regexp.MustCompile(`src="(/assets/[^"]+)"`).FindStringSubmatch(w.Body.String())
	if len(asset) < 2 {
		t.Fatal("shell html missing script asset")
	}

	req = httptest.NewRequest("GET", "/gui"+asset[1], nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("asset %s: want 200, got %d", asset[1], w.Code)
	}
}

// TestGUI_DoesNotShadowAPI verifies the /v1 API still routes through the
// middleware chain (GUI registered on the root mux must not shadow it).
func TestGUI_DoesNotShadowAPI(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore())
	srv.SetSessionStore(NewSessionStore())

	req := httptest.NewRequest("GET", "/v1/sessions", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("api: want 200, got %d", w.Code)
	}
}
