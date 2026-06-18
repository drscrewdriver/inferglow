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

// Package rag provides document loading, text splitting, and embedding pipeline
// for Retrieval-Augmented Generation (RAG) workflows.
package rag

// Document represents a unit of text content with associated metadata.
// It is the fundamental data structure flowing through the RAG pipeline.
type Document struct {
	// Content is the text content of the document.
	Content string `json:"content"`

	// Metadata holds arbitrary key-value pairs associated with the document.
	// Common keys include "source", "page", "title", "chunk_index".
	Metadata map[string]any `json:"metadata,omitempty"`

	// ID is an optional unique identifier for the document.
	// If empty, one will be generated during embedding/storage.
	ID string `json:"id,omitempty"`

	// Source identifies where the document came from (file path, URL, etc.).
	Source string `json:"source,omitempty"`
}

// EmbeddedDocument is a Document paired with its embedding vector.
type EmbeddedDocument struct {
	Document
	Vector []float32 `json:"vector"`
}

// SearchResult represents a document retrieved from a vector store with its similarity score.
type SearchResult struct {
	Document Document `json:"document"`
	Score    float64  `json:"score"`
}
