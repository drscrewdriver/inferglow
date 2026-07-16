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

package retrieval

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Tokenizer tests
// ---------------------------------------------------------------------------

func TestTokenize_Latin(t *testing.T) {
	tokens := Tokenize("Hello World from Go")
	want := []string{"hello", "world", "from", "go"}
	if len(tokens) != len(want) {
		t.Fatalf("got %d tokens %v, want %d tokens %v", len(tokens), tokens, len(want), want)
	}
	for i, w := range want {
		if tokens[i] != w {
			t.Errorf("token[%d] = %q, want %q", i, tokens[i], w)
		}
	}
}

func TestTokenize_CJK(t *testing.T) {
	// "机器学习" → bigrams: ["机器", "器学", "学习"]
	tokens := Tokenize("机器学习")
	want := []string{"机器", "器学", "学习"}
	if len(tokens) != len(want) {
		t.Fatalf("got %d tokens %v, want %d tokens %v", len(tokens), tokens, len(want), want)
	}
	for i, w := range want {
		if tokens[i] != w {
			t.Errorf("token[%d] = %q, want %q", i, tokens[i], w)
		}
	}
}

func TestTokenize_Mixed(t *testing.T) {
	// "Go语言机器学习" → ["go", "语言", "言语", "言机", "机器", "器学", "学习"]
	tokens := Tokenize("Go语言机器学习")
	if len(tokens) < 2 {
		t.Fatalf("expected multiple tokens, got %v", tokens)
	}
	// First token should be "go" (Latin word).
	if tokens[0] != "go" {
		t.Errorf("first token = %q, want %q", tokens[0], "go")
	}
	// Should contain CJK bigrams.
	found := false
	for _, tok := range tokens {
		if tok == "机器" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected bigram '机器' in tokens %v", tokens)
	}
}

func TestTokenize_Japanese(t *testing.T) {
	// Hiragana + Katakana
	tokens := Tokenize("こんにちは")
	// "こんにちは" → ["こん", "んに", "にち", "ちは"]
	if len(tokens) != 4 {
		t.Fatalf("got %d tokens %v, want 4", len(tokens), tokens)
	}
}

func TestTokenize_Empty(t *testing.T) {
	tokens := Tokenize("")
	if len(tokens) != 0 {
		t.Fatalf("expected 0 tokens, got %v", tokens)
	}
}

func TestTokenize_Punctuation(t *testing.T) {
	tokens := Tokenize("hello, world! test.")
	want := []string{"hello", "world", "test"}
	if len(tokens) != len(want) {
		t.Fatalf("got %d tokens %v, want %d tokens %v", len(tokens), tokens, len(want), want)
	}
}

// ---------------------------------------------------------------------------
// BM25 index tests
// ---------------------------------------------------------------------------

func TestBM25Index_AddSearch(t *testing.T) {
	idx := NewBM25Index()
	idx.Add(1, "the quick brown fox jumps over the lazy dog")
	idx.Add(2, "a fast red fox leaps across a sleepy canine")
	idx.Add(3, "completely unrelated document about databases")

	results, err := idx.Search(context.Background(), "fox", 10)
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results for 'fox', got %d", len(results))
	}
	// Both docs containing "fox" should be returned.
	for _, r := range results {
		if r.StepID != 1 && r.StepID != 2 {
			t.Errorf("unexpected doc %d in results", r.StepID)
		}
	}
}

func TestBM25Index_CJKSearch(t *testing.T) {
	idx := NewBM25Index()
	idx.Add(1, "机器学习是人工智能的一个分支")
	idx.Add(2, "深度学习使用神经网络进行机器学习")
	idx.Add(3, "今天天气真好适合出去散步")

	results, err := idx.Search(context.Background(), "机器学习", 10)
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	// Docs 1 and 2 both contain "机器学习" or its bigrams.
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results for CJK search, got %d: %v", len(results), results)
	}
	// Doc 3 should NOT appear.
	for _, r := range results {
		if r.StepID == 3 {
			t.Errorf("doc 3 should not match CJK search for '机器学习'")
		}
	}
}

func TestBM25Index_NoResults(t *testing.T) {
	idx := NewBM25Index()
	idx.Add(1, "hello world")

	results, err := idx.Search(context.Background(), "nonexistent", 10)
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestBM25Index_EmptyIndex(t *testing.T) {
	idx := NewBM25Index()
	results, err := idx.Search(context.Background(), "anything", 10)
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if results != nil {
		t.Fatalf("expected nil results from empty index, got %v", results)
	}
}

// ---------------------------------------------------------------------------
// Persistence tests
// ---------------------------------------------------------------------------

func TestBM25Index_SaveLoad(t *testing.T) {
	// Create and populate an index.
	orig := NewBM25Index()
	orig.Add(1, "hello world test")
	orig.Add(2, "another document here")
	orig.Add(3, "机器学习人工智能")

	// Save to buffer.
	var buf bytes.Buffer
	if err := orig.Save(&buf); err != nil {
		t.Fatalf("save error: %v", err)
	}

	// Load into a new index.
	restored := NewBM25Index()
	if err := restored.Load(&buf); err != nil {
		t.Fatalf("load error: %v", err)
	}

	// Verify search works on restored index.
	results, err := restored.Search(context.Background(), "hello", 10)
	if err != nil {
		t.Fatalf("search on restored index: %v", err)
	}
	if len(results) != 1 || results[0].StepID != 1 {
		t.Errorf("expected doc 1 for 'hello', got %v", results)
	}

	// Verify CJK search works on restored index.
	results, err = restored.Search(context.Background(), "机器学习", 10)
	if err != nil {
		t.Fatalf("CJK search on restored index: %v", err)
	}
	if len(results) != 1 || results[0].StepID != 3 {
		t.Errorf("expected doc 3 for CJK search, got %v", results)
	}
}

func TestBM25Index_LoadCorrupt(t *testing.T) {
	idx := NewBM25Index()
	err := idx.Load(strings.NewReader("not valid json"))
	if err == nil {
		t.Fatal("expected error loading corrupt data")
	}
}
