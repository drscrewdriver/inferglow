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

package rerank

import (
	"context"
	"log"
)

// FallbackReranker wraps a Reranker and returns the original document order
// if the underlying reranker fails.
type FallbackReranker struct {
	// Inner is the primary reranker.
	Inner Reranker
}

// WithFallback wraps a reranker with fallback behavior.
// If the inner reranker fails, the original document order is returned
// with zero scores.
func WithFallback(reranker Reranker) *FallbackReranker {
	return &FallbackReranker{Inner: reranker}
}

// Rerank attempts to rerank using the inner reranker.
// On failure, returns documents in their original order with zero scores.
func (f *FallbackReranker) Rerank(ctx context.Context, query string, docs []Document) ([]RankedDocument, error) {
	ranked, err := f.Inner.Rerank(ctx, query, docs)
	if err != nil {
		log.Printf("rerank fallback: inner reranker failed (%v), returning original order", err)
		// Return original order with zero scores
		result := make([]RankedDocument, len(docs))
		for i, doc := range docs {
			result[i] = RankedDocument{
				Document: doc,
				Score:    0,
				Index:    i,
			}
		}
		return result, nil
	}
	return ranked, nil
}
