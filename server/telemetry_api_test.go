// Copyright 2026 InferGlow Authors

package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/inferglow/server/config"
)

// telemetryTestServer builds a server backed by the fake OpenAI stream LLM
// with a usage data dir configured.
func telemetryTestServer(t *testing.T, withUsage bool) (*Server, *httptest.Server) {
	t.Helper()
	llmSrv := fakeOpenAIStreamLLMWithUsage([]string{"你好"}, withUsage)
	t.Cleanup(llmSrv.Close)

	store, err := NewConfigAgentStore(config.MultiLLMConfig{
		Providers: map[string]config.LLMConfig{
			"fake": {Provider: "openai", BaseURL: llmSrv.URL, Model: "mock-1", APIKey: "test-key"},
		},
	}, nil)
	if err != nil {
		t.Fatalf("config agent store: %v", err)
	}
	srv := NewServer(DefaultConfig(), store)
	srv.SetMessageStore(NewMessageStore())
	srv.cfg.UsageDataDir = t.TempDir()
	return srv, llmSrv
}

// runStream executes one stream-run and returns the SSE body.
func runStream(t *testing.T, srv *Server, sessionID string) string {
	t.Helper()
	req := httptest.NewRequest("POST", "/v1/agents/fake/stream-run",
		strings.NewReader(fmt.Sprintf(`{"message":"hi","session_id":%q}`, sessionID)))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	return w.Body.String()
}

func getJSON(t *testing.T, srv *Server, path string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return w.Code, body
}

// TestTraceAPIStructuredAndFilters covers the P3 trace endpoint: structured
// summaries (schema:2, requests, turns), the detail=false light mode and the
// from_turn/to_turn filters, plus the SSE turn/llm_request telemetry events.
func TestTraceAPIStructuredAndFilters(t *testing.T) {
	srv, _ := telemetryTestServer(t, true)
	body := runStream(t, srv, "trace-api")

	// SSE telemetry events (task 20): additive kinds + model on llm_end.
	for _, want := range []string{
		"event: turn_start", "event: llm_request", "event: turn_end",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("SSE body missing %s:\n%s", want, body)
		}
	}
	if !strings.Contains(body, `"model":"mock-1"`) {
		t.Errorf("llm_end must carry the model name:\n%s", body)
	}

	code, resp := getJSON(t, srv, "/v1/sessions/trace-api/trace")
	if code != http.StatusOK {
		t.Fatalf("GET /trace = %d", code)
	}
	traces := resp["traces"].([]any)
	if len(traces) != 1 {
		t.Fatalf("expected 1 trace, got %d", len(traces))
	}
	entry := traces[0].(map[string]any)
	summary, ok := entry["summary"].(map[string]any)
	if !ok {
		t.Fatalf("structured summary missing: %v", entry)
	}
	if summary["schema"].(float64) != 2 {
		t.Errorf("summary schema = %v; want 2", summary["schema"])
	}
	requests, _ := summary["requests"].([]any)
	if len(requests) != 1 {
		t.Errorf("summary requests = %v; want 1 entry", summary["requests"])
	}
	turns, _ := summary["turns"].([]any)
	if len(turns) != 1 {
		t.Errorf("summary turns = %v; want 1 entry", summary["turns"])
	}
	// Raw content string stays available for old clients.
	if entry["content"] == nil || entry["content"] == "" {
		t.Error("raw content must remain in the response")
	}

	// detail=false strips spans/requests.
	_, resp = getJSON(t, srv, "/v1/sessions/trace-api/trace?detail=false")
	summary = resp["traces"].([]any)[0].(map[string]any)["summary"].(map[string]any)
	if _, has := summary["requests"]; has {
		t.Errorf("detail=false must strip requests, got %v", summary)
	}

	// from_turn=2 excludes the single-turn run; from_turn=1 keeps it.
	_, resp = getJSON(t, srv, "/v1/sessions/trace-api/trace?from_turn=2")
	if len(resp["traces"].([]any)) != 0 {
		t.Error("from_turn=2 must exclude the run")
	}
	_, resp = getJSON(t, srv, "/v1/sessions/trace-api/trace?from_turn=1&to_turn=1")
	if len(resp["traces"].([]any)) != 1 {
		t.Error("from_turn=1&to_turn=1 must keep the run")
	}
}

// TestRequestsEndpoint covers GET /v1/sessions/{id}/requests — usage.jsonl as
// the primary source (round/model/tokens from the P1 wiring), with a fresh
// server instance verifying the data survives a restart.
func TestRequestsEndpoint(t *testing.T) {
	srv, _ := telemetryTestServer(t, true)
	runStream(t, srv, "req-api")

	code, resp := getJSON(t, srv, "/v1/sessions/req-api/requests")
	if code != http.StatusOK {
		t.Fatalf("GET /requests = %d", code)
	}
	requests := resp["requests"].([]any)
	if len(requests) != 1 {
		t.Fatalf("expected 1 request record, got %v", resp["requests"])
	}
	rec := requests[0].(map[string]any)
	if rec["model"] != "mock-1" {
		t.Errorf("request model = %v; want mock-1", rec["model"])
	}
	if rec["round"].(float64) != 0 {
		t.Errorf("request round = %v; want 0", rec["round"])
	}

	// Restart simulation: a fresh server over the same data dir reads the
	// same records from disk.
	srv2 := NewServer(DefaultConfig(), nil)
	srv2.cfg.UsageDataDir = srv.cfg.UsageDataDir
	code, resp = getJSON(t, srv2, "/v1/sessions/req-api/requests")
	if code != http.StatusOK || len(resp["requests"].([]any)) != 1 {
		t.Fatalf("restart: requests not readable from disk: %d %v", code, resp)
	}
}

// TestEventsEndpoint covers GET /v1/sessions/{id}/events — the rollout event
// stream with kinds filter (empty when no recorder is attached).
func TestEventsEndpoint(t *testing.T) {
	srv, _ := telemetryTestServer(t, false)
	code, resp := getJSON(t, srv, "/v1/sessions/ev-api/events?kinds=turn_start,llm_request")
	if code != http.StatusOK {
		t.Fatalf("GET /events = %d", code)
	}
	if events, ok := resp["events"].([]any); !ok || len(events) != 0 {
		t.Errorf("no recorder attached → empty events, got %v", resp["events"])
	}
}

// TestTraceNotLeakedIntoChat asserts trace-role records stay excluded from
// the chat history listing while old trace snapshots (no schema/requests
// keys) still parse into a spans-only summary.
func TestTraceNotLeakedIntoChat(t *testing.T) {
	srv, _ := telemetryTestServer(t, false)
	// Old-format trace: spans only, no schema/requests/turns keys.
	legacy := `{"agent_id":"fake","start":"2026-01-15T10:30:00Z","duration":"1s",` +
		`"spans":[{"kind":"llm","name":"inferglow.llm.call.0","duration_ms":42}],"error":""}`
	srv.recordMessage("legacy-sess", MessageRoleUser, "hello", "", "")
	srv.recordMessage("legacy-sess", MessageRoleTrace, legacy, "", "")

	code, resp := getJSON(t, srv, "/v1/sessions/legacy-sess/messages")
	if code != http.StatusOK {
		t.Fatalf("GET /messages = %d", code)
	}
	for _, m := range resp["messages"].([]any) {
		if m.(map[string]any)["role"] == string(MessageRoleTrace) {
			t.Fatalf("trace record leaked into chat history: %v", m)
		}
	}

	code, resp = getJSON(t, srv, "/v1/sessions/legacy-sess/trace")
	if code != http.StatusOK {
		t.Fatalf("GET /trace = %d for legacy snapshot", code)
	}
	entry := resp["traces"].([]any)[0].(map[string]any)
	summary := entry["summary"].(map[string]any)
	if _, has := summary["requests"]; has {
		t.Errorf("legacy summary must not fabricate requests: %v", summary)
	}
	spans, _ := summary["spans"].([]any)
	if len(spans) != 1 || summary["schema"] != nil {
		t.Errorf("legacy summary spans = %v schema = %v", spans, summary["schema"])
	}
}

// TestUsageReportPerSession covers the per-session /usage/report extension:
// by-turn and peak/off-peak billing buckets.
func TestUsageReportPerSession(t *testing.T) {
	srv, _ := telemetryTestServer(t, false)
	writeUsageRecord(t, srv.cfg.UsageDataDir, "usage-rep", "mock-1", timeNowLocal(9, 0), 100, 5)
	writeUsageRecord(t, srv.cfg.UsageDataDir, "usage-rep", "mock-1", timeNowLocal(2, 0), 50, 3)

	code, resp := getJSON(t, srv, "/v1/usage/report?session_id=usage-rep")
	if code != http.StatusOK {
		t.Fatalf("GET /usage/report = %d", code)
	}
	if resp["total_tokens"].(float64) != 150 {
		t.Errorf("total_tokens = %v; want 150", resp["total_tokens"])
	}
	byTurn := resp["by_turn"].(map[string]any)
	if byTurn["0"].(map[string]any)["count"].(float64) != 2 {
		t.Errorf("by_turn = %v; want turn 0 with 2 records", byTurn)
	}
	buckets := resp["by_billing_bucket"].(map[string]any)
	if buckets["peak"].(map[string]any)["tokens"].(float64) != 100 {
		t.Errorf("peak bucket = %v", buckets)
	}
	if buckets["off_peak"].(map[string]any)["tokens"].(float64) != 50 {
		t.Errorf("off_peak bucket = %v", buckets)
	}
}

// timeNowLocal builds a time at the given local hour/minute today.
func timeNowLocal(hour, minute int) time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
}
