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
	"fmt"
	"io"
)

// DocumentStore is the interface for persisting embedded documents.
// Concrete implementations (in-memory, vector DB, etc.) are provided separately.
type DocumentStore interface {
	// Add stores embedded documents.
	Add(ctx context.Context, docs []EmbeddedDocument) error
}

// DocumentPipeline orchestrates the full RAG ingestion flow:
// load → split → embed → store.
type DocumentPipeline struct {
	// Loader loads raw documents from input.
	Loader DocumentLoader

	// Splitter splits documents into smaller chunks.
	// If nil, documents are not split.
	Splitter TextSplitter

	// Embedder generates embedding vectors for text chunks.
	Embedder EmbeddingModel

	// Store persists embedded documents.
	Store DocumentStore
}

// Run executes the full pipeline: load → split → embed → store.
// It returns the embedded documents that were stored.
func (p *DocumentPipeline) Run(ctx context.Context, r io.Reader) ([]EmbeddedDocument, error) {
	// Step 1: Load documents
	docs, err := p.Loader.Load(ctx, r)
	if err != nil {
		return nil, fmt.Errorf("pipeline load: %w", err)
	}
	if len(docs) == 0 {
		return nil, nil
	}

	// Step 2: Split documents into chunks
	var chunks []string
	var chunkMeta []map[string]any

	for _, doc := range docs {
		if p.Splitter != nil {
			parts, err := p.Splitter.Split(doc.Content)
			if err != nil {
				return nil, fmt.Errorf("pipeline split: %w", err)
			}
			for _, part := range parts {
				chunks = append(chunks, part)
				// Copy original metadata and add chunk info
				meta := make(map[string]any)
				for k, v := range doc.Metadata {
					meta[k] = v
				}
				meta["source_id"] = doc.ID
				meta["source"] = doc.Source
				chunkMeta = append(chunkMeta, meta)
			}
		} else {
			chunks = append(chunks, doc.Content)
			chunkMeta = append(chunkMeta, doc.Metadata)
		}
	}

	if len(chunks) == 0 {
		return nil, nil
	}

	// Step 3: Embed chunks
	vectors, err := p.Embedder.Embed(ctx, chunks)
	if err != nil {
		return nil, fmt.Errorf("pipeline embed: %w", err)
	}

	if len(vectors) != len(chunks) {
		return nil, fmt.Errorf("pipeline embed: got %d vectors for %d chunks", len(vectors), len(chunks))
	}

	// Step 4: Build embedded documents
	embedded := make([]EmbeddedDocument, len(chunks))
	for i := range chunks {
		embedded[i] = EmbeddedDocument{
			Document: Document{
				Content:  chunks[i],
				Metadata: chunkMeta[i],
			},
			Vector: vectors[i],
		}
	}

	// Step 5: Store
	if p.Store != nil {
		if err := p.Store.Add(ctx, embedded); err != nil {
			return nil, fmt.Errorf("pipeline store: %w", err)
		}
	}

	return embedded, nil
}
