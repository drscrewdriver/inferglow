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
	"os"
	"path/filepath"
	"testing"
)

func TestApplyThemeLight(t *testing.T) {
	orig := activeTheme
	t.Cleanup(func() { activeTheme = orig })

	if err := applyTheme("light"); err != nil {
		t.Fatalf("applyTheme(light): %v", err)
	}
	if activeTheme != &lightTheme {
		t.Fatal("activeTheme should be lightTheme after applyTheme(light)")
	}
	if activeThemeName() != "light" {
		t.Fatalf("activeThemeName() = %q, want light", activeThemeName())
	}
}

func TestApplyThemeDarkAndAuto(t *testing.T) {
	orig := activeTheme
	t.Cleanup(func() { activeTheme = orig })

	if err := applyTheme("dark"); err != nil {
		t.Fatalf("applyTheme(dark): %v", err)
	}
	if activeTheme != &darkTheme {
		t.Fatal("activeTheme should be darkTheme after applyTheme(dark)")
	}
	// auto resolves to a concrete theme without error.
	if err := applyTheme("auto"); err != nil {
		t.Fatalf("applyTheme(auto): %v", err)
	}
	if activeTheme != &lightTheme && activeTheme != &darkTheme {
		t.Fatal("auto should resolve to a concrete theme")
	}
}

func TestApplyThemeUnknown(t *testing.T) {
	if err := applyTheme("neon"); err == nil {
		t.Fatal("applyTheme(unknown) should error")
	}
}

func TestThemePrefRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), themePrefFile)
	writeThemePrefTo(path, "light")
	if got := readThemePrefFrom(path); got != "light" {
		t.Fatalf("readThemePrefFrom = %q, want light", got)
	}
	// Corrupt → "".
	if err := os.WriteFile(path, []byte("{bad"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readThemePrefFrom(path); got != "" {
		t.Fatalf("corrupt theme should be \"\", got %q", got)
	}
	// Missing → "".
	if got := readThemePrefFrom(filepath.Join(t.TempDir(), "nope.json")); got != "" {
		t.Fatalf("missing theme should be \"\", got %q", got)
	}
}
