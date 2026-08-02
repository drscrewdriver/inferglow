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
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/inferglow/model"
)

// fixedTime returns a fixed timestamp for deterministic tests.
func fixedTime() time.Time {
	return time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)
}

func TestUsageRecorder_RecordAndSummary(t *testing.T) {
	dir := t.TempDir()
	recorder := NewUsageRecorder("test-session-1", dir, nil)

	usage1 := model.UsageInfo{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
	}
	usage2 := model.UsageInfo{
		PromptTokens:     200,
		CompletionTokens: 80,
		TotalTokens:      280,
	}

	recorder.Record(usage1, "gpt-4", "openai")
	recorder.Record(usage2, "gpt-4", "openai")

	summary := recorder.Summary()

	if summary.SessionID != "test-session-1" {
		t.Fatalf("expected sessionID test-session-1, got %s", summary.SessionID)
	}
	if summary.TotalPromptTokens != 300 {
		t.Fatalf("expected total_prompt_tokens 300, got %d", summary.TotalPromptTokens)
	}
	if summary.TotalCompletionTokens != 130 {
		t.Fatalf("expected total_completion_tokens 130, got %d", summary.TotalCompletionTokens)
	}
	if summary.TotalTokens != 430 {
		t.Fatalf("expected total_tokens 430, got %d", summary.TotalTokens)
	}
	if summary.RecordCount != 2 {
		t.Fatalf("expected record_count 2, got %d", summary.RecordCount)
	}
	if summary.TotalCost != 0 {
		t.Fatalf("expected total_cost 0 (no pricing), got %f", summary.TotalCost)
	}
	if len(summary.Records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(summary.Records))
	}
}

func TestUsageRecorder_WithPricing(t *testing.T) {
	dir := t.TempDir()
	pricing := &model.Pricing{
		Input:    3.0,  // $3 per 1M input tokens
		Output:   15.0, // $15 per 1M output tokens
		CacheHit: 0.15, // $0.15 per 1M cached tokens
		Currency: "USD",
	}

	recorder := NewUsageRecorder("test-session-2", dir, pricing)

	usage := model.UsageInfo{
		PromptTokens:     1000,
		CompletionTokens: 500,
		TotalTokens:      1500,
		PromptTokensDetails: map[string]int{
			"cached_tokens": 200,
		},
	}

	recorder.Record(usage, "claude-3", "anthropic")

	summary := recorder.Summary()

	if summary.TotalPromptTokens != 1000 {
		t.Fatalf("expected total_prompt_tokens 1000, got %d", summary.TotalPromptTokens)
	}
	if summary.TotalCachedTokens != 200 {
		t.Fatalf("expected total_cached_tokens 200, got %d", summary.TotalCachedTokens)
	}
	if summary.TotalCompletionTokens != 500 {
		t.Fatalf("expected total_completion_tokens 500, got %d", summary.TotalCompletionTokens)
	}
	if summary.TotalReasoningTokens != 0 {
		t.Fatalf("expected total_reasoning_tokens 0, got %d", summary.TotalReasoningTokens)
	}
	if summary.TotalTokens != 1500 {
		t.Fatalf("expected total_tokens 1500, got %d", summary.TotalTokens)
	}
	if summary.RecordCount != 1 {
		t.Fatalf("expected record_count 1, got %d", summary.RecordCount)
	}

	// Expected cost:
	// uncached input: 1000 - 200 = 800 => 800 * 3.0 / 1e6 = 0.0024
	// cached: 200 * 0.15 / 1e6 = 0.00003
	// output: 500 * 15.0 / 1e6 = 0.0075
	// total: 0.0024 + 0.00003 + 0.0075 = 0.00993
	expectedCost := 0.00993
	if summary.TotalCost != expectedCost {
		t.Fatalf("expected total_cost %f, got %f", expectedCost, summary.TotalCost)
	}

	if summary.Records[0].Model != "claude-3" {
		t.Fatalf("expected model claude-3, got %s", summary.Records[0].Model)
	}
	if summary.Records[0].Provider != "anthropic" {
		t.Fatalf("expected provider anthropic, got %s", summary.Records[0].Provider)
	}
}

func TestUsageRecorder_WithReasoningTokens(t *testing.T) {
	dir := t.TempDir()
	pricing := &model.Pricing{
		Input:    1.0,
		Output:   10.0,
		CacheHit: 0.1,
		Currency: "USD",
	}

	recorder := NewUsageRecorder("test-session-3", dir, pricing)

	usage := model.UsageInfo{
		PromptTokens:     500,
		CompletionTokens: 300,
		TotalTokens:      800,
		CompletionTokensDetails: map[string]int{
			"reasoning_tokens": 100,
		},
	}

	recorder.Record(usage, "deepseek-r1", "deepseek")

	summary := recorder.Summary()

	if summary.TotalReasoningTokens != 100 {
		t.Fatalf("expected total_reasoning_tokens 100, got %d", summary.TotalReasoningTokens)
	}
	if summary.Records[0].ReasoningTokens != 100 {
		t.Fatalf("expected record reasoning_tokens 100, got %d", summary.Records[0].ReasoningTokens)
	}
}

func TestUsageRecorder_Persistence(t *testing.T) {
	dir := t.TempDir()
	pricing := &model.Pricing{
		Input:    2.0,
		Output:   10.0,
		CacheHit: 0.1,
		Currency: "USD",
	}

	recorder := NewUsageRecorder("test-persist", dir, pricing)

	usage1 := model.UsageInfo{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
	}
	usage2 := model.UsageInfo{
		PromptTokens:     200,
		CompletionTokens: 100,
		TotalTokens:      300,
		PromptTokensDetails: map[string]int{
			"cached_tokens": 50,
		},
	}

	recorder.Record(usage1, "gpt-4", "openai")
	recorder.Record(usage2, "gpt-4o", "openai")

	// Verify the file was created
	usagePath := filepath.Join(dir, "sessions", "test-persist.usage.jsonl")
	if _, err := os.Stat(usagePath); os.IsNotExist(err) {
		t.Fatalf("usage.jsonl file was not created")
	}

	// Load usage from file and verify
	loaded, err := LoadUsage("test-persist", dir)
	if err != nil {
		t.Fatalf("LoadUsage failed: %v", err)
	}

	if loaded.SessionID != "test-persist" {
		t.Fatalf("expected sessionID test-persist, got %s", loaded.SessionID)
	}
	if loaded.TotalPromptTokens != 300 {
		t.Fatalf("expected total_prompt_tokens 300, got %d", loaded.TotalPromptTokens)
	}
	if loaded.TotalCompletionTokens != 150 {
		t.Fatalf("expected total_completion_tokens 150, got %d", loaded.TotalCompletionTokens)
	}
	if loaded.TotalCachedTokens != 50 {
		t.Fatalf("expected total_cached_tokens 50, got %d", loaded.TotalCachedTokens)
	}
	if loaded.TotalTokens != 450 {
		t.Fatalf("expected total_tokens 450, got %d", loaded.TotalTokens)
	}
	if loaded.RecordCount != 2 {
		t.Fatalf("expected record_count 2, got %d", loaded.RecordCount)
	}
	if len(loaded.Records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(loaded.Records))
	}
	if loaded.Records[0].Model != "gpt-4" {
		t.Fatalf("expected first record model gpt-4, got %s", loaded.Records[0].Model)
	}
	if loaded.Records[1].Model != "gpt-4o" {
		t.Fatalf("expected second record model gpt-4o, got %s", loaded.Records[1].Model)
	}
}

func TestUsageRecorder_EmptyState(t *testing.T) {
	dir := t.TempDir()

	// Load a non-existent session
	loaded, err := LoadUsage("non-existent", dir)
	if err != nil {
		t.Fatalf("LoadUsage for non-existent session should not error: %v", err)
	}
	if loaded.SessionID != "non-existent" {
		t.Fatalf("expected sessionID non-existent, got %s", loaded.SessionID)
	}
	if loaded.RecordCount != 0 {
		t.Fatalf("expected record_count 0 for empty session, got %d", loaded.RecordCount)
	}
	if loaded.TotalTokens != 0 {
		t.Fatalf("expected total_tokens 0 for empty session, got %d", loaded.TotalTokens)
	}
	if len(loaded.Records) != 0 {
		t.Fatalf("expected 0 records for empty session, got %d", len(loaded.Records))
	}
}

func TestUsageRecorder_SummaryWithNoPricing(t *testing.T) {
	dir := t.TempDir()
	recorder := NewUsageRecorder("test-nopricing", dir, nil)

	usage := model.UsageInfo{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
	}

	recorder.Record(usage, "gpt-4", "openai")

	summary := recorder.Summary()

	if summary.TotalCost != 0 {
		t.Fatalf("expected total_cost 0 when no pricing, got %f", summary.TotalCost)
	}
	if summary.Currency != "USD" {
		t.Fatalf("expected default currency USD, got %s", summary.Currency)
	}
}

func TestUsageRecorder_RecordWithCachedAndReasoning(t *testing.T) {
	dir := t.TempDir()
	pricing := &model.Pricing{
		Input:    5.0,
		Output:   20.0,
		CacheHit: 0.5,
		Currency: "USD",
	}

	recorder := NewUsageRecorder("test-cached-reasoning", dir, pricing)

	usage := model.UsageInfo{
		PromptTokens:     1000,
		CompletionTokens: 400,
		TotalTokens:      1400,
		PromptTokensDetails: map[string]int{
			"cached_tokens": 300,
		},
		CompletionTokensDetails: map[string]int{
			"reasoning_tokens": 150,
		},
	}

	recorder.Record(usage, "claude-opus", "anthropic")

	summary := recorder.Summary()

	if summary.TotalCachedTokens != 300 {
		t.Fatalf("expected total_cached_tokens 300, got %d", summary.TotalCachedTokens)
	}
	if summary.TotalReasoningTokens != 150 {
		t.Fatalf("expected total_reasoning_tokens 150, got %d", summary.TotalReasoningTokens)
	}
	if summary.TotalPromptTokens != 1000 {
		t.Fatalf("expected total_prompt_tokens 1000, got %d", summary.TotalPromptTokens)
	}
	if summary.TotalCompletionTokens != 400 {
		t.Fatalf("expected total_completion_tokens 400, got %d", summary.TotalCompletionTokens)
	}
	if summary.TotalTokens != 1400 {
		t.Fatalf("expected total_tokens 1400, got %d", summary.TotalTokens)
	}
	if summary.RecordCount != 1 {
		t.Fatalf("expected record_count 1, got %d", summary.RecordCount)
	}
	if summary.Records[0].CachedTokens != 300 {
		t.Fatalf("expected record cached_tokens 300, got %d", summary.Records[0].CachedTokens)
	}
	if summary.Records[0].ReasoningTokens != 150 {
		t.Fatalf("expected record reasoning_tokens 150, got %d", summary.Records[0].ReasoningTokens)
	}
}

func TestUsageRecorder_ClockOverride(t *testing.T) {
	dir := t.TempDir()
	recorder := NewUsageRecorder("test-clock", dir, nil)
	recorder.clock = fixedTime

	usage := model.UsageInfo{
		PromptTokens:     10,
		CompletionTokens: 5,
		TotalTokens:      15,
	}

	recorder.Record(usage, "gpt-4", "openai")

	summary := recorder.Summary()
	if !summary.Records[0].Timestamp.Equal(fixedTime()) {
		t.Fatalf("expected timestamp %v, got %v", fixedTime(), summary.Records[0].Timestamp)
	}
}

func TestLoadUsage_MalformedLine(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write a file with one valid JSON line and one malformed line
	content := `{"timestamp":"2026-01-15T10:30:00Z","model":"gpt-4","provider":"openai","prompt_tokens":100,"completion_tokens":50,"cached_tokens":0,"reasoning_tokens":0,"total_tokens":150,"cost":0,"currency":"USD"}
not-json
`
	path := filepath.Join(dir, "test-malformed.usage.jsonl")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// Load should skip the malformed line and still return the valid one
	loaded, err := LoadUsage("test-malformed", filepath.Dir(dir))
	if err != nil {
		t.Fatalf("LoadUsage should not error on malformed lines: %v", err)
	}
	if loaded.RecordCount != 1 {
		t.Fatalf("expected 1 valid record, got %d", loaded.RecordCount)
	}
	if loaded.TotalPromptTokens != 100 {
		t.Fatalf("expected total_prompt_tokens 100, got %d", loaded.TotalPromptTokens)
	}
}