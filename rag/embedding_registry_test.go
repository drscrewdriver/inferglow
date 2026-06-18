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
	"testing"
)

type mockEmbedding struct {
	dim int
}

func (m *mockEmbedding) Embed(_ context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i := range texts {
		vec := make([]float32, m.dim)
		for j := range vec {
			vec[j] = float32(i) / float32(len(texts))
		}
		result[i] = vec
	}
	return result, nil
}

func (m *mockEmbedding) Dim() int { return m.dim }

func TestEmbeddingRegistry(t *testing.T) {
	// Register a test model
	RegisterEmbeddingModel("test-mock", func() EmbeddingModel {
		return &mockEmbedding{dim: 128}
	})

	model, err := GetEmbeddingModel("test-mock")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model.Dim() != 128 {
		t.Errorf("expected dim=128, got %d", model.Dim())
	}

	// Test list
	names := ListEmbeddingModels()
	found := false
	for _, n := range names {
		if n == "test-mock" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'test-mock' in registered models")
	}
}

func TestEmbeddingRegistryUnknown(t *testing.T) {
	_, err := GetEmbeddingModel("nonexistent-model")
	if err == nil {
		t.Fatal("expected error for unknown model")
	}
}

func TestEmbeddingModelEmbed(t *testing.T) {
	model := &mockEmbedding{dim: 4}
	vecs, err := model.Embed(context.Background(), []string{"hello", "world"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vecs) != 2 {
		t.Fatalf("expected 2 vectors, got %d", len(vecs))
	}
	if len(vecs[0]) != 4 {
		t.Errorf("expected dim=4, got %d", len(vecs[0]))
	}
}
