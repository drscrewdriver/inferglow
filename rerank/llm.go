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
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// LLMCaller is an interface for generating text from a prompt using an LLM.
type LLMCaller interface {
	// Generate sends a prompt to the LLM and returns the generated text.
	Generate(ctx context.Context, prompt string) (string, error)
}

// LLMReranker uses an LLM to perform pairwise ranking of documents.
type LLMReranker struct {
	// Caller is the LLM caller used for generating rankings.
	Caller LLMCaller

	// TopN limits the number of results returned. 0 means return all.
	TopN int
}

// NewLLMReranker creates an LLMReranker with the given LLM caller.
func NewLLMReranker(caller LLMCaller) *LLMReranker {
	return &LLMReranker{Caller: caller}
}

// Rerank uses the LLM to score documents against the query.
// It asks the LLM to rate each document's relevance on a scale of 0-10.
func (r *LLMReranker) Rerank(ctx context.Context, query string, docs []Document) ([]RankedDocument, error) {
	if len(docs) == 0 {
		return nil, nil
	}

	prompt := buildRankingPrompt(query, docs)
	response, err := r.Caller.Generate(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("rerank llm: generate: %w", err)
	}

	scores := parseRankingResponse(response, len(docs))

	ranked := make([]RankedDocument, len(docs))
	for i, doc := range docs {
		ranked[i] = RankedDocument{
			Document: doc,
			Score:    scores[i],
			Index:    i,
		}
	}

	// Sort by score descending
	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].Score > ranked[j].Score
	})

	// Apply TopN limit
	if r.TopN > 0 && len(ranked) > r.TopN {
		ranked = ranked[:r.TopN]
	}

	return ranked, nil
}

// buildRankingPrompt constructs the prompt for LLM-based ranking.
func buildRankingPrompt(query string, docs []Document) string {
	var sb strings.Builder
	sb.WriteString("You are a relevance ranking assistant. Given a query and a list of documents, ")
	sb.WriteString("rate each document's relevance to the query on a scale from 0 to 10, ")
	sb.WriteString("where 0 is completely irrelevant and 10 is perfectly relevant.\n\n")
	sb.WriteString("Query: ")
	sb.WriteString(query)
	sb.WriteString("\n\nDocuments:\n")

	for i, doc := range docs {
		sb.WriteString(fmt.Sprintf("[Document %d]: %s\n", i, truncateForPrompt(doc.Content, 500)))
	}

	sb.WriteString("\nRate each document. Respond with one line per document in the format: ")
	sb.WriteString("Document N: score\n")
	sb.WriteString("Only output the ratings, nothing else.\n")

	return sb.String()
}

// truncateForPrompt truncates text to a maximum number of characters.
func truncateForPrompt(text string, maxChars int) string {
	if len(text) <= maxChars {
		return text
	}
	return text[:maxChars] + "..."
}

// parseRankingResponse parses the LLM's ranking response into scores.
func parseRankingResponse(response string, numDocs int) []float64 {
	scores := make([]float64, numDocs)

	lines := strings.Split(response, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Try to parse "Document N: score" format
		for i := 0; i < numDocs; i++ {
			prefix := fmt.Sprintf("Document %d:", i)
			if strings.HasPrefix(line, prefix) {
				scoreStr := strings.TrimSpace(strings.TrimPrefix(line, prefix))
				// Remove any trailing text after the number
				scoreStr = strings.Fields(scoreStr)[0]
				if score, err := strconv.ParseFloat(scoreStr, 64); err == nil {
					scores[i] = score
				}
				break
			}
		}
	}

	return scores
}
