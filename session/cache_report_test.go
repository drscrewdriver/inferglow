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

package session

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCacheReport_SingleSession(t *testing.T) {
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	records := []UsageRecord{
		{Timestamp: now, Model: "gpt-4o", Provider: "openai", PromptTokens: 1000, CompletionTokens: 200, CachedTokens: 300, TotalTokens: 1200, Cost: 0.003, Currency: "USD"},
	}

	gen := &ReportGenerator{clock: func() time.Time { return now }}
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 30, 23, 59, 59, 0, time.UTC)

	report := gen.generateFromRecords(records, from, to, "")

	if report.Overall.TotalPromptTokens != 1000 {
		t.Fatalf("expected total_prompt_tokens 1000, got %d", report.Overall.TotalPromptTokens)
	}
	if report.Overall.TotalCachedTokens != 300 {
		t.Fatalf("expected total_cached_tokens 300, got %d", report.Overall.TotalCachedTokens)
	}
	if !approxEqual(report.Overall.CacheHitRate, 0.3) {
		t.Fatalf("expected cache_hit_rate 0.3, got %f", report.Overall.CacheHitRate)
	}
	if !approxEqual(report.Overall.ActualCost, 0.003) {
		t.Fatalf("expected actual_cost ~0.003, got %f", report.Overall.ActualCost)
	}
	if report.Overall.SessionCount != 1 {
		t.Fatalf("expected session_count 1, got %d", report.Overall.SessionCount)
	}
	if len(report.ByModel) != 1 {
		t.Fatalf("expected 1 model, got %d", len(report.ByModel))
	}
	if report.ByModel[0].Model != "gpt-4o" {
		t.Fatalf("expected model gpt-4o, got %s", report.ByModel[0].Model)
	}
}

func TestCacheReport_MultipleSessions(t *testing.T) {
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	records := []UsageRecord{
		{Timestamp: now, Model: "gpt-4o", Provider: "openai", PromptTokens: 1000, CompletionTokens: 200, CachedTokens: 300, TotalTokens: 1200, Cost: 0.003, Currency: "USD"},
		{Timestamp: now, Model: "gpt-4o-mini", Provider: "openai", PromptTokens: 500, CompletionTokens: 100, CachedTokens: 100, TotalTokens: 600, Cost: 0.0002, Currency: "USD"},
		{Timestamp: now, Model: "gpt-4o", Provider: "openai", PromptTokens: 2000, CompletionTokens: 400, CachedTokens: 500, TotalTokens: 2400, Cost: 0.006, Currency: "USD"},
	}

	gen := &ReportGenerator{clock: func() time.Time { return now }}
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 30, 23, 59, 59, 0, time.UTC)

	report := gen.generateFromRecords(records, from, to, "")

	if report.Overall.TotalPromptTokens != 3500 {
		t.Fatalf("expected total_prompt_tokens 3500, got %d", report.Overall.TotalPromptTokens)
	}
	if report.Overall.TotalCachedTokens != 900 {
		t.Fatalf("expected total_cached_tokens 900, got %d", report.Overall.TotalCachedTokens)
	}
	if !approxEqual(report.Overall.ActualCost, 0.0092) {
		t.Fatalf("expected actual_cost ~0.0092, got %f", report.Overall.ActualCost)
	}
	if report.Overall.SessionCount != 2 {
		t.Fatalf("expected session_count 2, got %d", report.Overall.SessionCount)
	}
	if len(report.ByModel) != 2 {
		t.Fatalf("expected 2 models, got %d", len(report.ByModel))
	}

	// Check model order (sorted alphabetically)
	if report.ByModel[0].Model != "gpt-4o" {
		t.Fatalf("expected first model gpt-4o, got %s", report.ByModel[0].Model)
	}
	if report.ByModel[1].Model != "gpt-4o-mini" {
		t.Fatalf("expected second model gpt-4o-mini, got %s", report.ByModel[1].Model)
	}

	// Verify gpt-4o summary
	if report.ByModel[0].TotalPromptTokens != 3000 {
		t.Fatalf("expected gpt-4o prompt_tokens 3000, got %d", report.ByModel[0].TotalPromptTokens)
	}
	if report.ByModel[0].TotalCachedTokens != 800 {
		t.Fatalf("expected gpt-4o cached_tokens 800, got %d", report.ByModel[0].TotalCachedTokens)
	}
}

func TestCacheReport_TimeRangeFiltering(t *testing.T) {
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	records := []UsageRecord{
		{Timestamp: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), Model: "gpt-4o", PromptTokens: 1000, CompletionTokens: 200, CachedTokens: 300, TotalTokens: 1200, Cost: 0.003, Currency: "USD"},
		{Timestamp: time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC), Model: "gpt-4o", PromptTokens: 500, CompletionTokens: 100, CachedTokens: 100, TotalTokens: 600, Cost: 0.0015, Currency: "USD"},
		{Timestamp: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), Model: "gpt-4o", PromptTokens: 2000, CompletionTokens: 400, CachedTokens: 500, TotalTokens: 2400, Cost: 0.006, Currency: "USD"},
	}

	gen := &ReportGenerator{clock: func() time.Time { return now }}
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 30, 23, 59, 59, 0, time.UTC)

	report := gen.generateFromRecords(records, from, to, "")

	if report.Overall.TotalPromptTokens != 500 {
		t.Fatalf("expected total_prompt_tokens 500 (only June), got %d", report.Overall.TotalPromptTokens)
	}
	if report.Overall.TotalCachedTokens != 100 {
		t.Fatalf("expected total_cached_tokens 100 (only June), got %d", report.Overall.TotalCachedTokens)
	}
	if report.Overall.SessionCount != 1 {
		t.Fatalf("expected SessionCount 1 (one model group), got %d", report.Overall.SessionCount)
	}
}

func TestCacheReport_ModelFiltering(t *testing.T) {
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	records := []UsageRecord{
		{Timestamp: now, Model: "gpt-4o", PromptTokens: 1000, CompletionTokens: 200, CachedTokens: 300, TotalTokens: 1200, Cost: 0.003, Currency: "USD"},
		{Timestamp: now, Model: "gpt-4o-mini", PromptTokens: 500, CompletionTokens: 100, CachedTokens: 100, TotalTokens: 600, Cost: 0.0002, Currency: "USD"},
		{Timestamp: now, Model: "claude-3-5-sonnet", PromptTokens: 2000, CompletionTokens: 400, CachedTokens: 500, TotalTokens: 2400, Cost: 0.008, Currency: "USD"},
	}

	gen := &ReportGenerator{clock: func() time.Time { return now }}
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 30, 23, 59, 59, 0, time.UTC)

	// Filter to only gpt-4o
	report := gen.generateFromRecords(records, from, to, "gpt-4o")

	if report.Overall.TotalPromptTokens != 1000 {
		t.Fatalf("expected total_prompt_tokens 1000 (gpt-4o only), got %d", report.Overall.TotalPromptTokens)
	}
	if len(report.ByModel) != 1 {
		t.Fatalf("expected 1 model, got %d", len(report.ByModel))
	}
	if report.ByModel[0].Model != "gpt-4o" {
		t.Fatalf("expected model gpt-4o, got %s", report.ByModel[0].Model)
	}
	if report.ByModel[0].TotalPromptTokens != 1000 {
		t.Fatalf("expected gpt-4o prompt_tokens 1000, got %d", report.ByModel[0].TotalPromptTokens)
	}
}

func TestCacheReport_EmptyData(t *testing.T) {
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	gen := &ReportGenerator{clock: func() time.Time { return now }}
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 30, 23, 59, 59, 0, time.UTC)

	report := gen.generateFromRecords(nil, from, to, "")

	if report.Overall.TotalPromptTokens != 0 {
		t.Fatalf("expected total_prompt_tokens 0, got %d", report.Overall.TotalPromptTokens)
	}
	if report.Overall.TotalCachedTokens != 0 {
		t.Fatalf("expected total_cached_tokens 0, got %d", report.Overall.TotalCachedTokens)
	}
	if report.Overall.CacheHitRate != 0 {
		t.Fatalf("expected cache_hit_rate 0, got %f", report.Overall.CacheHitRate)
	}
	if report.Overall.TotalCost != 0 {
		t.Fatalf("expected total_cost 0, got %f", report.Overall.TotalCost)
	}
	if report.Overall.ActualCost != 0 {
		t.Fatalf("expected actual_cost 0, got %f", report.Overall.ActualCost)
	}
	if report.Overall.Savings != 0 {
		t.Fatalf("expected savings 0, got %f", report.Overall.Savings)
	}
	if len(report.ByModel) != 0 {
		t.Fatalf("expected 0 models, got %d", len(report.ByModel))
	}
}

func TestCacheReport_NoRecordsInRange(t *testing.T) {
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	records := []UsageRecord{
		{Timestamp: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), Model: "gpt-4o", PromptTokens: 1000, CompletionTokens: 200, CachedTokens: 300, TotalTokens: 1200, Cost: 0.003, Currency: "USD"},
	}

	gen := &ReportGenerator{clock: func() time.Time { return now }}
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 30, 23, 59, 59, 0, time.UTC)

	report := gen.generateFromRecords(records, from, to, "")

	if report.Overall.TotalPromptTokens != 0 {
		t.Fatalf("expected total_prompt_tokens 0 for empty range, got %d", report.Overall.TotalPromptTokens)
	}
	if len(report.ByModel) != 0 {
		t.Fatalf("expected 0 models for empty range, got %d", len(report.ByModel))
	}
}

func TestCacheReport_CacheHitRateCalculation(t *testing.T) {
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	records := []UsageRecord{
		{Timestamp: now, Model: "gpt-4o", PromptTokens: 1000, CompletionTokens: 200, CachedTokens: 250, TotalTokens: 1200, Cost: 0.003, Currency: "USD"},
		{Timestamp: now, Model: "gpt-4o", PromptTokens: 500, CompletionTokens: 100, CachedTokens: 500, TotalTokens: 600, Cost: 0.0015, Currency: "USD"},
		{Timestamp: now, Model: "gpt-4o", PromptTokens: 2000, CompletionTokens: 400, CachedTokens: 0, TotalTokens: 2400, Cost: 0.006, Currency: "USD"},
	}

	gen := &ReportGenerator{clock: func() time.Time { return now }}
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 30, 23, 59, 59, 0, time.UTC)

	report := gen.generateFromRecords(records, from, to, "")

	// Total prompt: 3500, Total cached: 750
	// Cache hit rate: 750 / 3500 = 0.2142857...
	expectedRate := float64(750) / float64(3500)
	if !approxEqual(report.Overall.CacheHitRate, expectedRate) {
		t.Fatalf("expected cache_hit_rate %f, got %f", expectedRate, report.Overall.CacheHitRate)
	}
	if !approxEqual(report.ByModel[0].CacheHitRate, expectedRate) {
		t.Fatalf("expected model cache_hit_rate %f, got %f", expectedRate, report.ByModel[0].CacheHitRate)
	}
}

func TestCacheReport_SavingsCalculation(t *testing.T) {
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	// gpt-4o pricing: Input=2.50, CacheHit=1.25, Output=10.00
	// With 1000 prompt tokens, 300 cached, 200 completion:
	//   Actual cost (with cache): (700 * 2.50 + 300 * 1.25 + 200 * 10.00) / 1e6 = (1750 + 375 + 2000) / 1e6 = 0.004125
	//   Total cost (without cache): (1000 * 2.50 + 200 * 10.00) / 1e6 = (2500 + 2000) / 1e6 = 0.0045
	//   Savings: 0.0045 - 0.004125 = 0.000375
	records := []UsageRecord{
		{Timestamp: now, Model: "gpt-4o", PromptTokens: 1000, CompletionTokens: 200, CachedTokens: 300, TotalTokens: 1200, Cost: 0.004125, Currency: "USD"},
	}

	gen := &ReportGenerator{clock: func() time.Time { return now }}
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 30, 23, 59, 59, 0, time.UTC)

	report := gen.generateFromRecords(records, from, to, "")

	expectedTotalCost := 0.0045    // without cache
	expectedActualCost := 0.004125 // with cache
	expectedSavings := 0.000375

	if !approxEqual(report.Overall.TotalCost, expectedTotalCost) {
		t.Fatalf("expected TotalCost %f, got %f", expectedTotalCost, report.Overall.TotalCost)
	}
	if !approxEqual(report.Overall.ActualCost, expectedActualCost) {
		t.Fatalf("expected ActualCost %f, got %f", expectedActualCost, report.Overall.ActualCost)
	}
	if !approxEqual(report.Overall.Savings, expectedSavings) {
		t.Fatalf("expected Savings %f, got %f", expectedSavings, report.Overall.Savings)
	}
}

func TestCacheReport_SavingsWithNoPricing(t *testing.T) {
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	records := []UsageRecord{
		{Timestamp: now, Model: "unknown-model", PromptTokens: 1000, CompletionTokens: 200, CachedTokens: 300, TotalTokens: 1200, Cost: 0.003, Currency: "USD"},
	}

	gen := &ReportGenerator{clock: func() time.Time { return now }}
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 30, 23, 59, 59, 0, time.UTC)

	report := gen.generateFromRecords(records, from, to, "")

	// No pricing info: TotalCost should equal ActualCost, Savings = 0
	if !approxEqual(report.Overall.TotalCost, 0.003) {
		t.Fatalf("expected TotalCost ~0.003 (same as actual when no pricing), got %f", report.Overall.TotalCost)
	}
	if report.Overall.Savings != 0 {
		t.Fatalf("expected Savings 0 when no pricing, got %f", report.Overall.Savings)
	}
}

func TestCacheReport_GenerateFromFiles(t *testing.T) {
	dir := t.TempDir()
	sessionsDir := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)

	// Write session 1 usage file
	records1 := []UsageRecord{
		{Timestamp: now, Model: "gpt-4o", Provider: "openai", PromptTokens: 1000, CompletionTokens: 200, CachedTokens: 300, TotalTokens: 1200, Cost: 0.003, Currency: "USD"},
		{Timestamp: now, Model: "gpt-4o-mini", Provider: "openai", PromptTokens: 500, CompletionTokens: 100, CachedTokens: 100, TotalTokens: 600, Cost: 0.0002, Currency: "USD"},
	}
	writeUsageFile(t, filepath.Join(sessionsDir, "session-1.usage.jsonl"), records1)

	// Write session 2 usage file
	records2 := []UsageRecord{
		{Timestamp: now, Model: "gpt-4o", Provider: "openai", PromptTokens: 2000, CompletionTokens: 400, CachedTokens: 500, TotalTokens: 2400, Cost: 0.006, Currency: "USD"},
	}
	writeUsageFile(t, filepath.Join(sessionsDir, "session-2.usage.jsonl"), records2)

	gen := NewReportGenerator(dir)
	gen.clock = func() time.Time { return now }

	from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 30, 23, 59, 59, 0, time.UTC)

	report, err := gen.Generate(context.Background(), from, to, "")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if report.Overall.TotalPromptTokens != 3500 {
		t.Fatalf("expected total_prompt_tokens 3500, got %d", report.Overall.TotalPromptTokens)
	}
	if report.Overall.TotalCachedTokens != 900 {
		t.Fatalf("expected total_cached_tokens 900, got %d", report.Overall.TotalCachedTokens)
	}
	if !approxEqual(report.Overall.ActualCost, 0.0092) {
		t.Fatalf("expected actual_cost ~0.0092, got %f", report.Overall.ActualCost)
	}
	if len(report.ByModel) != 2 {
		t.Fatalf("expected 2 models, got %d", len(report.ByModel))
	}
}

func TestCacheReport_GenerateFromFiles_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	gen := NewReportGenerator(dir)
	gen.clock = func() time.Time { return time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC) }

	from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 30, 23, 59, 59, 0, time.UTC)

	report, err := gen.Generate(context.Background(), from, to, "")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if report.Overall.TotalPromptTokens != 0 {
		t.Fatalf("expected total_prompt_tokens 0, got %d", report.Overall.TotalPromptTokens)
	}
	if len(report.ByModel) != 0 {
		t.Fatalf("expected 0 models, got %d", len(report.ByModel))
	}
}

func TestCacheReport_GenerateFromFiles_ModelFilter(t *testing.T) {
	dir := t.TempDir()
	sessionsDir := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)

	records := []UsageRecord{
		{Timestamp: now, Model: "gpt-4o", PromptTokens: 1000, CompletionTokens: 200, CachedTokens: 300, TotalTokens: 1200, Cost: 0.003, Currency: "USD"},
		{Timestamp: now, Model: "gpt-4o-mini", PromptTokens: 500, CompletionTokens: 100, CachedTokens: 100, TotalTokens: 600, Cost: 0.0002, Currency: "USD"},
	}
	writeUsageFile(t, filepath.Join(sessionsDir, "session-1.usage.jsonl"), records)

	gen := NewReportGenerator(dir)
	gen.clock = func() time.Time { return now }

	from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 30, 23, 59, 59, 0, time.UTC)

	report, err := gen.Generate(context.Background(), from, to, "gpt-4o")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if report.Overall.TotalPromptTokens != 1000 {
		t.Fatalf("expected total_prompt_tokens 1000, got %d", report.Overall.TotalPromptTokens)
	}
	if len(report.ByModel) != 1 {
		t.Fatalf("expected 1 model, got %d", len(report.ByModel))
	}
	if report.ByModel[0].Model != "gpt-4o" {
		t.Fatalf("expected model gpt-4o, got %s", report.ByModel[0].Model)
	}
}

func TestCacheReport_IndexFileIgnored(t *testing.T) {
	dir := t.TempDir()
	sessionsDir := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)

	// Write an index.jsonl file (should be ignored)
	indexContent := `{"uuid":"test","title":"test","created_at":1000000,"updated_at":1000000}
`
	if err := os.WriteFile(filepath.Join(sessionsDir, "index.jsonl"), []byte(indexContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write a valid usage file
	records := []UsageRecord{
		{Timestamp: now, Model: "gpt-4o", PromptTokens: 1000, CompletionTokens: 200, CachedTokens: 300, TotalTokens: 1200, Cost: 0.003, Currency: "USD"},
	}
	writeUsageFile(t, filepath.Join(sessionsDir, "session-1.usage.jsonl"), records)

	gen := NewReportGenerator(dir)
	gen.clock = func() time.Time { return now }

	from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 30, 23, 59, 59, 0, time.UTC)

	report, err := gen.Generate(context.Background(), from, to, "")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if report.Overall.TotalPromptTokens != 1000 {
		t.Fatalf("expected total_prompt_tokens 1000, got %d", report.Overall.TotalPromptTokens)
	}
	if len(report.ByModel) != 1 {
		t.Fatalf("expected 1 model, got %d", len(report.ByModel))
	}
}

func TestCacheReport_GeneratedAt(t *testing.T) {
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	gen := &ReportGenerator{clock: func() time.Time { return now }}
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 30, 23, 59, 59, 0, time.UTC)

	report := gen.generateFromRecords(nil, from, to, "")

	if !report.GeneratedAt.Equal(now) {
		t.Fatalf("expected GeneratedAt %v, got %v", now, report.GeneratedAt)
	}
	if !report.From.Equal(from) {
		t.Fatalf("expected From %v, got %v", from, report.From)
	}
	if !report.To.Equal(to) {
		t.Fatalf("expected To %v, got %v", to, report.To)
	}
}

func TestCacheReport_ZeroCachedTokens(t *testing.T) {
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	records := []UsageRecord{
		{Timestamp: now, Model: "gpt-4o", PromptTokens: 1000, CompletionTokens: 200, CachedTokens: 0, TotalTokens: 1200, Cost: 0.003, Currency: "USD"},
	}

	gen := &ReportGenerator{clock: func() time.Time { return now }}
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 30, 23, 59, 59, 0, time.UTC)

	report := gen.generateFromRecords(records, from, to, "")

	if !approxEqual(report.Overall.CacheHitRate, 0) {
		t.Fatalf("expected cache_hit_rate 0, got %f", report.Overall.CacheHitRate)
	}
	if report.Overall.TotalCachedTokens != 0 {
		t.Fatalf("expected total_cached_tokens 0, got %d", report.Overall.TotalCachedTokens)
	}
}

func TestCacheReport_AllCachedTokens(t *testing.T) {
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	records := []UsageRecord{
		{Timestamp: now, Model: "gpt-4o", PromptTokens: 1000, CompletionTokens: 200, CachedTokens: 1000, TotalTokens: 1200, Cost: 0.0015, Currency: "USD"},
	}

	gen := &ReportGenerator{clock: func() time.Time { return now }}
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 30, 23, 59, 59, 0, time.UTC)

	report := gen.generateFromRecords(records, from, to, "")

	if !approxEqual(report.Overall.CacheHitRate, 1.0) {
		t.Fatalf("expected cache_hit_rate 1.0, got %f", report.Overall.CacheHitRate)
	}
	if report.Overall.TotalCachedTokens != 1000 {
		t.Fatalf("expected total_cached_tokens 1000, got %d", report.Overall.TotalCachedTokens)
	}
}

// writeUsageFile writes records as JSONL to a file.
func writeUsageFile(t *testing.T, path string, records []UsageRecord) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	for _, r := range records {
		data, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(append(data, '\n')); err != nil {
			t.Fatal(err)
		}
	}
}

// approxEqual checks if two float64 values are approximately equal within a small epsilon.
func approxEqual(a, b float64) bool {
	epsilon := 1e-9
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < epsilon
}
