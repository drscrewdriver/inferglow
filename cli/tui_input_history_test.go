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
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
// MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO
// EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES
// OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
// ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
// DEALINGS IN THE SOFTWARE.

package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestInputHistoryAppendLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), inputHistoryFile)
	appendInputHistoryTo(path, "hello")
	appendInputHistoryTo(path, "world")
	got := loadInputHistoryFrom(path)
	if len(got) != 2 || got[0] != "hello" || got[1] != "world" {
		t.Fatalf("loaded = %v, want [hello world]", got)
	}
}

func TestInputHistoryAdjacentDedupe(t *testing.T) {
	path := filepath.Join(t.TempDir(), inputHistoryFile)
	appendInputHistoryTo(path, "same")
	appendInputHistoryTo(path, "same") // adjacent duplicate → dropped
	appendInputHistoryTo(path, "other")
	appendInputHistoryTo(path, "same") // non-adjacent → kept
	got := loadInputHistoryFrom(path)
	if len(got) != 3 || got[0] != "same" || got[1] != "other" || got[2] != "same" {
		t.Fatalf("loaded = %v, want [same other same]", got)
	}
}

func TestInputHistoryCap(t *testing.T) {
	path := filepath.Join(t.TempDir(), inputHistoryFile)
	for i := 0; i < inputHistoryMax+50; i++ {
		appendInputHistoryTo(path, fmt.Sprintf("line-%d", i))
	}
	got := loadInputHistoryFrom(path)
	if len(got) != inputHistoryMax {
		t.Fatalf("length = %d, want %d", len(got), inputHistoryMax)
	}
	// Oldest entries are trimmed.
	if got[0] != "line-50" {
		t.Fatalf("oldest kept = %q, want line-50", got[0])
	}
	if got[len(got)-1] != fmt.Sprintf("line-%d", inputHistoryMax+49) {
		t.Fatalf("newest = %q, want line-%d", got[len(got)-1], inputHistoryMax+49)
	}
}

func TestInputHistoryCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), inputHistoryFile)
	if err := os.WriteFile(path, []byte("garbage\n\"ok\"\n{corrupt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := loadInputHistoryFrom(path)
	if len(got) != 1 || got[0] != "ok" {
		t.Fatalf("loaded = %v, want [ok] (corrupt lines skipped)", got)
	}
	// Missing file → empty, no panic.
	if got := loadInputHistoryFrom(filepath.Join(t.TempDir(), "nope.json")); got != nil {
		t.Fatalf("missing file should yield nil, got %v", got)
	}
}
