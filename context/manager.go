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

// Package contextmgr implements the context management system for InferGlow.
//
// It provides a pluggable, multi-mode context manager that supports:
//   - passthrough: no compression, direct pass-through to session
//   - three_zone: adapter for existing ThreeZoneSession
//   - hybrid: full compression with L0-L4 levels, RAG, and long-term memory
//
// The context manager operates independently from the session layer,
// communicating via dual-write (Ingest) and context injection (BuildContext).
package contextmgr

import (
	"context"
)

// Mode represents the context management mode.
type Mode string

const (
	// ModePassthrough passes context through without compression.
	ModePassthrough Mode = "passthrough"
	// ModeThreeZone uses the three-zone session adapter.
	ModeThreeZone Mode = "three_zone"
	// ModeSummary uses session-level summary compaction (对标 Reasonix compact.go).
	// When prompt tokens exceed a threshold, older messages are summarized
	// by an LLM and replaced with a structured compaction summary.
	ModeSummary Mode = "summary"
	// ModeHybrid uses full compression with L0-L4 levels.
	ModeHybrid Mode = "hybrid"
	// ModeAssembly is the 9-layer context assembly engine (线A / C轨).
	// It wires the Retrieval/Render/Decay layers via a new Manager + Registry
	// mechanism; see docs/plans/00-integrated-master-spec.md 线A.
	ModeAssembly Mode = "assembly"
)

// ContextManager is the main interface for context management.
// Different modes implement this interface with varying strategies.
type ContextManager interface {
	// Mode returns the current operating mode.
	Mode() Mode

	// Ingest processes a new step into the context manager.
	// This is called after each agent step (user message, tool result, etc.).
	Ingest(step StepRecord) error

	// BuildContext assembles the context window for the next LLM call.
	// It returns rendered blocks ready for assembly into the prompt.
	BuildContext(ctx context.Context, windowTokens int) ([]RenderedBlock, error)

	// TriggerCompression manually triggers compression with the given options.
	TriggerCompression(ctx context.Context, opts CompressOpts) (*CompressResult, error)

	// Search performs a semantic search across context history.
	Search(ctx context.Context, query SearchQuery) ([]SearchHit, error)

	// SearchLongMem searches long-term memory.
	SearchLongMem(ctx context.Context, query string, category string, limit int) ([]LongMemRecord, error)

	// Expand retrieves content for a step.
	// When full is true, returns L0 (original); when false, returns L1 (denoised) if available.
	Expand(stepID int, full bool) (*ExpandResult, error)

	// Surround retrieves context around a step.
	Surround(stepID int, before, after int) ([]RenderedBlock, error)

	// Stats returns current context statistics.
	Stats() ContextStats

	// Close releases resources.
	Close() error
}

// CompressOpts holds options for manual compression trigger.
type CompressOpts struct {
	// Force forces compression even if below threshold.
	Force bool
	// TargetLevel is the target compression level (1-3).
	TargetLevel int
	// TaskGroupID limits compression to a specific task group (0 = all).
	TaskGroupID int
}

// CompressResult holds the result of a compression operation.
type CompressResult struct {
	// StepsCompressed is the number of steps that were compressed.
	StepsCompressed int
	// TokensSaved is the estimated token savings.
	TokensSaved int
	// NewLevels maps step IDs to their new compression levels.
	NewLevels map[int]int
}

// SearchQuery holds parameters for context search.
type SearchQuery struct {
	// Query is the search text.
	Query string
	// LevelMax limits results to steps at or below this compression level.
	LevelMax int
	// TaskGroup limits results to a specific task group.
	TaskGroup int
	// Limit is the maximum number of results.
	Limit int
}

// SearchHit is a single search result.
type SearchHit struct {
	// StepID is the step that matched.
	StepID int
	// Level is the current compression level.
	Level int
	// Score is the relevance score.
	Score float64
	// Snippet is a matching text fragment (≤200 chars).
	Snippet string
	// Type is the step type (user/tool/reasoning).
	Type string
}

// ExpandResult holds the result of an expand operation.
type ExpandResult struct {
	// StepID is the expanded step.
	StepID int
	// Level is the current compression level.
	Level int
	// Content is the expanded content.
	Content string
	// Tokens is the token count.
	Tokens int
	// Warning is an optional warning message (e.g., high token count).
	Warning string
}

// ContextStats holds context statistics.
type ContextStats struct {
	// TotalSteps is the total number of ingested steps.
	TotalSteps int
	// ActiveSteps is the number of steps in refs (not L4).
	ActiveSteps int
	// TotalTokens is the total token count across all active steps.
	TotalTokens int
	// CompressedTokens is the token count after compression.
	CompressedTokens int
	// LevelCounts maps compression levels to step counts.
	LevelCounts map[int]int
	// TaskGroups is the number of active task groups.
	TaskGroups int
	// HotFacts is the number of high-frequency facts (ref_count≥3).
	HotFacts int
	// LongMemCount is the number of long-term memories.
	LongMemCount int
	// WindowPressure is the current window pressure (0.0-1.0).
	WindowPressure float64
}
