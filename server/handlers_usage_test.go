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

package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/inferglow/session"
)

// writeUsageRecord appends one usage.jsonl line for a session under dataDir.
func writeUsageRecord(t *testing.T, dataDir, sessionID, model string, ts time.Time, tokens, cost int) {
	t.Helper()
	dir := filepath.Join(dataDir, "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	rec := session.UsageRecord{
		Timestamp:        ts,
		Model:            model,
		Provider:         "deepseek",
		PromptTokens:     60,
		CompletionTokens: tokens - 60,
		CachedTokens:     20,
		TotalTokens:      tokens,
		Cost:             float64(cost),
		Currency:         "USD",
	}
	line, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, sessionID+".usage.jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		t.Fatal(err)
	}
}

// TestUsageReport_Aggregation verifies GET /v1/usage/report aggregates usage
// records written by the session module's jsonl layout.
func TestUsageReport_Aggregation(t *testing.T) {
	dataDir := t.TempDir()
	now := time.Now()
	writeUsageRecord(t, dataDir, "s1", "deepseek-chat", now.Add(-time.Hour), 100, 1)
	writeUsageRecord(t, dataDir, "s1", "deepseek-chat", now.Add(-30*time.Minute), 200, 2)
	writeUsageRecord(t, dataDir, "s2", "deepseek-reasoner", now.Add(-time.Minute), 300, 3)

	cfg := DefaultConfig()
	cfg.UsageDataDir = dataDir
	srv := NewServer(cfg, newMockStore())

	req := httptest.NewRequest("GET", "/v1/usage/report", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", w.Code, w.Body.String())
	}
	var report session.CacheReport
	if err := json.NewDecoder(w.Body).Decode(&report); err != nil {
		t.Fatal(err)
	}
	if report.Overall.TotalPromptTokens != 180 {
		t.Fatalf("TotalPromptTokens = %d, want 180", report.Overall.TotalPromptTokens)
	}
	if report.Overall.SessionCount != 2 {
		t.Fatalf("SessionCount = %d, want 2", report.Overall.SessionCount)
	}
	if len(report.ByModel) != 2 {
		t.Fatalf("ByModel len = %d, want 2", len(report.ByModel))
	}
}

// TestUsageReport_ModelFilter verifies the ?model= query narrows the report.
func TestUsageReport_ModelFilter(t *testing.T) {
	dataDir := t.TempDir()
	now := time.Now()
	writeUsageRecord(t, dataDir, "s1", "deepseek-chat", now.Add(-time.Hour), 100, 1)
	writeUsageRecord(t, dataDir, "s1", "deepseek-reasoner", now.Add(-time.Minute), 400, 4)

	cfg := DefaultConfig()
	cfg.UsageDataDir = dataDir
	srv := NewServer(cfg, newMockStore())

	req := httptest.NewRequest("GET", "/v1/usage/report?model=deepseek-chat", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var report session.CacheReport
	if err := json.NewDecoder(w.Body).Decode(&report); err != nil {
		t.Fatal(err)
	}
	if report.Overall.TotalPromptTokens != 60 {
		t.Fatalf("TotalPromptTokens = %d, want 60 (model filtered)", report.Overall.TotalPromptTokens)
	}
}

// TestUsageReport_EmptyDataDir verifies a missing data dir yields an empty
// report instead of an error.
func TestUsageReport_EmptyDataDir(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore())

	req := httptest.NewRequest("GET", "/v1/usage/report", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", w.Code, w.Body.String())
	}
	var report session.CacheReport
	if err := json.NewDecoder(w.Body).Decode(&report); err != nil {
		t.Fatal(err)
	}
	if report.Overall.TotalPromptTokens != 0 {
		t.Fatalf("TotalPromptTokens = %d, want 0", report.Overall.TotalPromptTokens)
	}
}

// TestUsageReport_InvalidRange verifies malformed range params yield 400.
func TestUsageReport_InvalidRange(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore())

	for _, q := range []string{"from=not-a-time", "to=not-a-time"} {
		req := httptest.NewRequest("GET", "/v1/usage/report?"+q, nil)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s: want 400, got %d", q, w.Code)
		}
	}
}

// TestUsageReport_OutOfRange verifies records outside the default month window
// are excluded.
func TestUsageReport_OutOfRange(t *testing.T) {
	dataDir := t.TempDir()
	// One record two months ago — outside the default current-month window.
	writeUsageRecord(t, dataDir, "s1", "deepseek-chat", time.Now().AddDate(0, -2, 0), 999, 9)

	cfg := DefaultConfig()
	cfg.UsageDataDir = dataDir
	srv := NewServer(cfg, newMockStore())

	req := httptest.NewRequest("GET", "/v1/usage/report", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var report session.CacheReport
	if err := json.NewDecoder(w.Body).Decode(&report); err != nil {
		t.Fatal(err)
	}
	if report.Overall.TotalPromptTokens != 0 {
		t.Fatalf("TotalPromptTokens = %d, want 0 (out of default month window)", report.Overall.TotalPromptTokens)
	}

	// Explicit wide range includes it. UTC format avoids '+' in the query
	// string being decoded as a space by net/http.
	from := time.Now().AddDate(0, -3, 0).UTC().Format(time.RFC3339)
	to := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	req = httptest.NewRequest("GET", fmt.Sprintf("/v1/usage/report?from=%s&to=%s", from, to), nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if err := json.NewDecoder(w.Body).Decode(&report); err != nil {
		t.Fatal(err)
	}
	if report.Overall.TotalPromptTokens != 60 {
		t.Fatalf("TotalPromptTokens = %d, want 60 (explicit range)", report.Overall.TotalPromptTokens)
	}
}
