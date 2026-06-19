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

import "context"

// Document represents a text document to be reranked.
type Document struct {
	// Content is the text content of the document.
	Content string `json:"content"`

	// Metadata holds arbitrary metadata associated with the document.
	Metadata map[string]any `json:"metadata,omitempty"`
}

// RankedDocument is a document with an associated relevance score.
type RankedDocument struct {
	Document

	// Score is the relevance score (higher = more relevant).
	Score float64 `json:"score"`

	// Index is the original position in the input slice.
	Index int `json:"index"`
}

// Config holds configuration for creating a Reranker.
type Config struct {
	// Provider is the reranker provider: "cohere", "llm".
	Provider string `json:"provider"`

	// CohereAPIKey is the API key for Cohere reranking.
	CohereAPIKey string `json:"cohere_api_key,omitempty"`

	// CohereModel is the Cohere model name (default: "rerank-english-v3.0").
	CohereModel string `json:"cohere_model,omitempty"`

	// CohereBaseURL overrides the default Cohere API URL.
	CohereBaseURL string `json:"cohere_base_url,omitempty"`

	// TopN limits the number of results returned. 0 means return all.
	TopN int `json:"top_n,omitempty"`

	// MaxTokensPerDocument limits the tokens sent per document.
	MaxTokensPerDocument int `json:"max_tokens_per_document,omitempty"`
}

// Reranker reranks documents based on relevance to a query.
type Reranker interface {
	// Rerank takes a query and a list of documents, and returns them
	// sorted by relevance (highest score first).
	Rerank(ctx context.Context, query string, docs []Document) ([]RankedDocument, error)
}
