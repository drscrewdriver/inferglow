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

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// Interfaces for dependency injection (avoid import cycles)
// ---------------------------------------------------------------------------

// SessionMessage is a minimal message representation for the summary manager.
// The orchestrator layer converts session.ChatMessage to this type.
type SessionMessage struct {
	Role    string
	Content string
}

// RewritableSession is the session interface needed by SummaryManager.
// The orchestrator's SessionExtension satisfies this.
type RewritableSession interface {
	// PreparePrompt returns the current context window as messages.
	PreparePrompt() []SessionMessage
	// Rewrite replaces the context window and returns the original.
	Rewrite(msgs []SessionMessage) []SessionMessage
}

// Summarizer generates a compaction summary from a fold region.
// The orchestrator wires this to the LLM provider.
type Summarizer interface {
	// Summarize generates a structured summary of the given messages.
	// systemPrompt is the SummarySystemPrompt; standingFacts are injected
	// from the memory store.
	Summarize(ctx context.Context, systemPrompt string, msgs []SessionMessage, standingFacts string) (string, error)
}

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

// SummaryConfig holds configuration for ModeSummary.
type SummaryConfig struct {
	// WindowTokens is the context window size in tokens.
	WindowTokens int
	// SoftRatio triggers a notice (default 0.5).
	SoftRatio float64
	// SnipRatio triggers tool result snipping (default 0.6).
	SnipRatio float64
	// CompactRatio triggers compaction (default 0.8).
	CompactRatio float64
	// ForceRatio forces compaction (default 0.9).
	ForceRatio float64
	// TailTokens is the verbatim recent-tail budget (default 16384).
	TailTokens int
	// MinFoldTokens is the minimum foldable tokens for economics (default 400).
	MinFoldTokens int
	// MinRecentKeep is the minimum recent messages to keep (default 2).
	MinRecentKeep int
	// ArchiveDir is the directory for archived messages.
	ArchiveDir string
	// StatePath is the path for compaction state persistence.
	StatePath string
}

// DefaultSummaryConfig returns sensible defaults matching Reasonix compact.go.
func DefaultSummaryConfig() SummaryConfig {
	return SummaryConfig{
		WindowTokens:  128000,
		SoftRatio:     0.5,
		SnipRatio:     0.6,
		CompactRatio:  0.8,
		ForceRatio:    0.9,
		TailTokens:    16384,
		MinFoldTokens: 400,
		MinRecentKeep: 2,
	}
}

// ---------------------------------------------------------------------------
// SummaryManager
// ---------------------------------------------------------------------------

// SummaryManager implements ModeSummary: session-level summary compaction
// modeled after Reasonix's compact.go. It is independent of the L0-L4
// tiered compression (ModeHybrid).
type SummaryManager struct {
	cfg       SummaryConfig
	store     StepStoreLike
	session   RewritableSession
	summarizer Summarizer
	stateStore *SummaryStateStore
	mu        sync.RWMutex

	// Runtime state (restored from stateStore on init).
	consecutiveCompacts int
	compactStuck        bool
	softNoticed         bool
	compactCount        int

	// standingFacts are injected from the memory store before summarization.
	standingFacts string
}

// NewSummaryManager creates a ModeSummary context manager.
func NewSummaryManager(cfg SummaryConfig, store StepStoreLike, session RewritableSession, summarizer Summarizer) (*SummaryManager, error) {
	if cfg.SoftRatio <= 0 {
		cfg.SoftRatio = 0.5
	}
	if cfg.SnipRatio <= 0 {
		cfg.SnipRatio = 0.6
	}
	if cfg.CompactRatio <= 0 {
		cfg.CompactRatio = 0.8
	}
	if cfg.ForceRatio <= 0 {
		cfg.ForceRatio = 0.9
	}
	if cfg.TailTokens <= 0 {
		cfg.TailTokens = 16384
	}
	if cfg.MinFoldTokens <= 0 {
		cfg.MinFoldTokens = 400
	}
	if cfg.MinRecentKeep <= 0 {
		cfg.MinRecentKeep = 2
	}

	sm := &SummaryManager{
		cfg:        cfg,
		store:      store,
		session:    session,
		summarizer: summarizer,
	}

	// Restore persisted state.
	if cfg.StatePath != "" {
		sm.stateStore = NewSummaryStateStore(cfg.StatePath)
		state := sm.stateStore.Load()
		sm.consecutiveCompacts = state.ConsecutiveCompacts
		sm.compactStuck = state.CompactStuck
		sm.compactCount = state.CompactCount
	}

	return sm, nil
}

// Mode returns ModeSummary.
func (sm *SummaryManager) Mode() Mode { return ModeSummary }

// SetStandingFacts injects standing facts from the memory store.
// Called before Compact to include durable constraints in the summary.
func (sm *SummaryManager) SetStandingFacts(facts string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.standingFacts = facts
}

// ---------------------------------------------------------------------------
// ContextManager interface — Ingest / BuildContext
// ---------------------------------------------------------------------------

// Ingest stores a step into the underlying store (Write + Sync).
func (sm *SummaryManager) Ingest(step StepRecord) error {
	return sm.store.AppendStep(step)
}

// BuildContext returns the current session messages as rendered blocks.
// For ModeSummary, this is a passthrough — the session itself manages
// the context window; compaction rewrites it in place.
func (sm *SummaryManager) BuildContext(_ context.Context, _ int) ([]RenderedBlock, error) {
	if sm.session == nil {
		return nil, nil
	}
	msgs := sm.session.PreparePrompt()
	blocks := make([]RenderedBlock, 0, len(msgs))
	for i, m := range msgs {
		blocks = append(blocks, RenderedBlock{
			StepID:  i,
			Level:   0,
			Content: m.Content,
		})
	}
	return blocks, nil
}

// TriggerCompression manually triggers compaction.
func (sm *SummaryManager) TriggerCompression(ctx context.Context, opts CompressOpts) (*CompressResult, error) {
	err := sm.Compact(ctx, "manual", opts.Force)
	if err != nil {
		return nil, err
	}
	return &CompressResult{StepsCompressed: 1}, nil
}

// Search delegates to keyword search across steps.
func (sm *SummaryManager) Search(_ context.Context, query SearchQuery) ([]SearchHit, error) {
	ids, err := sm.store.AllActiveStepIDs()
	if err != nil {
		return nil, err
	}
	var hits []SearchHit
	q := strings.ToLower(query.Query)
	limit := query.Limit
	if limit <= 0 {
		limit = 5
	}
	for _, id := range ids {
		step, err := sm.store.GetStep(id)
		if err != nil {
			continue
		}
		if strings.Contains(strings.ToLower(step.Content), q) {
			snippet := step.Content
			if len(snippet) > 200 {
				snippet = snippet[:200] + "..."
			}
			hits = append(hits, SearchHit{StepID: id, Level: 0, Score: 1.0, Snippet: snippet, Type: step.Type})
			if len(hits) >= limit {
				break
			}
		}
	}
	return hits, nil
}

// SearchLongMem delegates to the store.
func (sm *SummaryManager) SearchLongMem(_ context.Context, query string, category string, limit int) ([]LongMemRecord, error) {
	return sm.store.SearchLongMem(query, category, limit)
}

// Expand retrieves original content for a step.
func (sm *SummaryManager) Expand(stepID int, full bool) (*ExpandResult, error) {
	step, err := sm.store.GetStep(stepID)
	if err != nil {
		return nil, err
	}
	return &ExpandResult{StepID: stepID, Level: 0, Content: step.Content, Tokens: step.TokenCount}, nil
}

// Surround retrieves context around a step.
func (sm *SummaryManager) Surround(stepID int, before, after int) ([]RenderedBlock, error) {
	steps, err := sm.store.RangeSteps(stepID-before, stepID+after)
	if err != nil {
		return nil, err
	}
	blocks := make([]RenderedBlock, 0, len(steps))
	for _, s := range steps {
		blocks = append(blocks, RenderedBlock{StepID: s.StepID, Level: 0, Content: s.Content})
	}
	return blocks, nil
}

// Stats returns context statistics.
func (sm *SummaryManager) Stats() ContextStats {
	ids, _ := sm.store.AllActiveStepIDs()
	totalTokens := 0
	for _, id := range ids {
		step, err := sm.store.GetStep(id)
		if err != nil {
			continue
		}
		totalTokens += step.TokenCount
	}
	return ContextStats{
		TotalSteps:  len(ids),
		ActiveSteps: len(ids),
		TotalTokens: totalTokens,
		LevelCounts: map[int]int{0: len(ids)},
	}
}

// Close releases resources.
func (sm *SummaryManager) Close() error { return nil }

// ---------------------------------------------------------------------------
// Core compaction logic (modeled after Reasonix compact.go)
// ---------------------------------------------------------------------------

// MaybeCompact checks whether compaction is needed based on prompt token
// usage relative to the configured context window. Call this after each
// LLM turn. windowTokens comes from SummaryConfig.WindowTokens.
func (sm *SummaryManager) MaybeCompact(ctx context.Context, promptTokens int) error {
	windowTokens := sm.cfg.WindowTokens
	if windowTokens <= 0 || promptTokens <= 0 {
		return nil
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	high := int(float64(windowTokens) * sm.cfg.CompactRatio)
	soft := int(float64(windowTokens) * sm.cfg.SoftRatio)

	// Soft notice: report growing context once without compacting.
	if promptTokens >= soft && promptTokens < high && !sm.softNoticed {
		sm.softNoticed = true
		return nil // caller can emit a notice event
	}

	if promptTokens < high {
		// Under threshold: reset stuck state.
		sm.consecutiveCompacts = 0
		sm.compactStuck = false
		return nil
	}

	if sm.compactStuck {
		return nil
	}

	force := promptTokens >= int(float64(windowTokens)*sm.cfg.ForceRatio)
	return sm.compactLocked(ctx, "auto", force)
}

// Compact performs compaction: summarize older messages and rewrite session.
func (sm *SummaryManager) Compact(ctx context.Context, trigger string, force bool) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.compactLocked(ctx, trigger, force)
}

// compactLocked is the internal compaction implementation. Caller holds sm.mu.
func (sm *SummaryManager) compactLocked(ctx context.Context, trigger string, force bool) error {
	if sm.session == nil {
		return fmt.Errorf("summary: no session configured")
	}
	if sm.summarizer == nil {
		return fmt.Errorf("summary: no summarizer configured")
	}

	msgs := sm.session.PreparePrompt()
	if len(msgs) < sm.cfg.MinRecentKeep+1 {
		return nil // not enough messages to compact
	}

	// Plan compaction: find the split point.
	head, start := sm.planCompaction(msgs)
	if start <= head {
		return nil // nothing to fold
	}

	// Partition: keep user messages verbatim, fold the rest.
	kept, fold := sm.partitionFold(msgs[head:start])
	if len(fold) == 0 {
		return nil
	}

	// Economics check: skip if fold is too small.
	if !force && !sm.foldEconomics(fold) {
		return nil
	}

	// Archive dropped messages.
	archived := ""
	if sm.cfg.ArchiveDir != "" {
		archMsgs := make([]ArchivedMessage, len(fold))
		for i, m := range fold {
			archMsgs[i] = ArchivedMessage{
				Role:      m.Role,
				Content:   m.Content,
				Timestamp: time.Now(),
			}
		}
		path, err := ArchiveMessages(sm.cfg.ArchiveDir, archMsgs)
		if err != nil {
			return fmt.Errorf("summary: archive: %w", err)
		}
		archived = path
	}

	// Generate summary via LLM.
	summary, err := sm.summarizer.Summarize(ctx, SummarySystemPrompt, fold, sm.standingFacts)
	if err != nil {
		// Mechanical fold: use a deterministic marker.
		summary = fmt.Sprintf("[compacted %d messages mechanically; summary unavailable: %v]", len(fold), err)
	}

	// Build compacted message list:
	// head (system etc.) + kept (user turns) + summary + tail (recent)
	compacted := make([]SessionMessage, 0, head+len(kept)+1+len(msgs)-start)
	compacted = append(compacted, msgs[:head]...)
	compacted = append(compacted, kept...)
	compacted = append(compacted, SessionMessage{
		Role: "user",
		Content: SummaryTagOpen + "\n" +
			SummaryPrefix +
			summary + "\n" +
			SummaryTagClose,
	})
	compacted = append(compacted, msgs[start:]...)

	// Rewrite session.
	sm.session.Rewrite(compacted)

	// Update state.
	sm.compactCount++
	sm.consecutiveCompacts++
	if sm.consecutiveCompacts >= 2 {
		sm.compactStuck = true
	}

	// Persist state.
	if sm.stateStore != nil {
		_ = sm.stateStore.Save(SummaryState{
			LastCompactAt:       time.Now().Unix(),
			CompactCount:        sm.compactCount,
			ConsecutiveCompacts: sm.consecutiveCompacts,
			CompactStuck:        sm.compactStuck,
			LastArchivePath:     archived,
		})
	}

	return nil
}

// planCompaction determines the head (protected prefix) and start (fold
// boundary) indices. The system message and recent tail are protected.
func (sm *SummaryManager) planCompaction(msgs []SessionMessage) (head, start int) {
	// Protect system message(s) at the start.
	head = 0
	for head < len(msgs) && msgs[head].Role == "system" {
		head++
	}

	// Protect recent tail: keep at least MinRecentKeep messages + tail budget.
	tailStart := len(msgs) - sm.cfg.MinRecentKeep
	if tailStart < head {
		tailStart = head
	}

	// Also protect by token budget: keep recent messages within TailTokens.
	tailTokens := 0
	for i := len(msgs) - 1; i >= head; i-- {
		tailTokens += estimateTokensSimple(msgs[i].Content)
		if tailTokens > sm.cfg.TailTokens {
			tailStart = i + 1
			break
		}
		tailStart = i
	}

	if tailStart < head {
		tailStart = head
	}

	return head, tailStart
}

// partitionFold separates user messages (kept verbatim) from the rest
// (folded into the summary). User turns are never summarized away.
func (sm *SummaryManager) partitionFold(region []SessionMessage) (kept, fold []SessionMessage) {
	for _, m := range region {
		if m.Role == "user" {
			kept = append(kept, m)
		} else {
			fold = append(fold, m)
		}
	}
	return kept, fold
}

// foldEconomics checks whether the foldable region is large enough to
// justify the summarization API call.
func (sm *SummaryManager) foldEconomics(fold []SessionMessage) bool {
	total := 0
	for _, m := range fold {
		total += estimateTokensSimple(m.Content)
	}
	return total >= sm.cfg.MinFoldTokens
}

// estimateTokensSimple provides a rough token count (4 chars ~ 1 token).
func estimateTokensSimple(s string) int {
	n := len(s) / 4
	if n < 1 {
		return 1
	}
	return n
}
