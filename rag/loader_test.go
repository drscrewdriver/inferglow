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
	"context"
	"strings"
	"testing"
)

func TestTextLoader(t *testing.T) {
	loader := NewTextLoader()
	input := "First paragraph.\n\nSecond paragraph.\n\nThird paragraph."
	docs, err := loader.Load(context.Background(), strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(docs) != 3 {
		t.Fatalf("expected 3 docs, got %d", len(docs))
	}
	if docs[0].Content != "First paragraph." {
		t.Errorf("expected 'First paragraph.', got %q", docs[0].Content)
	}
	if docs[1].Content != "Second paragraph." {
		t.Errorf("expected 'Second paragraph.', got %q", docs[1].Content)
	}
}

func TestTextLoaderEmpty(t *testing.T) {
	loader := NewTextLoader()
	docs, err := loader.Load(context.Background(), strings.NewReader("   "))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if docs != nil {
		t.Fatalf("expected nil docs for empty input, got %d", len(docs))
	}
}

func TestTextLoaderNoParagraphs(t *testing.T) {
	loader := NewTextLoader()
	docs, err := loader.Load(context.Background(), strings.NewReader("single line"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("expected 1 doc, got %d", len(docs))
	}
	if docs[0].Content != "single line" {
		t.Errorf("expected 'single line', got %q", docs[0].Content)
	}
}

func TestLineLoader(t *testing.T) {
	loader := NewLineLoader()
	input := "line1\nline2\n\nline4"
	docs, err := loader.Load(context.Background(), strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(docs) != 3 {
		t.Fatalf("expected 3 docs, got %d", len(docs))
	}
	if docs[0].Metadata["line_num"] != 1 {
		t.Errorf("expected line_num=1, got %v", docs[0].Metadata["line_num"])
	}
	if docs[2].Metadata["line_num"] != 4 {
		t.Errorf("expected line_num=4, got %v", docs[2].Metadata["line_num"])
	}
}

func TestMarkdownLoader(t *testing.T) {
	loader := &MarkdownLoader{}
	input := "# Title\n\nIntro text.\n\n## Section 1\n\nBody of section 1.\n\n## Section 2\n\nBody of section 2."
	docs, err := loader.Load(context.Background(), strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(docs) < 2 {
		t.Fatalf("expected at least 2 docs, got %d", len(docs))
	}
}

func TestHTMLLoader(t *testing.T) {
	loader := NewHTMLLoader()
	input := `<html><body><p>Hello world</p><p>Second paragraph</p></body></html>`
	docs, err := loader.Load(context.Background(), strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(docs) == 0 {
		t.Fatal("expected at least 1 doc")
	}
	found := false
	for _, d := range docs {
		if strings.Contains(d.Content, "Hello world") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find 'Hello world' in documents")
	}
}

func TestHTMLLoaderStripsScripts(t *testing.T) {
	loader := NewHTMLLoader()
	input := `<html><body><script>alert('x')</script><p>Content</p></body></html>`
	docs, err := loader.Load(context.Background(), strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, d := range docs {
		if strings.Contains(d.Content, "alert") {
			t.Error("script content should be stripped")
		}
	}
}

func TestJSONLoader(t *testing.T) {
	loader := &JSONLoader{}
	input := `{"text": "hello", "id": "1"}
{"text": "world", "id": "2"}`
	docs, err := loader.Load(context.Background(), strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("expected 2 docs, got %d", len(docs))
	}
}

func TestJSONLoaderWithContentField(t *testing.T) {
	loader := &JSONLoader{ContentField: "text"}
	input := `{"text": "hello", "id": "1"}`
	docs, err := loader.Load(context.Background(), strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("expected 1 doc, got %d", len(docs))
	}
	if docs[0].Content != "hello" {
		t.Errorf("expected content 'hello', got %q", docs[0].Content)
	}
}

func TestCSVLoader(t *testing.T) {
	loader := NewCSVLoader()
	input := "name,description\nAlice,A developer\nBob,A designer"
	docs, err := loader.Load(context.Background(), strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("expected 2 docs, got %d", len(docs))
	}
	if docs[0].Metadata["name"] != "Alice" {
		t.Errorf("expected name=Alice, got %v", docs[0].Metadata["name"])
	}
}

func TestCSVLoaderCustomComma(t *testing.T) {
	loader := &CSVLoader{HasHeader: true, Comma: ';'}
	input := "name;desc\nAlice;dev"
	docs, err := loader.Load(context.Background(), strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("expected 1 doc, got %d", len(docs))
	}
}
