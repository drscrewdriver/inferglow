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

package cli

import (
	"bytes"
	"encoding/json"
	"log"
	"os"
	"strings"
	"testing"
)

// silenceToolDenoiseLog redirects the std log (used for the [tool_denoise]
// report line) during a test and restores it on cleanup.
func silenceToolDenoiseLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return &buf
}

func TestDenoiseToolOutputDisabled(t *testing.T) {
	b := &MemoryBridge{}
	in := "\x1b[31merr\x1b[0m\nsame\nsame\n"
	if got := b.denoiseToolOutput("bash", in); got != in {
		t.Errorf("flag off must pass through byte-identical, got %q", got)
	}
}

func TestDenoiseToolOutputEnabled(t *testing.T) {
	buf := silenceToolDenoiseLog(t)
	b := &MemoryBridge{toolDenoise: true}
	in := "Downloading 1%\rDownloading 100%\n"
	if got := b.denoiseToolOutput("bash", in); got != "Downloading 100%\n" {
		t.Errorf("got %q, want %q", got, "Downloading 100%\n")
	}
	if !strings.Contains(buf.String(), "[tool_denoise] bash:") {
		t.Errorf("report log missing: %q", buf.String())
	}
}

func TestDenoiseToolOutputUnchangedSkipsLog(t *testing.T) {
	buf := silenceToolDenoiseLog(t)
	b := &MemoryBridge{toolDenoise: true}
	in := "already clean\n"
	if got := b.denoiseToolOutput("bash", in); got != in {
		t.Errorf("got %q, want unchanged", got)
	}
	if buf.Len() != 0 {
		t.Errorf("unexpected log for unchanged content: %q", buf.String())
	}
}

func TestDenoiseToolOutputClampsHugeInput(t *testing.T) {
	silenceToolDenoiseLog(t)
	b := &MemoryBridge{toolDenoise: true}
	in := strings.Repeat("a\r", 2_000_000) // ~4MB redraw noise
	got := b.denoiseToolOutput("bash", in)
	if len(got) > maxDenoiseInput {
		t.Errorf("result %d bytes exceeds the %d-byte scan bound", len(got), maxDenoiseInput)
	}
}

func TestFeatureFlagToolDenoiseParsing(t *testing.T) {
	// Explicit true parses.
	var cfg CLIConfig
	if err := json.Unmarshal([]byte(`{"features":{"tool_denoise":true}}`), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !cfg.Features.ToolDenoise {
		t.Error("tool_denoise=true did not parse")
	}
	// Default stays off (not present in DefaultCLIConfig).
	if DefaultCLIConfig().Features.ToolDenoise {
		t.Error("default config must leave tool_denoise off")
	}
	// Old configs without the key are unaffected.
	var old CLIConfig
	if err := json.Unmarshal([]byte(`{"data_dir":"/tmp/x"}`), &old); err != nil {
		t.Fatalf("legacy config: %v", err)
	}
	if old.Features.ToolDenoise {
		t.Error("legacy config without the key must stay off")
	}
	// Present-but-invalid: the repository-wide non-fatal convention applies —
	// Unmarshal surfaces a type error and the flag fails safe to off.
	cfg = DefaultCLIConfig()
	if err := json.Unmarshal([]byte(`{"features":{"tool_denoise":"yes"}}`), &cfg); err == nil {
		t.Error("invalid type should surface an error")
	}
	if cfg.Features.ToolDenoise {
		t.Error("invalid value must fail safe to off")
	}
}

func TestEnvOverrideToolDenoise(t *testing.T) {
	t.Setenv("INFERGLOW_TOOL_DENOISE", "1")
	cfg := DefaultCLIConfig()
	ApplyEnvOverrides(&cfg)
	if !cfg.Features.ToolDenoise {
		t.Error("INFERGLOW_TOOL_DENOISE=1 should enable the gate")
	}

	t.Setenv("INFERGLOW_TOOL_DENOISE", "yes")
	cfg = DefaultCLIConfig()
	ApplyEnvOverrides(&cfg)
	if cfg.Features.ToolDenoise {
		t.Error("non-exact env spellings must not enable the gate")
	}
}
