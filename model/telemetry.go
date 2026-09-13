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

// Package model — telemetry contract types shared by session (rollout),
// orchestrator/agent (event producers) and server (trace persistence/API).
// Keeping them here avoids server↔session↔orchestrator dependency cycles.
package model

import "time"

// ContextBreakdown is the six-category token composition of one LLM request,
// mirroring dsh-context's per-request breakdown (system / tool schemas /
// user / inject / assistant / tool results). Counts are estimates
// (≈4 chars per token); Total is their sum.
type ContextBreakdown struct {
	System      int `json:"system"`
	ToolSchemas int `json:"tool_schemas"`
	User        int `json:"user"`
	Inject      int `json:"inject"`
	Assistant   int `json:"assistant"`
	ToolResults int `json:"tool_results"`
	Total       int `json:"total"`
}

// LLMRequestPayload annotates a rollout llm_request event: one outgoing LLM
// call with its context composition at send time.
type LLMRequestPayload struct {
	Turn      int               `json:"turn"`
	Round     int               `json:"round"`
	Model     string            `json:"model,omitempty"`
	Provider  string            `json:"provider,omitempty"`
	Breakdown *ContextBreakdown `json:"breakdown,omitempty"`
}

// LLMResponsePayload annotates a rollout llm_response event: the settled
// outcome of one LLM call. Cost fields are filled by consumers holding
// pricing (UsageRecorder); the engine leaves them zero.
type LLMResponsePayload struct {
	Turn       int        `json:"turn"`
	Round      int        `json:"round"`
	Model      string     `json:"model,omitempty"`
	Provider   string     `json:"provider,omitempty"`
	Usage      *UsageInfo `json:"usage,omitempty"`
	Cost       float64    `json:"cost,omitempty"`
	Currency   string     `json:"currency,omitempty"`
	DurationMs int64      `json:"duration_ms,omitempty"`
	TTFTMs     int64      `json:"ttft_ms,omitempty"` // time to first streamed chunk
	Error      string     `json:"error,omitempty"`
}

// Context event kinds for ContextEventPayload.
const (
	ContextEventCompaction  = "compaction"
	ContextEventPrune       = "prune"
	ContextEventInject      = "inject"
	ContextEventModelSwitch = "model_switch"
)

// ContextEventPayload annotates a rollout context_event: compression /
// injection / model-switch occurrences. CompactionID + StartStep/EndStep map
// the harness's compaction/start|summary|end trio onto one stable-identity
// event (CompactionID is shared by every record of the compaction run).
type ContextEventPayload struct {
	Kind            string    `json:"kind"`
	CompactionID    string    `json:"compaction_id,omitempty"`
	StartStep       int       `json:"start_step,omitempty"`
	EndStep         int       `json:"end_step,omitempty"`
	StepsCompressed int       `json:"steps_compressed,omitempty"`
	TokensSaved     int       `json:"tokens_saved,omitempty"`
	FromModel       string    `json:"from_model,omitempty"`
	ToModel         string    `json:"to_model,omitempty"`
	Timestamp       time.Time `json:"timestamp,omitempty"`
}

// FileOpPayload annotates a rollout file_op event: a file-side effect folded
// out of a tool call (op = read | write | search).
type FileOpPayload struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Bytes int    `json:"bytes,omitempty"`
}

// TurnStartPayload annotates a rollout turn_start event.
type TurnStartPayload struct {
	Turn    int    `json:"turn"`
	Message string `json:"message,omitempty"`
}

// TurnEndPayload annotates a rollout turn_end event.
type TurnEndPayload struct {
	Turn        int    `json:"turn"`
	Rounds      int    `json:"rounds,omitempty"`
	TotalTokens int    `json:"total_tokens,omitempty"`
	WallMs      int64  `json:"wall_ms,omitempty"`
	Reason      string `json:"reason,omitempty"` // final | error | preempt
}

// SpanRecord is one persisted span line of a run trace summary (the server's
// former runSpanRec, promoted to a shared contract). Round/Turn/Model/Usage
// are filled only for llm spans.
type SpanRecord struct {
	Kind       string     `json:"kind"` // agent | llm | tool
	Name       string     `json:"name"`
	DurationMs int64      `json:"duration_ms"`
	HasError   bool       `json:"error,omitempty"`
	Round      int        `json:"round,omitempty"`
	Turn       int        `json:"turn,omitempty"`
	Model      string     `json:"model,omitempty"`
	Usage      *UsageInfo `json:"usage,omitempty"`
}

// RequestUsageSummary is one per-request (per LLM round) usage line of a run
// trace summary — the "requests" array of RunTraceSummary. CachedTokens is
// filled by the usage.jsonl-backed API path (trace-embedded entries carry it
// only if the producing side stamped it into Usage details).
type RequestUsageSummary struct {
	Round        int        `json:"round"`
	Turn         int        `json:"turn,omitempty"`
	Model        string     `json:"model,omitempty"`
	Timestamp    string     `json:"timestamp"`
	DurationMs   int64      `json:"duration_ms,omitempty"`
	Tokens       int        `json:"tokens"`
	CachedTokens int        `json:"cached_tokens,omitempty"`
	Usage        *UsageInfo `json:"usage,omitempty"`
}

// TurnSummary aggregates one turn inside a run trace summary. The server
// stream path defines one run = one turn (a single user message), so
// turns[] currently carries a single aggregated entry per run; the rollout
// event stream (P2) carries the per-turn granularity.
type TurnSummary struct {
	Turn        int    `json:"turn"`
	Rounds      int    `json:"rounds,omitempty"`
	TotalTokens int    `json:"total_tokens,omitempty"`
	WallMs      int64  `json:"wall_ms,omitempty"`
	Start       string `json:"start,omitempty"`
	Error       string `json:"error,omitempty"`
}

// RunTraceSummary is the strongly typed content of a persisted run trace
// record (server MessageRoleTrace). Marshals to the same JSON shape the
// string-typed map produced, plus the additive schema/requests keys.
type RunTraceSummary struct {
	Schema     int                   `json:"schema,omitempty"` // 2 once requests[] present
	AgentID    string                `json:"agent_id"`
	Start      string                `json:"start"`
	Duration   string                `json:"duration"`
	DurationMs int64                 `json:"duration_ms,omitempty"`
	Spans      []SpanRecord          `json:"spans"`
	Requests   []RequestUsageSummary `json:"requests,omitempty"`
	Turns      []TurnSummary         `json:"turns,omitempty"`
	Usage      *UsageInfo            `json:"usage,omitempty"`
	Error      string                `json:"error"`
}
