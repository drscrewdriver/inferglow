// Copyright 2026 InferGlow Authors

package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/inferglow/builtins/actions"
	"github.com/inferglow/model"
	"github.com/inferglow/server/config"
)

// TestNativeGrepRootFilesFirst — breadth-first with files-before-subdirs: a
// needle present both at the workspace root and deep inside an early subtree
// must surface the ROOT match first, and must be found at all regardless of
// how much tree noise precedes it. (The previous depth-first walk with a
// 5000-file budget starved top-level files: pr-checker.md sat unvisited.)
func TestNativeGrepRootFilesFirst(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "aaa", "deep")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 300; i++ {
		name := filepath.Join(deep, fmt.Sprintf("noise%03d.txt", i))
		content := "filler\n"
		if i == 150 {
			content = "needle-marker-42 in deep tree\n"
		}
		if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "pr-checker.md"), []byte("needle-marker-42 at root\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := &nativeGrepRunner{roots: []string{root}}
	matches, err := r.Run(context.Background(), actions.GrepRequest{Pattern: "needle-marker-42", Path: "."})
	if err != nil {
		t.Fatalf("grep: %v", err)
	}
	if len(matches) < 2 {
		t.Fatalf("want both matches, got %d: %+v", len(matches), matches)
	}
	if !strings.Contains(matches[0].File, "pr-checker.md") {
		t.Fatalf("root match must come first under BFS, got %+v", matches[0])
	}
}

// TestNativeGrepPathScope — a subpath request only searches that subtree, and
// paths escaping the roots are rejected.
func TestNativeGrepPathScope(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(root, "outside.txt"), []byte("scope-needle\n"), 0o644)
	os.WriteFile(filepath.Join(sub, "inside.txt"), []byte("scope-needle\n"), 0o644)

	r := &nativeGrepRunner{roots: []string{root}}
	m, err := r.Run(context.Background(), actions.GrepRequest{Pattern: "scope-needle", Path: "sub"})
	if err != nil || len(m) != 1 || !strings.Contains(m[0].File, "inside.txt") {
		t.Fatalf("scoped grep: %v %+v", err, m)
	}
	if _, err := r.Run(context.Background(), actions.GrepRequest{Pattern: "x", Path: filepath.Join(root, "..", "elsewhere")}); err == nil {
		t.Fatalf("path outside roots must be rejected")
	}
}

// TestEnableThinkingInjection — optionsInjectRequester merges
// chat_template_kwargs into every request, and caller-set Options keys win.
func TestEnableThinkingInjection(t *testing.T) {
	lc := config.LLMConfig{Provider: "openai", BaseURL: "http://127.0.0.1:1", Model: "m", APIKey: "k"}
	inner, err := modelRequesterFromServerConfig("openai", lc)
	if err != nil {
		t.Fatal(err)
	}
	wrapped := optionsInjectRequester{inner: inner, opts: map[string]any{
		"chat_template_kwargs": map[string]any{"enable_thinking": true},
	}}

	// Default path: the injection lands.
	d, err := wrapped.GenerateRequestData(context.Background(), &model.ModelRequest{Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	kwargs, ok := d.Options["chat_template_kwargs"].(map[string]any)
	if !ok || kwargs["enable_thinking"] != true {
		t.Fatalf("chat_template_kwargs missing: %+v", d.Options)
	}

	// Caller keys win: a pre-set chat_template_kwargs is not overwritten.
	pre := &model.ModelRequest{Model: "m", Options: map[string]any{
		"chat_template_kwargs": "caller-set",
	}}
	d2, err := wrapped.GenerateRequestData(context.Background(), pre)
	if err != nil {
		t.Fatal(err)
	}
	if d2.Options["chat_template_kwargs"] != "caller-set" {
		t.Fatalf("caller key overwritten: %+v", d2.Options)
	}
}
