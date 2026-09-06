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

package contextmgr

// StepRecord is the L0 original content record stored in {uuid}.jsonl.
// Each step represents one atomic unit of conversation (user message,
// tool call/result, reasoning block, etc.).
//
// "Original" is scoped to the ingest boundary: when the CLI tool_denoise
// flag is on, tool steps carry the mechanically denoised ingest text
// (ANSI/\r/duplicate-line cleanup); the untouched raw output remains in
// the session transcript and audit trail.
type StepRecord struct {
	StepID     int    `json:"step_id"`
	Type       string `json:"type"`                 // "user" | "tool" | "reasoning" | "plan" | "failed"
	Role       string `json:"role"`                 // "user" | "assistant" | "tool"
	Content    string `json:"content"`              // original content (tool steps: denoised when tool_denoise is on)
	TokenCount int    `json:"token_count"`          // token count of content
	ToolName   string `json:"tool_name,omitempty"`  // for tool steps
	KeyParams  string `json:"key_params,omitempty"` // summarized key params
	CreatedAt  int64  `json:"created_at,omitempty"` // unix timestamp
	// CallID pairs a tool-call step with its tool/result step (compression
	// pairing guard). Empty for non-tool steps.
	CallID string `json:"call_id,omitempty"`

	// Causal tracking metadata (B1)
	FilesRead     []string `json:"files_read,omitempty"`     // files read in this step
	FilesModified []string `json:"files_modified,omitempty"` // files modified in this step
	DependsOn     []int    `json:"depends_on,omitempty"`     // dependent step IDs
	TaskGroup     string   `json:"task_group,omitempty"`     // task group identifier

	// C-track Phase 1: transient step (tool call fragments, auto-excluded)
	Transient      bool   `json:"transient,omitempty"`
	TransientScope string `json:"transient_scope,omitempty"` // "tool_call" | "subtask" | "scratch"
	TransientRound int    `json:"transient_round,omitempty"` // creation round
}

// RefRecord tracks the current compression state and access statistics
// for each active step. Stored in {uuid}.refs.jsonl.
type RefRecord struct {
	StepID        int      `json:"step_id"`
	Level         int      `json:"level"`                   // current compression level (0-3)
	RefCount      int      `json:"ref_count"`               // cumulative §N citation count
	LastRefAtStep *int     `json:"last_ref_at_step"`        // last step that cited this one
	Strength      float64  `json:"strength"`                // accumulated access strength (init 1.0, +0.1/ref)
	TaskGroupID   int      `json:"task_group_id"`           // task group this step belongs to
	TaskBoundary  bool     `json:"task_boundary"`           // whether this is the group's starting step
	SemanticHold  bool     `json:"semantic_hold"`           // Redis semantic safety-net hold
	PendingL4     bool     `json:"pending_l4"`              // idle consolidation pre-mark for L4
	RelatedFiles  []string `json:"related_files,omitempty"` // associated files (active edit → file_mod=0.3)

	// CrossGroupRefs is the number of cross-group citations (A-5 跨组引用计数).
	// JSONL persists automatically; SQL backends currently leave it at zero
	// (column migration is deferred to a later wp).
	CrossGroupRefs int `json:"cross_group_refs"`
	// Heat is the heat/access-temperature signal (A-13, 0-100).
	// JSONL persists automatically; SQL backends currently leave it at zero
	// (column migration is deferred to a later wp). 0 acts as "no heat data".
	Heat int `json:"heat"`

	// LockL0 forces the step to stay at L0 (original), preventing any compression.
	// Only settable via context_lock_l0 tool. Audit trail is in {uuid}.audit.jsonl.
	LockL0 bool `json:"lock_l0"`
}

// AuditRecord is an append-only audit log entry stored in {uuid}.audit.jsonl.
// Every lock/unlock and other policy-sensitive operations are recorded here.
type AuditRecord struct {
	Timestamp int64  `json:"ts"`     // unix timestamp
	Action    string `json:"action"` // "lock_l0" | "unlock_l0" | ...
	StepID    int    `json:"step_id"`
	Reason    string `json:"reason,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

// L1Record is the simple compression (denoised summary) stored in {uuid}.l1.jsonl.
type L1Record struct {
	StepID           int    `json:"step_id"`
	Content          string `json:"content"`            // denoised/slimmed content
	TokenCount       int    `json:"token_count"`        // compressed token count
	CompressedAtStep int    `json:"compressed_at_step"` // step number when compression occurred
	CompactionID     string `json:"compaction_id,omitempty"` // owning compression transaction
}

// L2Record is the fact extraction stored in {uuid}.l2.jsonl.
type L2Record struct {
	StepID           int      `json:"step_id"`
	Facts            []string `json:"facts"`              // extracted key facts
	TokenCount       int      `json:"token_count"`        // compressed token count
	CompressedAtStep int      `json:"compressed_at_step"` // step number when compression occurred
	CompactionID     string   `json:"compaction_id,omitempty"` // owning compression transaction
}

// L3Record is the behavior mask stored in {uuid}.l3.jsonl.
type L3Record struct {
	StepID           int    `json:"step_id"`
	Mask             string `json:"mask"`               // one-line behavior mask
	TokenCount       int    `json:"token_count"`        // compressed token count
	CompressedAtStep int    `json:"compressed_at_step"` // step number when compression occurred
	CompactionID     string `json:"compaction_id,omitempty"` // owning compression transaction
}

// LongMemRecord is a long-term memory entry promoted from L2 facts.
// Stored in {uuid}.longmem.jsonl or PostgreSQL longterm_memories table.
type LongMemRecord struct {
	MemID             string   `json:"mem_id"`              // globally unique memory ID
	Facts             []string `json:"facts"`               // memory content (promoted from .l2.jsonl)
	SourceSteps       []int    `json:"source_steps"`        // traceable source steps
	SourceSessions    []string `json:"source_sessions"`     // source sessions (cross-session marker)
	Category          string   `json:"category"`            // "config" | "decision" | "constraint" | "pattern"
	CreatedAtStep     int      `json:"created_at_step"`     // step when promoted
	LastValidatedStep int      `json:"last_validated_step"` // last step that validated this memory
	Confidence        float64  `json:"confidence"`          // confidence (init 0.8, +0.04/ref, negated → 0)
}

// RenderedBlock is the output unit of BuildContext, representing one
// rendered segment ready for assembly into the context window.
type RenderedBlock struct {
	StepID  int    `json:"step_id"`
	Level   int    `json:"level"`   // compression level used for rendering
	Content string `json:"content"` // rendered text (with ⟨§N·type·Lx⟩ marker)
	// SourceStepIDs lists the original step IDs this block represents
	// (shadowing record for compressed blocks; normally the block's own
	// StepID). Mirrors the shadowedSeqs concept of harness compaction.
	SourceStepIDs []int `json:"source_step_ids,omitempty"`
}

// StepSessionLink maps a step_id to its position in Session.FullContext.
type StepSessionLink struct {
	StepID    int    `json:"step_id"`
	MsgIndex  int    `json:"msg_index"` // FullContext index
	SessionID string `json:"session_id"`
}

// CheckpointMeta is the header metadata for refs checkpoint copies.
type CheckpointMeta struct {
	IsCheckpoint bool   `json:"_checkpoint"` // always true, marks this as a copy
	AtStep       int    `json:"at_step"`     // current step at snapshot time
	HeaderVer    string `json:"header_ver"`  // head_buffer version at snapshot time
	CacheValid   bool   `json:"cache_valid"` // cache validity marker
	CompactionID string `json:"compaction_id,omitempty"` // owning compaction transaction
}

// IsCheckpointSource reports whether this checkpoint belongs to the given
// compaction transaction (backend-independent recognition).
func (c CheckpointMeta) IsCheckpointSource(compactionID string) bool {
	return c.IsCheckpoint && c.CompactionID != "" && c.CompactionID == compactionID
}
