package actions

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/inferglow/action"
)

func TestFileWriteSpec(t *testing.T) {
	if FileWriteSpec.SideEffectLevel != action.SideEffectWrite {
		t.Errorf("SideEffectLevel = %q, want %q", FileWriteSpec.SideEffectLevel, action.SideEffectWrite)
	}
	if !FileWriteSpec.ApprovalRequired {
		t.Errorf("ApprovalRequired = false, want true")
	}
	if FileWriteSpec.SandboxRequired {
		t.Errorf("SandboxRequired = true, want false")
	}
}

func TestFileWriteSuccess(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "out.txt")
	a := NewFileWriteAction(FileWriteConfig{AllowedDirs: []string{dir}})
	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"path":    target,
		"content": "hello",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	out, ok := res.Result.(FileWriteResult)
	if !ok {
		t.Fatalf("Result not FileWriteResult: %T", res.Result)
	}
	if out.BytesWritten != int64(len("hello")) {
		t.Errorf("BytesWritten = %d, want %d", out.BytesWritten, len("hello"))
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("file content = %q, want %q", got, "hello")
	}
}

func TestFileWriteMissingPath(t *testing.T) {
	a := NewFileWriteAction(FileWriteConfig{AllowedDirs: []string{t.TempDir()}})
	res, _ := a.Executor.Execute(context.Background(), map[string]any{
		"content": "x",
	})
	if res.OK {
		t.Errorf("expected OK=false for missing path")
	}
}

func TestFileWriteOutsideAllowedDir(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	a := NewFileWriteAction(FileWriteConfig{AllowedDirs: []string{dir}})
	res, _ := a.Executor.Execute(context.Background(), map[string]any{
		"path":    filepath.Join(outside, "x.txt"),
		"content": "evil",
	})
	if res.OK {
		t.Errorf("expected OK=false for path outside allowed dir")
	}
	if !strings.Contains(res.Error, "outside allowed") {
		t.Errorf("expected outside-allowed error, got %q", res.Error)
	}
}

func TestFileWriteAppendMode(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "log.txt")
	if err := os.WriteFile(target, []byte("first\n"), 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}
	a := NewFileWriteAction(FileWriteConfig{AllowedDirs: []string{dir}})
	res, _ := a.Executor.Execute(context.Background(), map[string]any{
		"path":    target,
		"content": "second\n",
		"append":  true,
	})
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	got, _ := os.ReadFile(target)
	if string(got) != "first\nsecond\n" {
		t.Errorf("file content = %q, want %q", got, "first\nsecond\n")
	}
}

func TestFileWriteOverwriteMode(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "o.txt")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}
	a := NewFileWriteAction(FileWriteConfig{AllowedDirs: []string{dir}})
	res, _ := a.Executor.Execute(context.Background(), map[string]any{
		"path":    target,
		"content": "new",
	})
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	got, _ := os.ReadFile(target)
	if string(got) != "new" {
		t.Errorf("file content = %q, want %q", got, "new")
	}
}

func TestFileWriteCreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "sub", "deep", "x.txt")
	a := NewFileWriteAction(FileWriteConfig{AllowedDirs: []string{dir}})
	res, _ := a.Executor.Execute(context.Background(), map[string]any{
		"path":    target,
		"content": "nested",
	})
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("expected file to exist: %v", err)
	}
}

func TestFileWriteEmptyAllowedDirs(t *testing.T) {
	a := NewFileWriteAction(FileWriteConfig{})
	res, _ := a.Executor.Execute(context.Background(), map[string]any{
		"path":    "/tmp/x.txt",
		"content": "x",
	})
	if res.OK {
		t.Errorf("expected OK=false when no dirs allowed")
	}
}

func TestFileWriteActionRegistration(t *testing.T) {
	r := action.NewRegistry()
	if err := r.Register(NewFileWriteAction(FileWriteConfig{AllowedDirs: []string{t.TempDir()}})); err != nil {
		t.Fatalf("Register error: %v", err)
	}
	if !r.Has(FileWriteActionID) {
		t.Errorf("registry missing %q", FileWriteActionID)
	}
}
