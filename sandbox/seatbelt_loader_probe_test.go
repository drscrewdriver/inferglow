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

//go:build darwin

package sandbox

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestSeatbeltProviderLoaderSelfTestProbe verifies the loader probe path:
// when a seatbelt-loader binary passes --self-test, the provider prefers it
// over the sandbox-exec CLI.
func TestSeatbeltProviderLoaderSelfTestProbe(t *testing.T) {
	fake := filepath.Join(t.TempDir(), "seatbelt-loader")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake loader: %v", err)
	}
	t.Setenv(seatbeltLoaderEnv, fake)

	p := NewSeatbeltProvider()
	if !p.available {
		t.Fatal("expected provider available via loader probe")
	}
	if p.loaderPath != fake {
		t.Errorf("expected loaderPath %q, got %q", fake, p.loaderPath)
	}
	if p.sandboxExecPath != "" {
		t.Errorf("expected sandbox-exec fallback to be unused, got %q", p.sandboxExecPath)
	}
}

// TestSeatbeltProviderLoaderSelfTestFailure verifies that a loader failing
// --self-test is rejected; the provider must then fall back to sandbox-exec
// or stay unavailable (fail-closed), never treating the broken loader as
// usable.
func TestSeatbeltProviderLoaderSelfTestFailure(t *testing.T) {
	fake := filepath.Join(t.TempDir(), "seatbelt-loader")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nexit 125\n"), 0o755); err != nil {
		t.Fatalf("write fake loader: %v", err)
	}
	t.Setenv(seatbeltLoaderEnv, fake)

	p := NewSeatbeltProvider()
	if p.loaderPath != "" {
		t.Errorf("expected broken loader to be rejected, got loaderPath %q", p.loaderPath)
	}
	if p.available {
		// Available via the sandbox-exec fallback: acceptable on systems
		// that still ship the deprecated CLI. Otherwise the provider must
		// be unavailable (fail-closed).
		if p.sandboxExecPath == "" {
			t.Error("provider available but neither loader nor sandbox-exec path set")
		}
		return
	}
	if _, err := exec.LookPath("sandbox-exec"); err == nil {
		t.Error("expected fallback to sandbox-exec when the CLI exists")
	}
}

// TestSeatbeltProviderLoaderMissingFallback verifies that a missing loader
// falls back to the sandbox-exec CLI when present, and fails closed when the
// CLI is also absent.
func TestSeatbeltProviderLoaderMissingFallback(t *testing.T) {
	t.Setenv(seatbeltLoaderEnv, filepath.Join(t.TempDir(), "no-such-loader"))

	p := NewSeatbeltProvider()
	if !p.available {
		if _, err := exec.LookPath("sandbox-exec"); err == nil {
			t.Error("expected sandbox-exec fallback to keep the provider available")
		}
		return
	}
	if p.loaderPath != "" {
		t.Errorf("expected loader to be unresolvable, got %q", p.loaderPath)
	}
	if p.sandboxExecPath == "" {
		t.Error("expected provider availability to come from sandbox-exec fallback")
	}
}
