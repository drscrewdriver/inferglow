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

package rag

import (
	"strings"
	"testing"
)

func TestRecursiveCharacterTextSplitter(t *testing.T) {
	splitter := NewRecursiveCharacterTextSplitter(50, 10)
	text := "First paragraph.\n\nSecond paragraph.\n\nThird paragraph."
	chunks, err := splitter.Split(text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("expected at least 1 chunk")
	}
	for _, c := range chunks {
		if len(c) > 60 { // Allow some margin for trimming
			t.Errorf("chunk too long (%d): %q", len(c), c)
		}
	}
}

func TestRecursiveCharacterTextSplitterEmpty(t *testing.T) {
	splitter := NewRecursiveCharacterTextSplitter(50, 10)
	chunks, err := splitter.Split("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chunks != nil {
		t.Fatalf("expected nil for empty input, got %d chunks", len(chunks))
	}
}

func TestRecursiveCharacterTextSplitterSmallText(t *testing.T) {
	splitter := NewRecursiveCharacterTextSplitter(1000, 0)
	text := "Short text."
	chunks, err := splitter.Split(text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0] != "Short text." {
		t.Errorf("expected 'Short text.', got %q", chunks[0])
	}
}

func TestTokenSplitter(t *testing.T) {
	splitter := NewTokenSplitter(10, 2)
	// 10 tokens ≈ 40 chars
	text := strings.Repeat("word ", 30) // ~150 chars, ~37 tokens
	chunks, err := splitter.Split(text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
	}
}

func TestTokenSplitterEmpty(t *testing.T) {
	splitter := NewTokenSplitter(10, 2)
	chunks, err := splitter.Split("   ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chunks != nil {
		t.Fatalf("expected nil for empty input, got %d chunks", len(chunks))
	}
}

func TestMarkdownSplitter(t *testing.T) {
	splitter := NewMarkdownSplitter(1000)
	text := "# Title\n\nIntro.\n\n## Section 1\n\nBody 1.\n\n## Section 2\n\nBody 2."
	chunks, err := splitter.Split(text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
	}
}

func TestMarkdownSplitterEmpty(t *testing.T) {
	splitter := NewMarkdownSplitter(1000)
	chunks, err := splitter.Split("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chunks != nil {
		t.Fatalf("expected nil for empty input, got %d chunks", len(chunks))
	}
}

func TestMarkdownSplitterOversizedSection(t *testing.T) {
	splitter := NewMarkdownSplitter(50)
	text := "# Title\n\n" + strings.Repeat("word ", 50)
	chunks, err := splitter.Split(text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks for oversized section, got %d", len(chunks))
	}
}
