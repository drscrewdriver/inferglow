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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/inferglow/action"
)

func TestFileReadSpec(t *testing.T) {
	if FileReadSpec.SideEffectLevel != action.SideEffectRead {
		t.Errorf("SideEffectLevel = %q, want %q", FileReadSpec.SideEffectLevel, action.SideEffectRead)
	}
	if FileReadSpec.ApprovalRequired {
		t.Errorf("ApprovalRequired = true, want false")
	}
	if FileReadSpec.SandboxRequired {
		t.Errorf("SandboxRequired = true, want false")
	}
}

func TestFileReadSuccess(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "data.txt")
	if err := os.WriteFile(target, []byte("file body"), 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}
	a := NewFileReadAction(FileReadConfig{AllowedDirs: []string{dir}})
	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"path": target,
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	out, ok := res.Result.(FileReadResult)
	if !ok {
		t.Fatalf("Result not FileReadResult: %T", res.Result)
	}
	if out.Content != "file body" {
		t.Errorf("Content = %q, want %q", out.Content, "file body")
	}
	if out.BytesRead != int64(len("file body")) {
		t.Errorf("BytesRead = %d, want %d", out.BytesRead, len("file body"))
	}
}

func TestFileReadMissingPath(t *testing.T) {
	a := NewFileReadAction(FileReadConfig{AllowedDirs: []string{t.TempDir()}})
	res, _ := a.Executor.Execute(context.Background(), map[string]any{})
	if res.OK {
		t.Errorf("expected OK=false for missing path")
	}
}

func TestFileReadOutsideAllowedDir(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(target, []byte("secret"), 0o600); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}
	a := NewFileReadAction(FileReadConfig{AllowedDirs: []string{dir}})
	res, _ := a.Executor.Execute(context.Background(), map[string]any{
		"path": target,
	})
	if res.OK {
		t.Errorf("expected OK=false for path outside allowed dir")
	}
	if !strings.Contains(res.Error, "outside allowed") {
		t.Errorf("expected outside-allowed error, got %q", res.Error)
	}
}

func TestFileReadEmptyAllowedDirs(t *testing.T) {
	a := NewFileReadAction(FileReadConfig{}) // no allowed dirs
	res, _ := a.Executor.Execute(context.Background(), map[string]any{
		"path": "/etc/hosts",
	})
	if res.OK {
		t.Errorf("expected OK=false when no dirs are allowed")
	}
}

func TestFileReadNonexistent(t *testing.T) {
	dir := t.TempDir()
	a := NewFileReadAction(FileReadConfig{AllowedDirs: []string{dir}})
	res, _ := a.Executor.Execute(context.Background(), map[string]any{
		"path": filepath.Join(dir, "missing.txt"),
	})
	if res.OK {
		t.Errorf("expected OK=false for nonexistent file")
	}
	if !strings.Contains(res.Error, "open") {
		t.Errorf("expected open error, got %q", res.Error)
	}
}

func TestFileReadTruncation(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "big.txt")
	body := strings.Repeat("a", 1000)
	if err := os.WriteFile(target, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}
	a := NewFileReadAction(FileReadConfig{AllowedDirs: []string{dir}, MaxBytes: 100})
	res, _ := a.Executor.Execute(context.Background(), map[string]any{
		"path": target,
	})
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	out, _ := res.Result.(FileReadResult)
	if out.BytesRead != 100 {
		t.Errorf("BytesRead = %d, want 100", out.BytesRead)
	}
	if res.Metadata == nil || res.Metadata["truncated"] != true {
		t.Errorf("expected truncated metadata, got %+v", res.Metadata)
	}
}

func TestFileReadInputMaxBytesOverride(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "x.txt")
	body := strings.Repeat("b", 500)
	if err := os.WriteFile(target, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}
	a := NewFileReadAction(FileReadConfig{AllowedDirs: []string{dir}, MaxBytes: 1 << 20})
	res, _ := a.Executor.Execute(context.Background(), map[string]any{
		"path":      target,
		"max_bytes": float64(50),
	})
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	out, _ := res.Result.(FileReadResult)
	if out.BytesRead != 50 {
		t.Errorf("BytesRead = %d, want 50", out.BytesRead)
	}
}

func TestFileReadSymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("pwned"), 0o600); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(outsideFile, link); err != nil {
		t.Fatalf("Symlink error: %v", err)
	}
	a := NewFileReadAction(FileReadConfig{AllowedDirs: []string{dir}})
	res, _ := a.Executor.Execute(context.Background(), map[string]any{
		"path": link,
	})
	// The cleaned absolute path of the symlink target should be outside
	// the allowed dir (filepath.Abs follows symlinks at the lexical
	// level only, so this exercises the path check, not link resolution).
	if res.OK {
		t.Errorf("expected OK=false for symlink escape")
	}
}

func TestFileReadActionRegistration(t *testing.T) {
	r := action.NewRegistry()
	if err := r.Register(NewFileReadAction(FileReadConfig{AllowedDirs: []string{t.TempDir()}})); err != nil {
		t.Fatalf("Register error: %v", err)
	}
	if !r.Has(FileReadActionID) {
		t.Errorf("registry missing %q", FileReadActionID)
	}
}

func TestIsPathAllowed(t *testing.T) {
	dir := "/tmp/allowed"
	cases := []struct {
		path    string
		allowed []string
		want    bool
	}{
		{"/tmp/allowed/a.txt", []string{dir}, true},
		{"/tmp/allowed/sub/b.txt", []string{dir}, true},
		{"/tmp/other/c.txt", []string{dir}, false},
		{"/tmp/allowed", []string{dir}, false}, // dir itself, not file inside
		{"/tmp/allowedescape.txt", []string{dir}, false},
	}
	for _, tc := range cases {
		got := isPathAllowed(filepath.Clean(tc.path), tc.allowed)
		if got != tc.want {
			t.Errorf("isPathAllowed(%q, %v) = %v, want %v", tc.path, tc.allowed, got, tc.want)
		}
	}
}

func TestExecute_WithOffsetChunkedRead(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "data.txt")
	body := "0123456789"
	if err := os.WriteFile(target, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}
	a := NewFileReadAction(FileReadConfig{AllowedDirs: []string{dir}})

	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"path":   target,
		"offset": float64(5),
		"limit":  float64(3),
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	out, ok := res.Result.(FileReadResult)
	if !ok {
		t.Fatalf("Result not FileReadResult: %T", res.Result)
	}
	if out.Content != "567" {
		t.Errorf("Content = %q, want %q", out.Content, "567")
	}
	if out.Offset != 5 {
		t.Errorf("Offset = %d, want 5", out.Offset)
	}
	if out.Truncated {
		t.Errorf("Truncated = true, want false (exact segment)")
	}

	// Segment shorter than limit at file end.
	res, _ = a.Executor.Execute(context.Background(), map[string]any{
		"path":   target,
		"offset": float64(8),
		"limit":  float64(10),
	})
	out, _ = res.Result.(FileReadResult)
	if out.Content != "89" {
		t.Errorf("tail Content = %q, want %q", out.Content, "89")
	}
	if !out.Truncated {
		t.Errorf("expected Truncated=true at file end")
	}

	// Offset beyond end of file.
	res, _ = a.Executor.Execute(context.Background(), map[string]any{
		"path":   target,
		"offset": float64(100),
	})
	if res.OK {
		t.Errorf("expected OK=false for offset beyond EOF")
	}
}

func TestExecute_OversizeReturnsDigestNotInline(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "big.log")
	body := strings.Repeat("x", 1000) + "TAILMARK"
	if err := os.WriteFile(target, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}
	a := NewFileReadAction(FileReadConfig{
		AllowedDirs:    []string{dir},
		MaxInlineBytes: 100,
		HardCapBytes:   1 << 20,
		SpillStore:     action.NewLocalSpillStore(t.TempDir()),
	})
	res, err := a.Executor.Execute(context.Background(), map[string]any{"path": target})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	out, ok := res.Result.(FileReadResult)
	if !ok {
		t.Fatalf("Result not FileReadResult: %T", res.Result)
	}
	if out.Content != "" {
		t.Errorf("Content should be empty for oversized file, got %d bytes", len(out.Content))
	}
	if out.Locator == "" || out.RetrievalHint == "" {
		t.Errorf("expected locator + retrieval hint, got %+v", out)
	}
	if out.Digest == nil {
		t.Fatalf("expected positional digest")
	}
	if !strings.Contains(out.Digest.Tail, "TAILMARK") {
		t.Errorf("digest Tail missing tail content: %q", out.Digest.Tail)
	}
	if out.TotalBytes != int64(len(body)) {
		t.Errorf("TotalBytes = %d, want %d", out.TotalBytes, len(body))
	}
	if res.Metadata == nil || res.Metadata["spilled"] != true {
		t.Errorf("expected spilled metadata, got %+v", res.Metadata)
	}
	// Spill artifact must contain the full content.
	full, err := os.ReadFile(out.Locator)
	if err != nil {
		t.Fatalf("read spill artifact: %v", err)
	}
	if string(full) != body {
		t.Errorf("spill artifact content mismatch (%d vs %d bytes)", len(full), len(body))
	}
}

func TestExecute_HardCapReturnsStatsOnly(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "huge.bin")
	body := strings.Repeat("z", 5000)
	if err := os.WriteFile(target, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}
	a := NewFileReadAction(FileReadConfig{
		AllowedDirs:    []string{dir},
		MaxInlineBytes: 100,
		HardCapBytes:   1000,
		SpillStore:     action.NewLocalSpillStore(t.TempDir()),
	})
	res, err := a.Executor.Execute(context.Background(), map[string]any{"path": target})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	out, ok := res.Result.(FileReadResult)
	if !ok {
		t.Fatalf("Result not FileReadResult: %T", res.Result)
	}
	if out.Content != "" || out.Locator != "" {
		t.Errorf("expected no inline content and no locator, got %+v", out)
	}
	if out.Digest == nil || out.Digest.TotalBytes != int64(len(body)) {
		t.Errorf("expected stats digest with TotalBytes=%d, got %+v", len(body), out.Digest)
	}
	if res.Metadata == nil || res.Metadata["refused_inline"] != true {
		t.Errorf("expected refused_inline metadata, got %+v", res.Metadata)
	}
}

func TestExecute_OversizeWithoutSpillFallsBackToDigest(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "nospill.txt")
	body := strings.Repeat("n", 500)
	if err := os.WriteFile(target, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}
	a := NewFileReadAction(FileReadConfig{
		AllowedDirs:    []string{dir},
		MaxInlineBytes: 50,
		HardCapBytes:   1 << 20,
	})
	res, err := a.Executor.Execute(context.Background(), map[string]any{"path": target})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	out, _ := res.Result.(FileReadResult)
	if out.Content != "" || out.Digest == nil {
		t.Errorf("expected digest-only fallback, got %+v", out)
	}
}
