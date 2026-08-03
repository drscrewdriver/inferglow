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

package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// --- ExecutionAccessGrant tests ---

func TestExecutionAccessGrantCanReadWrite(t *testing.T) {
	g := &ExecutionAccessGrant{
		ExecutionID: "exec-1",
		ReadPaths:   []string{"data/", "config.yaml"},
		WritePaths:  []string{"output/"},
	}
	if !g.CanRead("data/") {
		t.Error("expected CanRead(data/) = true")
	}
	if !g.CanRead("config.yaml") {
		t.Error("expected CanRead(config.yaml) = true")
	}
	if g.CanRead("secret/") {
		t.Error("expected CanRead(secret/) = false")
	}
	if !g.CanWrite("output/") {
		t.Error("expected CanWrite(output/) = true")
	}
	if g.CanWrite("data/") {
		t.Error("expected CanWrite(data/) = false")
	}
}

func TestExecutionAccessGrantWildcard(t *testing.T) {
	g := &ExecutionAccessGrant{
		ExecutionID: "exec-2",
		ReadPaths:   []string{"*"},
		WritePaths:  []string{"*"},
	}
	if !g.CanRead("anything") {
		t.Error("wildcard read should match any path")
	}
	if !g.CanWrite("anything") {
		t.Error("wildcard write should match any path")
	}
}

func TestExecutionAccessGrantExpired(t *testing.T) {
	g := &ExecutionAccessGrant{
		ExecutionID: "exec-3",
		ReadPaths:   []string{"*"},
		ExpiresAt:   time.Now().Add(-time.Hour),
	}
	if !g.IsExpired() {
		t.Error("expected expired")
	}
	if g.CanRead("anything") {
		t.Error("expired grant should deny reads")
	}
}

func TestExecutionAccessStore(t *testing.T) {
	store := NewExecutionAccessStore()
	g := &ExecutionAccessGrant{
		ExecutionID: "exec-1",
		ReadPaths:   []string{"data/"},
		WritePaths:  []string{"out/"},
	}
	if err := store.Grant(g); err != nil {
		t.Fatalf("Grant: %v", err)
	}
	if err := store.CheckRead("exec-1", "data/"); err != nil {
		t.Errorf("CheckRead: %v", err)
	}
	if err := store.CheckWrite("exec-1", "out/"); err != nil {
		t.Errorf("CheckWrite: %v", err)
	}
	if err := store.CheckRead("exec-1", "secret/"); !errors.Is(err, ErrAccessDenied) {
		t.Errorf("expected ErrAccessDenied, got %v", err)
	}
	if err := store.CheckRead("unknown", "data/"); !errors.Is(err, ErrGrantNotFound) {
		t.Errorf("expected ErrGrantNotFound, got %v", err)
	}
	if !store.Revoke("exec-1") {
		t.Error("expected Revoke=true")
	}
	if store.Revoke("exec-1") {
		t.Error("expected Revoke=false after revoke")
	}
}

// --- IdentityCatalog tests ---

func TestIdentityCatalog(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	cat := NewIdentityCatalog()
	id, err := cat.ObservePath(path)
	if err != nil {
		t.Fatalf("ObservePath: %v", err)
	}
	if id.Digest == "" {
		t.Error("expected non-empty digest")
	}
	if id.ContentVersion != "v1" {
		t.Errorf("expected v1, got %s", id.ContentVersion)
	}
	if cat.GetVersion(path) != "v1" {
		t.Error("GetVersion mismatch")
	}

	// Same content → same version.
	id2, err := cat.ObservePath(path)
	if err != nil {
		t.Fatalf("ObservePath2: %v", err)
	}
	if id2.ContentVersion != "v1" {
		t.Errorf("expected same version for same content, got %s", id2.ContentVersion)
	}

	// Different content → new version.
	if err := os.WriteFile(path, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	id3, err := cat.ObservePath(path)
	if err != nil {
		t.Fatalf("ObservePath3: %v", err)
	}
	if id3.ContentVersion != "v2" {
		t.Errorf("expected v2 after change, got %s", id3.ContentVersion)
	}

	all := cat.ListIdentities()
	if len(all) != 1 {
		t.Errorf("expected 1 identity, got %d", len(all))
	}
}

// --- ContextSource tests ---

func TestContextSource(t *testing.T) {
	dir := t.TempDir()
	ws, err := New(Config{RootDir: dir})
	if err != nil {
		t.Fatalf("New workspace: %v", err)
	}
	if err := ws.WriteFile("a.txt", []byte("alpha")); err != nil {
		t.Fatal(err)
	}
	if err := ws.WriteFile("b.txt", []byte("beta")); err != nil {
		t.Fatal(err)
	}

	src := NewWorkspaceContextSource(ws)
	descs, cursor, err := src.EnumerateDescriptors(nil, "", 10)
	if err != nil {
		t.Fatalf("EnumerateDescriptors: %v", err)
	}
	if len(descs) != 2 {
		t.Errorf("expected 2 descriptors, got %d", len(descs))
	}
	if cursor != "" {
		t.Errorf("expected empty cursor, got %q", cursor)
	}

	content, truncated, err := src.ReadFile("a.txt", 100)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if content != "alpha" {
		t.Errorf("expected 'alpha', got %q", content)
	}
	if truncated {
		t.Error("expected not truncated")
	}
}
