// Copyright 2026 InferGlow Authors

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/inferglow/orchestrator/agent"
	"github.com/inferglow/server/config"
)

// fakeOpenAIStreamLLM answers any /chat/completions request with a fixed
// OpenAI-compatible SSE stream (same wire format model/openai_bugfix_test.go
// fixtures use), letting the real provider+engine path be exercised without a
// network LLM.
func fakeOpenAIStreamLLM(chunks []string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, c := range chunks {
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q},\"finish_reason\":null}]}\n\n", c)
		}
		w.Write([]byte("data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n"))
		w.Write([]byte("data: [DONE]\n"))
	}))
}

// TestStreamRunRealAgentDeltaEndToEnd drives the true streaming path:
// config agent store → *agent.Agent (real engine) → stream-run SSE with
// delta events → completion-time persistence.
func TestStreamRunRealAgentDeltaEndToEnd(t *testing.T) {
	chunks := []string{"你好", "，", "世界"}
	llmSrv := fakeOpenAIStreamLLM(chunks)
	defer llmSrv.Close()

	store, err := NewConfigAgentStore(config.MultiLLMConfig{
		Providers: map[string]config.LLMConfig{
			"fake": {Provider: "openai", BaseURL: llmSrv.URL, Model: "mock-1", APIKey: "test-key"},
		},
	})
	if err != nil {
		t.Fatalf("config agent store: %v", err)
	}
	ag := store.Get("fake")
	if ag == nil {
		t.Fatalf("agent fake missing")
	}
	if _, ok := ag.(CallbacksRunner); !ok {
		t.Fatalf("ConfigAgent must satisfy CallbacksRunner")
	}
	// Identity must serialize (contrast: demo agent marshals as {}).
	idJSON, err := json.Marshal(ag)
	if err != nil || !strings.Contains(string(idJSON), `"id":"fake"`) || !strings.Contains(string(idJSON), `"model":"mock-1"`) {
		t.Fatalf("agent json = %s, err=%v", idJSON, err)
	}

	srv := NewServer(DefaultConfig(), store)
	srv.SetMessageStore(NewMessageStore())

	req := httptest.NewRequest("POST", "/v1/agents/fake/stream-run", strings.NewReader(`{"message":"hi","session_id":"sess-e2e"}`))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "event: run_start") {
		t.Fatalf("missing run_start:\n%s", body)
	}
	// Every chunk must arrive as its own delta event, in order, before run_end.
	if deltas := strings.Count(body, "event: delta"); deltas != len(chunks) {
		t.Fatalf("want %d delta events, got %d:\n%s", len(chunks), deltas, body)
	}
	last := -1
	for _, c := range chunks {
		marker := fmt.Sprintf("\"delta\":%q", c)
		idx := strings.Index(body, marker)
		if idx < 0 {
			t.Fatalf("missing delta %q:\n%s", c, body)
		}
		if idx < last {
			t.Fatalf("delta %q out of order:\n%s", c, body)
		}
		last = idx
	}
	if !strings.Contains(body, "event: run_end") {
		t.Fatalf("missing run_end:\n%s", body)
	}
	if !strings.Contains(body, "event: done") {
		t.Fatalf("missing done:\n%s", body)
	}

	// Completion-time persistence: user + assistant records exist even though
	// persistence no longer rides the client write loop.
	recs, _ := srv.msgStore.ListBefore("sess-e2e", time.Time{}, 10)
	var roles []string
	var assistant string
	for _, m := range recs {
		roles = append(roles, string(m.Role))
		if m.Role == MessageRoleAssistant {
			assistant = m.Content
		}
	}
	full := strings.Join(chunks, "")
	if assistant != full {
		t.Fatalf("assistant record = %q, want %q", assistant, full)
	}
	if len(roles) != 2 {
		t.Fatalf("want 2 persisted records (user+assistant), got %v", roles)
	}
}

// TestMergeCallbacksChainsBase asserts mergeCallbacks never drops the agent's
// construction-time hooks while adding the stream bridge.
func TestMergeCallbacksChainsBase(t *testing.T) {
	baseCalled := false
	base := &agent.AgentCallbacks{
		OnToken: func(_ context.Context, _ string) { baseCalled = true },
	}
	stream := &agent.AgentCallbacks{}
	merged := mergeCallbacks(stream, base)
	merged.OnToken(context.Background(), "x")
	if !baseCalled {
		t.Fatalf("base OnToken was replaced instead of chained")
	}
	passthrough := mergeCallbacks(&agent.AgentCallbacks{}, base)
	if passthrough.OnToken == nil {
		t.Fatalf("base-only OnToken lost")
	}
	if mergeCallbacks(nil, base) != base {
		t.Fatalf("nil stream handling broken")
	}
	if mergeCallbacks(stream, nil) != stream {
		t.Fatalf("nil base handling broken")
	}
}
