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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
)

const defaultCohereURL = "https://api.cohere.ai/v1/rerank"

// HTTPDoer is an interface for making HTTP requests.
// *http.Client satisfies this interface.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// CohereReranker uses the Cohere Rerank API to rerank documents.
type CohereReranker struct {
	// APIKey is the Cohere API key.
	APIKey string

	// Model is the rerank model name. Default: "rerank-english-v3.0".
	Model string

	// BaseURL overrides the default Cohere API URL.
	BaseURL string

	// TopN limits the number of results returned. 0 means return all.
	TopN int

	// Client is the HTTP client used for API calls.
	// If nil, http.DefaultClient is used.
	Client HTTPDoer
}

// NewCohereReranker creates a CohereReranker with the given API key.
func NewCohereReranker(apiKey string) *CohereReranker {
	return &CohereReranker{
		APIKey: apiKey,
		Model:  "rerank-english-v3.0",
		Client: http.DefaultClient,
	}
}

type cohereRequest struct {
	Query          string   `json:"query"`
	Documents      []string `json:"documents"`
	Model          string   `json:"model"`
	TopN           int      `json:"top_n,omitempty"`
	ReturnDocuments bool    `json:"return_documents"`
}

type cohereResponse struct {
	Results []cohereResult `json:"results"`
}

type cohereResult struct {
	Index          int     `json:"index"`
	RelevanceScore float64 `json:"relevance_score"`
}

// Rerank calls the Cohere Rerank API and returns documents sorted by relevance.
func (r *CohereReranker) Rerank(ctx context.Context, query string, docs []Document) ([]RankedDocument, error) {
	if len(docs) == 0 {
		return nil, nil
	}

	model := r.Model
	if model == "" {
		model = "rerank-english-v3.0"
	}
	baseURL := r.BaseURL
	if baseURL == "" {
		baseURL = defaultCohereURL
	}

	// Build request body
	docTexts := make([]string, len(docs))
	for i, d := range docs {
		docTexts[i] = d.Content
	}

	reqBody := cohereRequest{
		Query:           query,
		Documents:       docTexts,
		Model:           model,
		ReturnDocuments: false,
	}
	if r.TopN > 0 {
		reqBody.TopN = r.TopN
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("rerank cohere: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("rerank cohere: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.APIKey)

	client := r.Client
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rerank cohere: http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("rerank cohere: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rerank cohere: API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var cohereResp cohereResponse
	if err := json.Unmarshal(respBody, &cohereResp); err != nil {
		return nil, fmt.Errorf("rerank cohere: unmarshal response: %w", err)
	}

	// Build ranked documents
	ranked := make([]RankedDocument, len(cohereResp.Results))
	for i, result := range cohereResp.Results {
		if result.Index >= 0 && result.Index < len(docs) {
			ranked[i] = RankedDocument{
				Document: docs[result.Index],
				Score:    result.RelevanceScore,
				Index:    result.Index,
			}
		}
	}

	// Sort by score descending
	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].Score > ranked[j].Score
	})

	return ranked, nil
}
