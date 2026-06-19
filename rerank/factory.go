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

import "fmt"

// NewRerankerFromConfig creates a Reranker based on the configuration.
// Supported providers: "cohere", "llm" (requires LLMCaller to be set via options).
func NewRerankerFromConfig(cfg Config) (Reranker, error) {
	switch cfg.Provider {
	case "cohere":
		if cfg.CohereAPIKey == "" {
			return nil, fmt.Errorf("rerank factory: cohere provider requires API key")
		}
		r := NewCohereReranker(cfg.CohereAPIKey)
		if cfg.CohereModel != "" {
			r.Model = cfg.CohereModel
		}
		if cfg.CohereBaseURL != "" {
			r.BaseURL = cfg.CohereBaseURL
		}
		if cfg.TopN > 0 {
			r.TopN = cfg.TopN
		}
		return r, nil

	case "llm":
		return nil, fmt.Errorf("rerank factory: llm provider requires an LLMCaller; use NewLLMReranker directly")

	default:
		return nil, fmt.Errorf("rerank factory: unknown provider %q", cfg.Provider)
	}
}

// NewRerankerFromConfigWithLLM creates an LLM-based Reranker with the given caller.
func NewRerankerFromConfigWithLLM(cfg Config, caller LLMCaller) (Reranker, error) {
	if cfg.Provider != "llm" {
		return nil, fmt.Errorf("rerank factory: expected provider 'llm', got %q", cfg.Provider)
	}
	if caller == nil {
		return nil, fmt.Errorf("rerank factory: llm provider requires a non-nil LLMCaller")
	}
	r := NewLLMReranker(caller)
	if cfg.TopN > 0 {
		r.TopN = cfg.TopN
	}
	return r, nil
}
