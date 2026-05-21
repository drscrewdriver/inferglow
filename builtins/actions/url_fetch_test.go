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

package actions

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inferglow/action"
)

func TestURLFetchSpec(t *testing.T) {
	if URLFetchSpec.SideEffectLevel != action.SideEffectRead {
		t.Errorf("SideEffectLevel = %q, want %q", URLFetchSpec.SideEffectLevel, action.SideEffectRead)
	}
	if URLFetchSpec.ApprovalRequired {
		t.Errorf("ApprovalRequired = true, want false")
	}
	if URLFetchSpec.SandboxRequired {
		t.Errorf("SandboxRequired = true, want false")
	}
}

func TestURLFetchSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello world"))
	}))
	defer srv.Close()

	a := NewURLFetchAction(URLFetchConfig{})
	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"url": srv.URL,
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	out, ok := res.Result.(URLFetchResult)
	if !ok {
		t.Fatalf("Result not URLFetchResult: %T", res.Result)
	}
	if out.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", out.StatusCode)
	}
	if out.Content != "hello world" {
		t.Errorf("Content = %q, want %q", out.Content, "hello world")
	}
	if out.BytesRead != int64(len("hello world")) {
		t.Errorf("BytesRead = %d, want %d", out.BytesRead, len("hello world"))
	}
}

func TestURLFetchMissingURL(t *testing.T) {
	a := NewURLFetchAction(URLFetchConfig{})
	res, _ := a.Executor.Execute(context.Background(), map[string]any{})
	if res.OK {
		t.Errorf("expected OK=false for missing url")
	}
}

func TestURLFetchDisallowedScheme(t *testing.T) {
	a := NewURLFetchAction(URLFetchConfig{AllowedSchemes: []string{"https"}})
	res, _ := a.Executor.Execute(context.Background(), map[string]any{
		"url": "file:///etc/passwd",
	})
	if res.OK {
		t.Errorf("expected OK=false for file:// scheme")
	}
	if !strings.Contains(res.Error, "not allowed") {
		t.Errorf("expected scheme rejection, got %q", res.Error)
	}
}

func TestURLFetchSizeLimit(t *testing.T) {
	big := strings.Repeat("x", 2048)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(big))
	}))
	defer srv.Close()

	a := NewURLFetchAction(URLFetchConfig{MaxBytes: 100})
	res, _ := a.Executor.Execute(context.Background(), map[string]any{
		"url": srv.URL,
	})
	if res.OK {
		t.Errorf("expected OK=false for oversized response")
	}
	if !strings.Contains(res.Error, "exceeds max_bytes") {
		t.Errorf("expected size limit error, got %q", res.Error)
	}
}

func TestURLFetchInputMaxBytesOverride(t *testing.T) {
	body := strings.Repeat("y", 200)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	}))
	defer srv.Close()

	// Default limit is large; input max_bytes is smaller and should win.
	a := NewURLFetchAction(URLFetchConfig{MaxBytes: 1 << 20})
	res, _ := a.Executor.Execute(context.Background(), map[string]any{
		"url":       srv.URL,
		"max_bytes": float64(50),
	})
	if res.OK {
		t.Errorf("expected OK=false due to input max_bytes override")
	}
}

func TestURLFetchConnectionError(t *testing.T) {
	a := NewURLFetchAction(URLFetchConfig{})
	// Port 1 is reserved and should refuse connections.
	res, _ := a.Executor.Execute(context.Background(), map[string]any{
		"url": "http://127.0.0.1:1/nope",
	})
	if res.OK {
		t.Errorf("expected OK=false for connection failure")
	}
}

func TestURLFetchActionRegistration(t *testing.T) {
	r := action.NewRegistry()
	if err := r.Register(NewURLFetchAction(URLFetchConfig{})); err != nil {
		t.Fatalf("Register error: %v", err)
	}
	if !r.Has(URLFetchActionID) {
		t.Errorf("registry missing %q", URLFetchActionID)
	}
}

func TestSchemeAllowed(t *testing.T) {
	if !schemeAllowed("https", []string{"http", "https"}) {
		t.Errorf("https should be allowed")
	}
	if schemeAllowed("file", []string{"http", "https"}) {
		t.Errorf("file should not be allowed")
	}
	if !schemeAllowed("HTTP", []string{"http", "https"}) {
		t.Errorf("case-insensitive match should pass")
	}
}
