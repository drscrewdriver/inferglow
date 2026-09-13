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
	"net/http"
	"strconv"
	"time"

	"github.com/inferglow/model"
	"github.com/inferglow/session"
)

// handleGetSessionRequests handles GET /v1/sessions/{id}/requests — the
// per-request (per LLM round) usage records backing the Context Trend chart
// and the per-turn cost/token columns.
//
// Data source: the session's usage.jsonl (per-request records written since
// P1, turn/round dimensions included). When no usage file exists, falls back
// to the requests[] arrays embedded in persisted trace summaries, so old
// sessions still serve data.
//
// Query params:
//   - turn:  filter to one turn number
//   - limit: max records returned (default 200)
func (s *Server) handleGetSessionRequests(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	turnFilter := -1
	if raw := r.URL.Query().Get("turn"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			turnFilter = n
		} else {
			writeError(w, http.StatusBadRequest, "invalid turn")
			return
		}
	}
	limit := 200
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = min(n, 1000)
		}
	}

	requests := s.sessionRequests(id, turnFilter, limit)
	writeJSON(w, http.StatusOK, map[string]any{
		"session_id": id,
		"requests":   requests,
		"count":      len(requests),
	})
}

// sessionRequests collects per-request usage records for one session, newest
// last: usage.jsonl records first, trace-embedded requests[] as fallback.
func (s *Server) sessionRequests(sessionID string, turnFilter, limit int) []model.RequestUsageSummary {
	// Primary source: usage.jsonl (written per LLM round, survives restarts).
	if s.cfg.UsageDataDir != "" {
		if stats, err := session.LoadUsage(sessionID, s.cfg.UsageDataDir); err == nil && len(stats.Records) > 0 {
			out := make([]model.RequestUsageSummary, 0, len(stats.Records))
			for _, rec := range stats.Records {
				if turnFilter >= 0 && rec.Turn != turnFilter {
					continue
				}
				out = append(out, model.RequestUsageSummary{
					Round:        rec.Round,
					Turn:         rec.Turn,
					Model:        rec.Model,
					Timestamp:    rec.Timestamp.UTC().Format(time.RFC3339),
					Tokens:       rec.TotalTokens,
					CachedTokens: rec.CachedTokens,
					Usage: &model.UsageInfo{
						PromptTokens:     rec.PromptTokens,
						CompletionTokens: rec.CompletionTokens,
						TotalTokens:      rec.TotalTokens,
					},
				})
			}
			if len(out) > limit {
				out = out[len(out)-limit:]
			}
			return out
		}
	}

	// Fallback: requests[] embedded in the persisted trace summaries.
	if s.msgStore == nil {
		return nil
	}
	var out []model.RequestUsageSummary
	for _, trace := range s.msgStore.ListTraces(sessionID, 100) {
		var summary model.RunTraceSummary
		if err := json.Unmarshal([]byte(trace.Content), &summary); err != nil {
			continue
		}
		for _, req := range summary.Requests {
			if turnFilter >= 0 && req.Turn != turnFilter {
				continue
			}
			out = append(out, req)
		}
	}
	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}

// handleGetSessionEvents handles GET /v1/sessions/{id}/events — the session's
// rollout event stream (turn_start/turn_end, llm_request/llm_response,
// context_event, file_op, ...). Data source: the rollout JSONL written by the
// engine when a recorder is attached (P2).
//
// Query params:
//   - kinds: comma-separated RolloutItemType filter (absent = all)
//   - limit: max items returned (default 500, newest last)
func (s *Server) handleGetSessionEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.cfg.UsageDataDir == "" {
		writeJSON(w, http.StatusOK, map[string]any{"session_id": id, "events": []any{}})
		return
	}

	kinds := map[string]bool{}
	if raw := r.URL.Query().Get("kinds"); raw != "" {
		for _, k := range splitComma(raw) {
			if k != "" {
				kinds[k] = true
			}
		}
	}
	limit := 500
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = min(n, 5000)
		}
	}

	recorder := session.NewRolloutRecorder(s.cfg.UsageDataDir, id)
	items, err := recorder.List(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read rollout events: "+err.Error())
		return
	}
	out := make([]session.RolloutItem, 0, len(items))
	for _, item := range items {
		if len(kinds) > 0 && !kinds[string(item.Type)] {
			continue
		}
		out = append(out, item)
	}
	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"session_id": id,
		"events":     out,
		"count":      len(out),
	})
}

func splitComma(s string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return out
}

// offPeakEndHour is the local-time boundary of the off-peak (谷) billing
// window [00:00, 08:30); everything else counts as peak (峰).
const offPeakEndHour, offPeakEndMin = 8, 30

// sessionUsageReport aggregates one session's usage.jsonl into per-turn and
// peak/off-peak buckets (P3 extension of GET /v1/usage/report?session_id=).
func (s *Server) sessionUsageReport(sessionID string) map[string]any {
	stats, err := session.LoadUsage(sessionID, s.cfg.UsageDataDir)
	if err != nil {
		return map[string]any{"session_id": sessionID, "error": err.Error()}
	}

	type bucket struct {
		Tokens int     `json:"tokens"`
		Cost   float64 `json:"cost"`
		Count  int     `json:"count"`
	}
	byTurn := map[string]*bucket{}
	byBucket := map[string]*bucket{"peak": {}, "off_peak": {}}
	for _, rec := range stats.Records {
		tb := byTurn[strconv.Itoa(rec.Turn)]
		if tb == nil {
			tb = &bucket{}
			byTurn[strconv.Itoa(rec.Turn)] = tb
		}
		tb.Tokens += rec.TotalTokens
		tb.Cost += rec.Cost
		tb.Count++

		bb := byBucket["peak"]
		if h, m := rec.Timestamp.Local().Hour(), rec.Timestamp.Local().Minute(); h < offPeakEndHour || (h == offPeakEndHour && m < offPeakEndMin) {
			bb = byBucket["off_peak"]
		}
		bb.Tokens += rec.TotalTokens
		bb.Cost += rec.Cost
		bb.Count++
	}

	return map[string]any{
		"session_id":           sessionID,
		"total_tokens":         stats.TotalTokens,
		"total_cost":           stats.TotalCost,
		"currency":             stats.Currency,
		"record_count":         stats.RecordCount,
		"by_turn":              byTurn,
		"by_billing_bucket":    byBucket,
		"billing_bucket_model": "off_peak = local [00:00, 08:30); peak = rest",
	}
}
