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
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/inferglow/model"
)

// CacheReportSummary is the per-model or aggregate cache efficiency summary.
type CacheReportSummary struct {
	Model             string  `json:"model,omitempty"`
	TotalPromptTokens int     `json:"total_prompt_tokens"`
	TotalCachedTokens int     `json:"total_cached_tokens"`
	CacheHitRate      float64 `json:"cache_hit_rate"` // 0.0 - 1.0
	TotalCost         float64 `json:"total_cost"`
	ActualCost        float64 `json:"actual_cost"` // with cache pricing
	Savings           float64 `json:"savings"`     // cost saved due to cache
	Currency          string  `json:"currency"`
	SessionCount      int     `json:"session_count"`
}

// CacheReport is the full cache efficiency report.
type CacheReport struct {
	GeneratedAt time.Time             `json:"generated_at"`
	From        time.Time             `json:"from"`
	To          time.Time             `json:"to"`
	Overall     CacheReportSummary    `json:"overall"`
	ByModel     []CacheReportSummary  `json:"by_model,omitempty"`
}

// ReportGenerator generates cache efficiency reports.
type ReportGenerator struct {
	dataDir string
	clock   func() time.Time
}

// NewReportGenerator creates a new report generator.
func NewReportGenerator(dataDir string) *ReportGenerator {
	return &ReportGenerator{
		dataDir: dataDir,
		clock:   time.Now,
	}
}

// Generate generates a cache report for the given time range.
// It scans all session usage.jsonl files in the sessions directory.
// If model is specified, filters to that model only.
func (g *ReportGenerator) Generate(ctx context.Context, from, to time.Time, model string) (*CacheReport, error) {
	records, err := g.loadAllRecords(ctx)
	if err != nil {
		return nil, fmt.Errorf("load records: %w", err)
	}

	return g.generateFromRecords(records, from, to, model), nil
}

// loadAllRecords loads all usage records from all usage.jsonl files in the sessions directory.
func (g *ReportGenerator) loadAllRecords(ctx context.Context) ([]UsageRecord, error) {
	sessionsDir := filepath.Join(g.dataDir, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read sessions dir: %w", err)
	}

	var records []UsageRecord
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !isUsageFile(entry.Name()) {
			continue
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		path := filepath.Join(sessionsDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue // skip unreadable files
		}

		for _, line := range splitLines(string(data)) {
			if line == "" {
				continue
			}
			var record UsageRecord
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				continue // skip malformed lines
			}
			records = append(records, record)
		}
	}

	return records, nil
}

// generateFromRecords takes a slice of UsageRecords and produces a CacheReport.
func (g *ReportGenerator) generateFromRecords(records []UsageRecord, from, to time.Time, modelFilter string) *CacheReport {
	// Filter by time range and model
	var filtered []UsageRecord
	for _, r := range records {
		if r.Timestamp.Before(from) || r.Timestamp.After(to) {
			continue
		}
		if modelFilter != "" && r.Model != modelFilter {
			continue
		}
		filtered = append(filtered, r)
	}

	report := &CacheReport{
		GeneratedAt: g.clock(),
		From:        from,
		To:          to,
	}

	if len(filtered) == 0 {
		report.Overall = CacheReportSummary{
			Currency: "USD",
		}
		return report
	}

	// Group by model
	modelGroups := make(map[string][]UsageRecord)
	for _, r := range filtered {
		modelGroups[r.Model] = append(modelGroups[r.Model], r)
	}

	// Build per-model summaries
	var modelSummaries []CacheReportSummary
	for modelName, modelRecords := range modelGroups {
		summary := g.buildSummary(modelName, modelRecords)
		modelSummaries = append(modelSummaries, summary)
	}

	// Sort by model name for deterministic output
	sort.Slice(modelSummaries, func(i, j int) bool {
		return modelSummaries[i].Model < modelSummaries[j].Model
	})

	report.ByModel = modelSummaries

	// Build overall summary
	overall := g.buildOverallSummary(modelSummaries)
	report.Overall = overall

	return report
}

// isUsageFile checks if a file name matches the usage.jsonl pattern.
func isUsageFile(name string) bool {
	ext := filepath.Ext(name)
	if ext != ".jsonl" {
		return false
	}
	// Must end with .usage.jsonl
	return len(name) > 12 && name[len(name)-12:] == ".usage.jsonl"
}

// buildSummary creates a CacheReportSummary for a set of records sharing the same model.
func (g *ReportGenerator) buildSummary(modelName string, records []UsageRecord) CacheReportSummary {
	summary := CacheReportSummary{
		Model: modelName,
	}

	var totalPromptTokens, totalCachedTokens int
	var actualCost float64
	currency := "USD"

	for _, r := range records {
		totalPromptTokens += r.PromptTokens
		totalCachedTokens += r.CachedTokens
		actualCost += r.Cost
		if r.Currency != "" {
			currency = r.Currency
		}
	}

	summary.TotalPromptTokens = totalPromptTokens
	summary.TotalCachedTokens = totalCachedTokens
	summary.ActualCost = actualCost
	summary.Currency = currency

	// Calculate cache hit rate
	if totalPromptTokens > 0 {
		summary.CacheHitRate = float64(totalCachedTokens) / float64(totalPromptTokens)
	}

	// Calculate total cost (without cache) and savings
	pricing, ok := model.LookupPricing(modelName)
	if ok {
		// Without cache: all prompt tokens at Input price, plus output tokens at Output price
		var totalOutputTokens int
		for _, r := range records {
			totalOutputTokens += r.CompletionTokens
		}

		// TotalCost = what cost would be without cache pricing
		const perMillion = 1e6
		totalCostWithoutCache := float64(totalPromptTokens)*pricing.Input/perMillion +
			float64(totalOutputTokens)*pricing.Output/perMillion

		summary.TotalCost = totalCostWithoutCache

		// Savings = cost without cache - actual cost
		savings := totalCostWithoutCache - actualCost
		if savings < 0 {
			savings = 0
		}
		summary.Savings = savings
	} else {
		// No pricing info available: use actual cost as total cost, savings = 0
		summary.TotalCost = actualCost
		summary.Savings = 0
	}

	summary.SessionCount = 1 // per-model group, each model appears in at least one session

	return summary
}

// buildOverallSummary aggregates per-model summaries into an overall summary.
func (g *ReportGenerator) buildOverallSummary(modelSummaries []CacheReportSummary) CacheReportSummary {
	overall := CacheReportSummary{
		Currency: "USD",
	}

	var totalPromptTokens, totalCachedTokens int
	var totalCost, actualCost float64
	currency := "USD"

	for _, s := range modelSummaries {
		totalPromptTokens += s.TotalPromptTokens
		totalCachedTokens += s.TotalCachedTokens
		totalCost += s.TotalCost
		actualCost += s.ActualCost
		if s.Currency != "" {
			currency = s.Currency
		}
	}

	overall.TotalPromptTokens = totalPromptTokens
	overall.TotalCachedTokens = totalCachedTokens
	overall.TotalCost = totalCost
	overall.ActualCost = actualCost
	overall.Currency = currency
	overall.SessionCount = len(modelSummaries)

	if totalCost > 0 {
		overall.Savings = totalCost - actualCost
		if overall.Savings < 0 {
			overall.Savings = 0
		}
	}

	if totalPromptTokens > 0 {
		overall.CacheHitRate = float64(totalCachedTokens) / float64(totalPromptTokens)
	}

	return overall
}