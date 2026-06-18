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

type mockStore struct {
	docs []EmbeddedDocument
}

func (s *mockStore) Add(_ context.Context, docs []EmbeddedDocument) error {
	s.docs = append(s.docs, docs...)
	return nil
}

func TestDocumentPipeline(t *testing.T) {
	store := &mockStore{}
	pipeline := &DocumentPipeline{
		Loader:   NewTextLoader(),
		Splitter: NewRecursiveCharacterTextSplitter(100, 0),
		Embedder: &mockEmbedding{dim: 8},
		Store:    store,
	}

	input := "First paragraph.\n\nSecond paragraph."
	embedded, err := pipeline.Run(context.Background(), strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(embedded) == 0 {
		t.Fatal("expected embedded documents")
	}
	if len(store.docs) != len(embedded) {
		t.Errorf("store has %d docs, expected %d", len(store.docs), len(embedded))
	}
	for _, doc := range embedded {
		if len(doc.Vector) != 8 {
			t.Errorf("expected vector dim=8, got %d", len(doc.Vector))
		}
		if doc.Content == "" {
			t.Error("expected non-empty content")
		}
	}
}

func TestDocumentPipelineNoSplitter(t *testing.T) {
	store := &mockStore{}
	pipeline := &DocumentPipeline{
		Loader:   NewTextLoader(),
		Embedder: &mockEmbedding{dim: 4},
		Store:    store,
	}

	input := "Single paragraph."
	embedded, err := pipeline.Run(context.Background(), strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(embedded) != 1 {
		t.Fatalf("expected 1 embedded doc, got %d", len(embedded))
	}
}

func TestDocumentPipelineEmptyInput(t *testing.T) {
	pipeline := &DocumentPipeline{
		Loader:   NewTextLoader(),
		Embedder: &mockEmbedding{dim: 4},
	}

	embedded, err := pipeline.Run(context.Background(), strings.NewReader("   "))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if embedded != nil {
		t.Fatalf("expected nil for empty input, got %d", len(embedded))
	}
}

func TestDocumentPipelineNoStore(t *testing.T) {
	pipeline := &DocumentPipeline{
		Loader:   NewTextLoader(),
		Embedder: &mockEmbedding{dim: 4},
		// No store
	}

	embedded, err := pipeline.Run(context.Background(), strings.NewReader("Test content."))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(embedded) == 0 {
		t.Fatal("expected embedded docs even without store")
	}
}
