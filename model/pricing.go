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

import "math"

// Pricing defines the cost model for a single model.
// All rates are expressed in currency units per million tokens.
//
// Pricing is distinct from PricingInfo (router.go): PricingInfo is used
// for Router path selection/sorting, while Pricing is used for actual
// cost calculation with cache-hit differentiation.
type Pricing struct {
	// CacheHit is the cost per 1M cached prompt tokens.
	CacheHit float64 `json:"cache_hit" yaml:"cache_hit"`
	// Input is the cost per 1M uncached prompt tokens.
	Input float64 `json:"input" yaml:"input"`
	// Output is the cost per 1M completion tokens.
	Output float64 `json:"output" yaml:"output"`
	// Currency is the ISO 4217 currency code (e.g. "USD").
	Currency string `json:"currency" yaml:"currency"`
}

// Cost calculates the total cost for a given UsageInfo.
// It differentiates cached tokens (from PromptTokensDetails["cached_tokens"])
// from uncached input tokens.
//
// Returns 0 if p or u is nil.
func (p *Pricing) Cost(u *UsageInfo) float64 {
	if p == nil || u == nil {
		return 0
	}

	cachedTokens := 0
	if u.PromptTokensDetails != nil {
		cachedTokens = u.PromptTokensDetails["cached_tokens"]
	}
	uncachedInput := u.PromptTokens - cachedTokens
	if uncachedInput < 0 {
		uncachedInput = 0
	}

	const perMillion = 1e6
	cost := float64(uncachedInput)*p.Input/perMillion +
		float64(cachedTokens)*p.CacheHit/perMillion +
		float64(u.CompletionTokens)*p.Output/perMillion

	return math.Round(cost*1e6) / 1e6 // round to 6 decimal places
}

// SessionCost accumulates cost across multiple turns in a session.
type SessionCost struct {
	// TotalCost is the accumulated cost in the given Currency.
	TotalCost float64 `json:"total_cost"`
	// CachedTokens is the total number of cached prompt tokens.
	CachedTokens int `json:"cached_tokens"`
	// TotalTokens is the total number of tokens (input + output).
	TotalTokens int `json:"total_tokens"`
	// Currency is the ISO 4217 currency code.
	Currency string `json:"currency"`
}

// Add accumulates cost from a single UsageInfo using the given Pricing.
func (sc *SessionCost) Add(u *UsageInfo, p *Pricing) {
	if sc == nil || u == nil || p == nil {
		return
	}

	cachedTokens := 0
	if u.PromptTokensDetails != nil {
		cachedTokens = u.PromptTokensDetails["cached_tokens"]
	}

	sc.TotalCost += p.Cost(u)
	sc.CachedTokens += cachedTokens
	sc.TotalTokens += u.TotalTokens
	if sc.Currency == "" {
		sc.Currency = p.Currency
	}
}
