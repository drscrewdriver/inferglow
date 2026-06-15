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

// Package retrieval implements the three-way fusion retriever (§7.3).
package retrieval

import "context"

// SemanticSearcher performs vector similarity search.
type SemanticSearcher interface {
	Search(ctx context.Context, query []float32, limit int) ([]SearchResult, error)
}

// KeywordSearcher performs keyword/BM25 search.
type KeywordSearcher interface {
	Search(ctx context.Context, query string, limit int) ([]SearchResult, error)
}

// RecencySearcher performs recency-weighted search.
type RecencySearcher interface {
	Search(ctx context.Context, limit int) ([]SearchResult, error)
}

// SearchResult is a single retrieval result.
type SearchResult struct {
	StepID int
	Score  float64
	Text   string
}

// FusionRetriever combines semantic, keyword, and recency search (§7.3).
//
// Weights: semantic 0.50 + keyword 0.30 + recency 0.20
// Threshold: 0.35 (results below this are filtered out)
type FusionRetriever struct {
	Semantic  SemanticSearcher
	Keyword   KeywordSearcher
	Recency   RecencySearcher
	Embedder  Embedder
	Weights   [3]float64
	Threshold float64
}

// NewFusionRetriever creates a fusion retriever with default weights.
func NewFusionRetriever(semantic SemanticSearcher, keyword KeywordSearcher, recency RecencySearcher, embedder Embedder) *FusionRetriever {
	return &FusionRetriever{
		Semantic:  semantic,
		Keyword:   keyword,
		Recency:   recency,
		Embedder:  embedder,
		Weights:   [3]float64{0.50, 0.30, 0.20},
		Threshold: 0.35,
	}
}

// Search performs three-way fusion retrieval.
func (f *FusionRetriever) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	// Get embeddings for query
	var queryVec []float32
	if f.Embedder != nil && f.Semantic != nil {
		var err error
		queryVec, err = f.Embedder.Embed(ctx, query)
		if err != nil {
			queryVec = nil
		}
	}

	// Run all three searches
	var semanticResults, keywordResults, recencyResults []SearchResult

	if f.Semantic != nil && queryVec != nil {
		semanticResults, _ = f.Semantic.Search(ctx, queryVec, limit*2)
	}
	if f.Keyword != nil {
		keywordResults, _ = f.Keyword.Search(ctx, query, limit*2)
	}
	if f.Recency != nil {
		recencyResults, _ = f.Recency.Search(ctx, limit*2)
	}

	// Fuse scores
	fused := f.fuseScores(semanticResults, keywordResults, recencyResults)

	// Filter by threshold and sort
	var filtered []SearchResult
	for _, r := range fused {
		if r.Score >= f.Threshold {
			filtered = append(filtered, r)
		}
	}

	// Limit results
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}

	return filtered, nil
}

// SearchLongMem searches long-term memory with fusion.
func (f *FusionRetriever) SearchLongMem(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	// Long-term memory uses the same fusion approach
	return f.Search(ctx, query, limit)
}

// fuseScores combines results from three retrievers using weighted sum.
func (f *FusionRetriever) fuseScores(semantic, keyword, recency []SearchResult) []SearchResult {
	scoreMap := make(map[int]float64)
	textMap := make(map[int]string)

	// Normalize and weight semantic scores
	maxS := maxScoreVal(semantic)
	if maxS > 0 {
		for _, r := range semantic {
			scoreMap[r.StepID] += (r.Score / maxS) * f.Weights[0]
			textMap[r.StepID] = r.Text
		}
	}

	// Normalize and weight keyword scores
	maxS = maxScoreVal(keyword)
	if maxS > 0 {
		for _, r := range keyword {
			scoreMap[r.StepID] += (r.Score / maxS) * f.Weights[1]
			if _, ok := textMap[r.StepID]; !ok {
				textMap[r.StepID] = r.Text
			}
		}
	}

	// Normalize and weight recency scores
	maxS = maxScoreVal(recency)
	if maxS > 0 {
		for _, r := range recency {
			scoreMap[r.StepID] += (r.Score / maxS) * f.Weights[2]
			if _, ok := textMap[r.StepID]; !ok {
				textMap[r.StepID] = r.Text
			}
		}
	}

	// Build results
	var results []SearchResult
	for id, score := range scoreMap {
		results = append(results, SearchResult{
			StepID: id,
			Score:  score,
			Text:   textMap[id],
		})
	}

	// Sort by score descending (simple bubble sort for now)
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	return results
}

func maxScoreVal(results []SearchResult) float64 {
	max := 0.0
	for _, r := range results {
		if r.Score > max {
			max = r.Score
		}
	}
	return max
}
