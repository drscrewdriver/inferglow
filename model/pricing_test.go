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

import (
	"math"
	"testing"
)

func TestPricingCost(t *testing.T) {
	tests := []struct {
		name     string
		pricing  *Pricing
		usage    *UsageInfo
		expected float64
	}{
		{
			name:     "nil pricing returns 0",
			pricing:  nil,
			usage:    &UsageInfo{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
			expected: 0,
		},
		{
			name:     "nil usage returns 0",
			pricing:  &Pricing{Input: 1.0, Output: 2.0, Currency: "USD"},
			usage:    nil,
			expected: 0,
		},
		{
			name:    "pure input/output no cache",
			pricing: &Pricing{Input: 1.0, Output: 2.0, Currency: "USD"},
			usage: &UsageInfo{
				PromptTokens:     1_000_000,
				CompletionTokens: 1_000_000,
				TotalTokens:      2_000_000,
			},
			expected: 3.0, // 1M * $1/M + 1M * $2/M
		},
		{
			name:    "with cache hit",
			pricing: &Pricing{CacheHit: 0.5, Input: 1.0, Output: 2.0, Currency: "USD"},
			usage: &UsageInfo{
				PromptTokens:          1_000_000,
				CompletionTokens:      500_000,
				TotalTokens:           1_500_000,
				PromptTokensDetails:   map[string]int{"cached_tokens": 400_000},
			},
			// uncached: 600K * $1/M = $0.6
			// cached:   400K * $0.5/M = $0.2
			// output:   500K * $2/M = $1.0
			// total = $1.8
			expected: 1.8,
		},
		{
			name:    "all cached",
			pricing: &Pricing{CacheHit: 0.1, Input: 1.0, Output: 2.0, Currency: "USD"},
			usage: &UsageInfo{
				PromptTokens:        1_000_000,
				CompletionTokens:    0,
				TotalTokens:         1_000_000,
				PromptTokensDetails: map[string]int{"cached_tokens": 1_000_000},
			},
			expected: 0.1, // 1M * $0.1/M
		},
		{
			name:    "zero pricing",
			pricing: &Pricing{},
			usage: &UsageInfo{
				PromptTokens:     1_000_000,
				CompletionTokens: 1_000_000,
				TotalTokens:      2_000_000,
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.pricing.Cost(tt.usage)
			if math.Abs(got-tt.expected) > 1e-6 {
				t.Errorf("Cost() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSessionCostAdd(t *testing.T) {
	pricing := &Pricing{CacheHit: 0.5, Input: 1.0, Output: 2.0, Currency: "USD"}
	sc := &SessionCost{}

	u1 := &UsageInfo{
		PromptTokens:        100_000,
		CompletionTokens:    50_000,
		TotalTokens:         150_000,
		PromptTokensDetails: map[string]int{"cached_tokens": 30_000},
	}
	u2 := &UsageInfo{
		PromptTokens:     200_000,
		CompletionTokens: 100_000,
		TotalTokens:      300_000,
	}

	sc.Add(u1, pricing)
	sc.Add(u2, pricing)

	if sc.Currency != "USD" {
		t.Errorf("Currency = %q, want USD", sc.Currency)
	}
	if sc.TotalTokens != 450_000 {
		t.Errorf("TotalTokens = %d, want 450000", sc.TotalTokens)
	}
	if sc.CachedTokens != 30_000 {
		t.Errorf("CachedTokens = %d, want 30000", sc.CachedTokens)
	}
	if sc.TotalCost <= 0 {
		t.Errorf("TotalCost should be positive, got %v", sc.TotalCost)
	}
}

func TestSessionCostAddNilSafety(t *testing.T) {
	sc := &SessionCost{}
	sc.Add(nil, &Pricing{}) // should not panic
	sc.Add(&UsageInfo{}, nil) // should not panic

	var nilSC *SessionCost
	nilSC.Add(&UsageInfo{}, &Pricing{}) // should not panic
}
