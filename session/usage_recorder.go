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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/inferglow/model"
)

// UsageRecorder records and aggregates per-call usage stats.
type UsageRecorder struct {
	mu        sync.Mutex
	sessionID string
	dataDir   string
	stats     SessionUsageStats
	pricing   *model.Pricing // for cost calculation, can be nil
	clock     func() time.Time
	// file is the persistent handle to sessions/<id>.usage.jsonl, opened
	// lazily on first Record and reused (O_APPEND) — replacing the previous
	// open/write/close per record. Close releases it.
	file *os.File
}

// NewUsageRecorder creates a UsageRecorder for the given session.
func NewUsageRecorder(sessionID, dataDir string, pricing *model.Pricing) *UsageRecorder {
	return &UsageRecorder{
		sessionID: sessionID,
		dataDir:   dataDir,
		stats: SessionUsageStats{
			SessionID: sessionID,
			Currency:  "USD",
		},
		pricing: pricing,
		clock:   time.Now,
	}
}

// Record records a single usage event.
func (r *UsageRecorder) Record(usage model.UsageInfo, modelName, provider string) {
	r.RecordAt(usage, modelName, provider, 0, 0, 0)
}

// RecordAt records a single usage event located inside the run: turn is the
// user-message-scoped turn number, round the engine LLM-call index, and
// sessionSeq the rollout seq the call answered (all optional; zero = unset).
func (r *UsageRecorder) RecordAt(usage model.UsageInfo, modelName, provider string, turn, round int, sessionSeq int64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	cachedTokens := 0
	if usage.PromptTokensDetails != nil {
		cachedTokens = usage.PromptTokensDetails["cached_tokens"]
	}

	reasoningTokens := 0
	if usage.CompletionTokensDetails != nil {
		reasoningTokens = usage.CompletionTokensDetails["reasoning_tokens"]
	}

	currency := "USD"
	cost := 0.0
	if r.pricing != nil {
		cost = r.pricing.Cost(&usage)
		currency = r.pricing.Currency
	}

	record := UsageRecord{
		Timestamp:        r.clock(),
		Model:            modelName,
		Provider:         provider,
		Turn:             turn,
		Round:            round,
		SessionSeq:       sessionSeq,
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		CachedTokens:     cachedTokens,
		ReasoningTokens:  reasoningTokens,
		TotalTokens:      usage.TotalTokens,
		Cost:             cost,
		Currency:         currency,
	}

	// Update aggregation
	r.stats.TotalPromptTokens += usage.PromptTokens
	r.stats.TotalCompletionTokens += usage.CompletionTokens
	r.stats.TotalCachedTokens += cachedTokens
	r.stats.TotalReasoningTokens += reasoningTokens
	r.stats.TotalTokens += usage.TotalTokens
	r.stats.TotalCost += cost
	r.stats.Currency = currency
	r.stats.RecordCount++
	r.stats.Records = append(r.stats.Records, record)

	// Persist to jsonl
	if err := r.appendRecord(record); err != nil {
		// Non-fatal: logging or recording failure should not interrupt the caller.
		// In production, a structured logger would be used here.
		_ = err
	}
}

// Summary returns a copy of the current aggregated stats.
func (r *UsageRecorder) Summary() SessionUsageStats {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stats
}

// Flush syncs the persistent usage file handle to disk. Safe to call
// concurrently with Record and repeatedly; a no-op when nothing was recorded.
func (r *UsageRecorder) Flush() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		return nil
	}
	return r.file.Sync()
}

// Close flushes and releases the persistent usage file handle. The recorder
// must not be used for Record afterwards.
func (r *UsageRecorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		return nil
	}
	err := r.file.Close()
	r.file = nil
	return err
}

// appendRecord writes a single UsageRecord as a JSON line to the usage.jsonl
// file via the persistent O_APPEND handle (opened lazily on first record).
func (r *UsageRecorder) appendRecord(record UsageRecord) error {
	if r.file == nil {
		dir := filepath.Join(r.dataDir, "sessions")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create sessions dir: %w", err)
		}
		f, err := os.OpenFile(filepath.Join(dir, r.sessionID+".usage.jsonl"),
			os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return fmt.Errorf("open usage file: %w", err)
		}
		r.file = f
	}

	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal usage record: %w", err)
	}

	if _, err := r.file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write usage record: %w", err)
	}

	return nil
}

// LoadUsage loads usage records from usage.jsonl for a given session.
func LoadUsage(sessionID, dataDir string) (*SessionUsageStats, error) {
	path := filepath.Join(dataDir, "sessions", sessionID+".usage.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &SessionUsageStats{
				SessionID: sessionID,
				Currency:  "USD",
			}, nil
		}
		return nil, fmt.Errorf("read usage file: %w", err)
	}

	stats := SessionUsageStats{
		SessionID: sessionID,
		Currency:  "USD",
	}

	for _, line := range splitLines(string(data)) {
		if line == "" {
			continue
		}
		var record UsageRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			continue
		}
		stats.TotalPromptTokens += record.PromptTokens
		stats.TotalCompletionTokens += record.CompletionTokens
		stats.TotalCachedTokens += record.CachedTokens
		stats.TotalReasoningTokens += record.ReasoningTokens
		stats.TotalTokens += record.TotalTokens
		stats.TotalCost += record.Cost
		stats.Currency = record.Currency
		stats.RecordCount++
		stats.Records = append(stats.Records, record)
	}

	return &stats, nil
}

// splitLines splits text into lines, handling both \n and \r\n.
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			line := s[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			lines = append(lines, line)
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
