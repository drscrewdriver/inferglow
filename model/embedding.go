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

package model

import "context"

// EmbeddingRequester is the unified batch embedding interface at the model
// layer. It supersedes the single-text retrieval.Embedder and the
// rag.EmbeddingModel with a consistent batch-first API.
//
// Implementations wrap provider-specific embedding APIs:
//   - OpenAI: text-embedding-3-small / text-embedding-3-large
//   - Cohere: embed-english-v3 / embed-multilingual-v3
//   - Qwen/DashScope: text-embedding-v2
//
// The batch signature enables efficient multi-text embedding in a single
// API call, reducing HTTP overhead for RAG ingest and memory vectorization.
type EmbeddingRequester interface {
	// Embed returns embedding vectors for the given texts.
	// The returned slice must have the same length as the input texts.
	// An empty texts slice returns an empty slice (no error).
	Embed(ctx context.Context, texts []string) ([][]float32, error)

	// Dim returns the dimensionality of the embedding vectors.
	Dim() int

	// ModelName returns the embedding model identifier (e.g. "text-embedding-3-small").
	ModelName() string
}

// NoopEmbeddingRequester is a placeholder that returns nil vectors.
// Used when embedding is disabled or no provider is configured.
type NoopEmbeddingRequester struct{}

func (n *NoopEmbeddingRequester) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	return out, nil
}

func (n *NoopEmbeddingRequester) Dim() int       { return 0 }
func (n *NoopEmbeddingRequester) ModelName() string { return "noop" }

// SingleEmbedAdapter wraps a single-text embedder function into an
// EmbeddingRequester. Useful for adapting legacy single-text APIs.
type SingleEmbedAdapter struct {
	fn        func(ctx context.Context, text string) ([]float32, error)
	dim       int
	modelName string
}

// NewSingleEmbedAdapter creates an EmbeddingRequester from a single-text function.
func NewSingleEmbedAdapter(fn func(ctx context.Context, text string) ([]float32, error), dim int, modelName string) *SingleEmbedAdapter {
	return &SingleEmbedAdapter{fn: fn, dim: dim, modelName: modelName}
}

// Embed implements batch embedding by calling the single-text function
// for each input. Providers with native batch support should implement
// EmbeddingRequester directly for better performance.
func (a *SingleEmbedAdapter) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i, text := range texts {
		vec, err := a.fn(ctx, text)
		if err != nil {
			return results, err
		}
		results[i] = vec
	}
	return results, nil
}

func (a *SingleEmbedAdapter) Dim() int         { return a.dim }
func (a *SingleEmbedAdapter) ModelName() string { return a.modelName }
