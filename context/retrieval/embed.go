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

package retrieval

import "context"

// Embedder converts text into a vector embedding for semantic search.
// Implementations may use local models (e.g., sentence-transformers)
// or remote APIs (e.g., OpenAI embeddings).
type Embedder interface {
	// Embed returns the vector representation of the input text.
	Embed(ctx context.Context, text string) ([]float32, error)
	// Dim returns the dimensionality of the embedding vectors.
	Dim() int
}

// NoopEmbedder is a placeholder embedder that returns nil vectors.
// Used when semantic search is disabled.
type NoopEmbedder struct{}

func (n *NoopEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return nil, nil
}

func (n *NoopEmbedder) Dim() int {
	return 0
}
