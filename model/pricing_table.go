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

import "strings"

// DefaultPricingTable contains per-million-token pricing for mainstream
// LLM providers. All rates are in USD per 1M tokens.
//
// Sources (last verified 2026-07):
//   - OpenAI:     platform.openai.com/pricing
//   - Anthropic:  anthropic.com/pricing
//   - Google:     ai.google.dev/pricing
//   - DeepSeek:   platform.deepseek.com/pricing
//   - Qwen:       dashscope.aliyun.com/pricing
//   - Cohere:     cohere.com/pricing
//
// This table is intentionally conservative. Callers should verify current
// pricing before using for billing purposes. Update quarterly.
var DefaultPricingTable = map[string]Pricing{
	// --- OpenAI ---
	"gpt-4o":          {Input: 2.50, CacheHit: 1.25, Output: 10.00, Currency: "USD"},
	"gpt-4o-2024-08-06": {Input: 2.50, CacheHit: 1.25, Output: 10.00, Currency: "USD"},
	"gpt-4o-mini":     {Input: 0.15, CacheHit: 0.075, Output: 0.60, Currency: "USD"},
	"gpt-4o-mini-2024-07-18": {Input: 0.15, CacheHit: 0.075, Output: 0.60, Currency: "USD"},
	"gpt-4-turbo":     {Input: 10.00, CacheHit: 5.00, Output: 30.00, Currency: "USD"},
	"gpt-4":           {Input: 30.00, CacheHit: 15.00, Output: 60.00, Currency: "USD"},
	"gpt-3.5-turbo":   {Input: 0.50, CacheHit: 0.25, Output: 1.50, Currency: "USD"},
	"o1":              {Input: 15.00, CacheHit: 7.50, Output: 60.00, Currency: "USD"},
	"o1-mini":         {Input: 3.00, CacheHit: 1.50, Output: 12.00, Currency: "USD"},
	"o1-pro":          {Input: 150.00, CacheHit: 75.00, Output: 600.00, Currency: "USD"},
	"o3":              {Input: 10.00, CacheHit: 2.50, Output: 40.00, Currency: "USD"},
	"o3-mini":         {Input: 1.10, CacheHit: 0.55, Output: 4.40, Currency: "USD"},
	"o4-mini":         {Input: 1.10, CacheHit: 0.275, Output: 4.40, Currency: "USD"},

	// --- Anthropic ---
	"claude-opus-4-20250514":  {Input: 15.00, CacheHit: 1.50, Output: 75.00, Currency: "USD"},
	"claude-sonnet-4-20250514": {Input: 3.00, CacheHit: 0.30, Output: 15.00, Currency: "USD"},
	"claude-3-7-sonnet-20250219": {Input: 3.00, CacheHit: 0.30, Output: 15.00, Currency: "USD"},
	"claude-3-5-sonnet-20241022": {Input: 3.00, CacheHit: 0.30, Output: 15.00, Currency: "USD"},
	"claude-3-5-haiku-20241022": {Input: 0.80, CacheHit: 0.08, Output: 4.00, Currency: "USD"},
	"claude-3-opus-20240229": {Input: 15.00, CacheHit: 1.50, Output: 75.00, Currency: "USD"},
	"claude-3-haiku-20240307": {Input: 0.25, CacheHit: 0.025, Output: 1.25, Currency: "USD"},

	// --- Google ---
	"gemini-2.5-pro":   {Input: 1.25, CacheHit: 0.315, Output: 10.00, Currency: "USD"},
	"gemini-2.5-flash": {Input: 0.15, CacheHit: 0.0375, Output: 0.60, Currency: "USD"},
	"gemini-2.0-flash": {Input: 0.10, CacheHit: 0.025, Output: 0.40, Currency: "USD"},
	"gemini-1.5-pro":   {Input: 1.25, CacheHit: 0.315, Output: 5.00, Currency: "USD"},
	"gemini-1.5-flash": {Input: 0.075, CacheHit: 0.01875, Output: 0.30, Currency: "USD"},

	// --- DeepSeek ---
	"deepseek-chat":     {Input: 0.27, CacheHit: 0.07, Output: 1.10, Currency: "USD"},
	"deepseek-v3":       {Input: 0.27, CacheHit: 0.07, Output: 1.10, Currency: "USD"},
	"deepseek-reasoner": {Input: 0.55, CacheHit: 0.14, Output: 2.19, Currency: "USD"},
	"deepseek-r1":       {Input: 0.55, CacheHit: 0.14, Output: 2.19, Currency: "USD"},

	// --- Qwen / DashScope ---
	"qwen-turbo":        {Input: 0.30, CacheHit: 0.15, Output: 0.60, Currency: "USD"},
	"qwen-plus":         {Input: 0.80, CacheHit: 0.40, Output: 2.00, Currency: "USD"},
	"qwen-max":          {Input: 2.00, CacheHit: 1.00, Output: 6.00, Currency: "USD"},
	"qwen-long":         {Input: 0.05, CacheHit: 0.025, Output: 0.20, Currency: "USD"},

	// --- Cohere ---
	"command-r":         {Input: 0.15, CacheHit: 0.075, Output: 0.60, Currency: "USD"},
	"command-r-plus":    {Input: 2.50, CacheHit: 1.25, Output: 10.00, Currency: "USD"},
	"command-light":     {Input: 0.15, CacheHit: 0.075, Output: 0.60, Currency: "USD"},
	"command":           {Input: 1.00, CacheHit: 0.50, Output: 2.00, Currency: "USD"},
}

// LookupPricing returns the pricing for a model name.
// It performs exact match first, then prefix match (e.g. "gpt-4o-2024-08-06"
// matches "gpt-4o" if no exact entry exists).
//
// Returns (pricing, true) on match, (zero, false) if not found.
func LookupPricing(modelName string) (Pricing, bool) {
	// 1. Exact match.
	if p, ok := DefaultPricingTable[modelName]; ok {
		return p, true
	}

	// 2. Prefix match: find the longest key that is a prefix of modelName.
	bestKey := ""
	bestLen := 0
	for key := range DefaultPricingTable {
		if strings.HasPrefix(modelName, key) && len(key) > bestLen {
			bestKey = key
			bestLen = len(key)
		}
	}
	if bestKey != "" {
		return DefaultPricingTable[bestKey], true
	}

	return Pricing{}, false
}

// ListPricedModels returns the names of all models in the pricing table.
func ListPricedModels() []string {
	names := make([]string, 0, len(DefaultPricingTable))
	for name := range DefaultPricingTable {
		names = append(names, name)
	}
	return names
}

// AddPricing adds or overrides a model's pricing in the default table.
// Useful for custom models or updated pricing without modifying source.
func AddPricing(modelName string, pricing Pricing) {
	DefaultPricingTable[modelName] = pricing
}
