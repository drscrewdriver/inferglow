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
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/inferglow/context/retrieval"
)

// HybridManager implements ModeHybrid with full L0-L4 compression.
type HybridManager struct {
	cfg   Config
	store StepStoreLike

	mu            sync.RWMutex
	currentStep   int32 // atomic counter for step IDs
	headBuffer    []RenderedBlock
	headBufferVer string
	taskGroupID   int32
	idleCount     int32

	// --- Phase 0: rendered-block cache (A-4) ---
	renderCache *RenderedCache

	// --- Phase 0: sweet-spot passthrough ---
	sweetSpotTokens   int     // effective value (may be adjusted by tolerance)
	sweetSpotOriginal int     // original config value (immutable after init)
	sweetSpotTolerance float64 // current multiplier (1.0 = original)
	toleranceMu       sync.Mutex

	// sliding window for citation tracking
	recentRefCounts []int
	recentRefIdx    int

	// --- Phase 0: warmup pre-compression ---
	warmupPending atomic.Bool

	// --- Phase 0: Zone 0.5 constitutional entries ---
	constitutionalEntries []string
	constitutionalMu      sync.RWMutex

	// --- Phase 0: head buffer archive ---
	archivedHeads []ArchivedHead
	headSeq       int // monotonic version counter for head buffer (A-3)

	// --- CM-4: semantic drift detection ---
	drift *DriftDetector

	// --- A-7: three-way fusion retrieval ---
	fusion       *retrieval.FusionRetriever
	vsIndex      *retrieval.VectorStore
	bm25Index    *retrieval.BM25Index
	recencyIndex *retrieval.RecencyIndex
	fusionMu     sync.Mutex // serializes reindex; guards fusionDirty
	fusionDirty  bool

	// --- compression lock (T2.3): one in-flight compression at a time ---
	compactionMu  sync.Mutex
	compactionRun *compactionRunInfo

	// reorgEngine is the LLM compression engine used by Reorganize when the
	// caller passes nil; attach via SetReorganizeEngine.
	reorgEngine CompressEngine
}

// compactionRunInfo tracks one in-flight compression transaction.
type compactionRunInfo struct {
	ID        string
	StartedAt time.Time
	Owner     string // "trigger" | "reorganize" | ...
}

// ArchivedHead is a previous head buffer kept for audit/rollback.
type ArchivedHead struct {
	Content    []RenderedBlock
	Version    string
	ArchivedAt time.Time
	Seq        int // monotonic version sequence (A-3)
}

// NewHybridManager creates a hybrid context manager.
func NewHybridManager(cfg Config, store StepStoreLike) (ContextManager, error) {
	h := &HybridManager{
		cfg:                cfg,
		store:              store,
		headBufferVer:      "h_v1",
		renderCache:        NewRenderedCache(),
		sweetSpotTokens:    cfg.SweetSpotTokens,
		sweetSpotOriginal:  cfg.SweetSpotTokens,
		sweetSpotTolerance: 1.0,
		recentRefCounts:    make([]int, 10), // sliding window of 10
		drift:              NewDriftDetector(cfg.DriftCheckInterval, cfg.DriftThreshold),
	}
	if cfg.WarmupRatio <= 0 {
		h.cfg.WarmupRatio = 0.8
	}
	if cfg.ToleranceDecayRate <= 0 {
		h.cfg.ToleranceDecayRate = 0.98
	}
	if cfg.Retrieval.EnableFusion {
		h.enableFusionRetrieval(cfg.Retrieval)
	}
	h.checkOrphanCompactions()
	return h, nil
}

// enableFusionRetrieval wires the three-way fusion retriever into the manager.
// The semantic path uses NoopEmbedder this wp (real embeddings arrive with
// OT-4); keyword + recency paths are fully active. Zero-valued weights/threshold
// fall back to the fusion defaults so a partially-specified config stays safe.
func (h *HybridManager) enableFusionRetrieval(rcfg RetrievalConfig) {
	if rcfg.Weights == [3]float64{0, 0, 0} {
		rcfg.Weights = [3]float64{0.50, 0.30, 0.20}
	}
	if rcfg.Threshold <= 0 {
		rcfg.Threshold = 0.35
	}
	vs := retrieval.NewVectorStore()
	bm25 := retrieval.NewBM25Index()
	recency := retrieval.NewRecencyIndexWithWeights(rcfg.RecencyW, rcfg.StrengthW)
	fusion := retrieval.NewFusionRetriever(vs, bm25, recency, &retrieval.NoopEmbedder{})
	fusion.Weights = rcfg.Weights
	fusion.Threshold = rcfg.Threshold
	h.vsIndex = vs
	h.bm25Index = bm25
	h.recencyIndex = recency
	h.fusion = fusion
	h.fusionDirty = true
}

func (h *HybridManager) Mode() Mode { return ModeHybrid }

// StepStore returns the underlying step store for direct access (e.g., causal tracing).
func (h *HybridManager) StepStore() StepStoreLike { return h.store }

// SetHeadBuffer sets the immutable head buffer (zone 1). Called once at session init.
func (h *HybridManager) SetHeadBuffer(blocks []RenderedBlock, version string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.headBuffer = blocks
	h.headBufferVer = version
}

// Ingest processes a new step: stores L0, creates ref, checks per-step decay.
func (h *HybridManager) Ingest(step StepRecord) error {
	sid := int(atomic.AddInt32(&h.currentStep, 1))
	step.StepID = sid

	// Store L0
	if err := h.store.AppendStep(step); err != nil {
		return fmt.Errorf("hybrid ingest: append step: %w", err)
	}

	// Create initial ref (level=0, strength=1.0)
	ref := RefRecord{
		StepID:      sid,
		Level:       0,
		RefCount:    0,
		Strength:    1.0,
		TaskGroupID: int(h.taskGroupID),
	}
	if err := h.store.UpsertRef(ref); err != nil {
		return fmt.Errorf("hybrid ingest: upsert ref: %w", err)
	}
	h.markFusionDirty()

	// Reset idle counter
	atomic.StoreInt32(&h.idleCount, 0)

	// Per-step decay check (layer 1 of 5)
	if err := h.perStepDecay(sid); err != nil {
		return err
	}

	// Warmup pre-compression: async pre-compress old steps when approaching sweet spot
	h.maybeWarmup()

	// CM-4: semantic drift detection
	if h.drift != nil && h.drift.MaybeCheck(int32(sid), step.Content, h.headKeywords()) {
		h.drift.SetHint()
	}

	// Decay tolerance back toward 1.0
	h.decayTolerance()

	return nil
}

// perStepDecay checks if the new step triggers compression for older steps.
// When sweet-spot is enabled and total tokens are below the threshold,
// decay is suppressed to maximise prefix cache hit rate.
func (h *HybridManager) perStepDecay(currentStep int) error {
	// Sweet-spot passthrough: skip decay if below threshold
	if h.sweetSpotTokens > 0 && h.estimateTotalTokens() < h.sweetSpotTokens {
		return nil
	}

	ids, err := h.store.AllActiveStepIDs()
	if err != nil {
		return err
	}

	// Compute raw_decay for each step: sum of tokens from last_ref+1 to current
	for _, id := range ids {
		if id == currentStep {
			continue
		}
		ref, err := h.store.GetRef(id)
		if err != nil || ref.Level >= 3 || ref.LockL0 {
			continue
		}

		// Compute raw_decay
		lastRef := currentStep - 1
		if ref.LastRefAtStep != nil {
			lastRef = *ref.LastRefAtStep
		}
		rawDecay := h.sumTokens(lastRef+1, currentStep)

		decay, trace := ComputeDecay(*ref, rawDecay, false, int(h.taskGroupID), h.cfg.Decay)
		target := TargetLevel(decay, h.stepType(id), h.cfg.Thresholds)
		trace.TargetLevel = target
		maxLvl := MaxLevelForType(h.stepType(id))
		if target > maxLvl {
			target = maxLvl
		}

		if target > ref.Level {
			// Level upgrade needed — actual compression is done by compress engine
			ref.Level = target
			if err := h.store.UpsertRef(*ref); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *HybridManager) stepType(stepID int) string {
	step, err := h.store.GetStep(stepID)
	if err != nil {
		return "reasoning"
	}
	return step.Type
}

func (h *HybridManager) sumTokens(from, to int) int {
	total := 0
	steps, err := h.store.RangeSteps(from, to)
	if err != nil {
		return 0
	}
	for _, s := range steps {
		total += s.TokenCount
	}
	return total
}

// SetReorganizeEngine attaches the LLM compression engine used by Reorganize
// when the caller passes a nil engine. Passing nil detaches the fallback.
func (h *HybridManager) SetReorganizeEngine(e CompressEngine) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.reorgEngine = e
}

// BuildContext assembles the 5-zone context window (§4.5).
func (h *HybridManager) BuildContext(ctx context.Context, windowTokens int) ([]RenderedBlock, error) {
	// T3.3: cheap mechanical reorganization as a pressure-triggered safety
	// net before assembly (never blocks on an LLM).
	if h.cfg.MechanicalPressureThreshold > 0 && h.windowPressure() > h.cfg.MechanicalPressureThreshold {
		if _, err := h.MechanicalReorganize(ctx, false); err != nil {
			log.Printf("contextmgr: BuildContext mechanical reorganize failed: %v", err)
		}
	}
	h.mu.RLock()
	defer h.mu.RUnlock()

	var blocks []RenderedBlock

	// Zone 0.5: constitutional entries (dynamic, before head buffer)
	h.constitutionalMu.RLock()
	if len(h.constitutionalEntries) > 0 {
		content := "<constitutional>\n"
		for _, e := range h.constitutionalEntries {
			content += "- " + e + "\n"
		}
		content += "</constitutional>"
		blocks = append(blocks, RenderedBlock{
			StepID:  -3, // constitutional pseudo-step
			Level:   0,
			Content: content,
		})
	}
	h.constitutionalMu.RUnlock()

	// Zone 1: head_buffer (never compressed)
	blocks = append(blocks, h.headBuffer...)

	// Zone 2: hot facts injection — ranked by strength descending (A-8 数值隔离)
	hotFacts, err := h.store.HotFacts(h.cfg.HotFacts.MinRefCount, h.cfg.HotFacts.MinStrength)
	if err == nil && len(hotFacts) > 0 {
		type rankedFact struct {
			L2Record
			strength float64
		}
		ranked := make([]rankedFact, 0, len(hotFacts))
		for _, f := range hotFacts {
			ref, refErr := h.store.GetRef(f.StepID)
			if refErr != nil {
				continue
			}
			ranked = append(ranked, rankedFact{L2Record: f, strength: ref.Strength})
		}
		sort.Slice(ranked, func(i, j int) bool { return ranked[i].strength > ranked[j].strength })
		if len(ranked) > 0 {
			content := "[facts | session | ranked by strength]\n"
			rank := 0
			for _, rf := range ranked {
				for _, fact := range rf.Facts {
					rank++
					content += fmt.Sprintf("  #%d (strength: %.1f) %s\n", rank, rf.strength, fact)
				}
			}
			content += "[/facts]"
			blocks = append(blocks, RenderedBlock{StepID: -4, Level: 2, Content: content})
		}
	}

	// Zone 2 extended: long-term memory facts — ranked by confidence descending (A-8)
	if h.cfg.LongMem.Enabled {
		longMems, err := h.store.SearchLongMem("", "", 20)
		if err == nil {
			// Filter and sort by confidence descending.
			filtered := make([]LongMemRecord, 0, len(longMems))
			for _, mem := range longMems {
				if mem.Confidence >= 0.7 {
					filtered = append(filtered, mem)
				}
			}
			sort.Slice(filtered, func(i, j int) bool { return filtered[i].Confidence > filtered[j].Confidence })
			if len(filtered) > 0 {
				content := "[facts | longterm | ranked by confidence]\n"
				rank := 0
				for _, mem := range filtered {
					for _, fact := range mem.Facts {
						rank++
						content += fmt.Sprintf("  #%d (confidence: %.2f) %s (跨 session 验证)\n", rank, mem.Confidence, fact)
					}
				}
				content += "[/facts]"
				blocks = append(blocks, RenderedBlock{StepID: -2, Level: 2, Content: content})
			}
		}
	}

	// Zone 3 + 4: compressed history + tail original
	allIDs, err := h.store.AllActiveStepIDs()
	if err != nil {
		return nil, err
	}

	tailStart := len(allIDs) - h.cfg.TailKeepSteps
	if tailStart < 0 {
		tailStart = 0
	}

	for i, id := range allIDs {
		step, err := h.store.GetStep(id)
		if err != nil {
			continue
		}
		// A-12: skip transient steps (tool call fragments)
		if step.Transient {
			continue
		}
		if i >= tailStart {
			// Zone 4: tail original (L0) — reuse step
			blocks = append(blocks, renderBlock(id, 0, step.Content, step.Type))
		} else {
			// Zone 3: compressed history（经 RenderStepWithCache 走缓存）
			ref, err := h.store.GetRef(id)
			if err != nil {
				continue
			}
			block, err := RenderStepWithCache(id, *ref, h.renderCache, h.store)
			if err != nil {
				continue
			}
			blocks = append(blocks, block)
		}
	}

	// Zone 4.5: same-group backtrack (A-9, Layer 8 injection)
	if h.cfg.Backtrack.Enabled {
		if b, ok := h.buildBacktrackBlock(allIDs); ok {
			blocks = append(blocks, b)
		}
	}

	// Zone 5: HintBlock (dynamic)
	hint := h.buildHintBlock(len(allIDs))
	blocks = append(blocks, hint)

	// Fit to window
	return h.fitToWindow(blocks, windowTokens), nil
}

// renderStepContent reads the appropriate .lN content for a step.
// It delegates to renderFromStore, the single canonical render implementation.
func (h *HybridManager) renderStepContent(stepID, level int) (string, error) {
	return renderFromStore(stepID, level, h.store)
}

// windowPressure estimates the current window pressure as the ratio of
// total active tokens to the configured window (0.0-1.0+).
func (h *HybridManager) windowPressure() float64 {
	window := h.cfg.WindowTokens
	if window <= 0 {
		return 0
	}
	total := h.totalTokens()
	if total <= 0 {
		return 0
	}
	return float64(total) / float64(window)
}

// renderBlock creates a RenderedBlock with the ⟨§N·type·Lx⟩ marker (§4.7).
func renderBlock(stepID, level int, content, stepType string) RenderedBlock {
	marker := fmt.Sprintf("⟨§%d·%s·L%d⟩", stepID, stepType, level)
	return RenderedBlock{
		StepID:        stepID,
		Level:         level,
		Content:       marker + " " + content,
		SourceStepIDs: []int{stepID},
	}
}

// buildHintBlock creates zone 5 dynamic hint.
func (h *HybridManager) buildHintBlock(activeSteps int) RenderedBlock {
	pressure := float64(activeSteps) / 100.0 // simplified
	if pressure > 1.0 {
		pressure = 1.0
	}
	content := fmt.Sprintf("[hint] 上下文压力: %.0f%% | 当前任务组: #%d | 活跃 steps: %d | tail: %d steps",
		pressure*100, h.taskGroupID, activeSteps, h.cfg.TailKeepSteps)
	// CM-4: append drift warning if active
	if h.drift != nil && h.drift.HintActive() {
		content += " | ⚠ 背景可能已过时，建议 /rebackground"
		h.drift.ClearHint()
	}
	return RenderedBlock{
		StepID:  -1,
		Level:   0,
		Content: content,
	}
}

// headKeywords extracts keywords from the current Zone 1 head buffer for drift detection.
func (h *HybridManager) headKeywords() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if len(h.headBuffer) == 0 {
		return nil
	}
	// Concatenate all head buffer content and extract top keywords.
	var sb strings.Builder
	for _, b := range h.headBuffer {
		sb.WriteString(b.Content)
		sb.WriteString(" ")
	}
	return ExtractKeywords(sb.String(), 30)
}

// fitToWindow trims blocks to fit within windowTokens by upgrading compression
// from the oldest steps first (zone 3 head).
func (h *HybridManager) fitToWindow(blocks []RenderedBlock, windowTokens int) []RenderedBlock {
	// Simple estimation: 1 token ≈ 4 chars
	totalTokens := 0
	for _, b := range blocks {
		totalTokens += len(b.Content) / 4
	}

	if totalTokens <= windowTokens {
		return blocks
	}

	// Try upgrading zone 3 blocks from head (real re-render at higher level).
	// Runs in a single O(N) pass, updating the running token total incrementally
	// (subtract old, add new) to avoid an O(N²) recount per block.
	for i := range blocks {
		b := &blocks[i]
		if b.StepID <= 0 || b.Level >= 3 {
			continue
		}
		oldTokens := len(b.Content) / 4

		// Real re-render at ref.Level+1 via the rendered cache (key: (stepID, level)).
		// The raised level is a temporary window pad only, NOT persisted — the next
		// BuildContext re-renders at the actual stored level.
		ref, err := h.store.GetRef(b.StepID)
		if err == nil {
			ref.Level = b.Level + 1
			if rendered, rerr := RenderStepWithCache(b.StepID, *ref, h.renderCache, h.store); rerr == nil {
				newTokens := len(rendered.Content) / 4
				totalTokens += newTokens - oldTokens
				*b = rendered
				if totalTokens <= windowTokens {
					break
				}
				continue
			}
		}

		// Fallback: truncate preserving the ⟨§N·type·Lx⟩ marker.
		keep := len(b.Content) / 2
		truncated := truncateRespectingMarker(b.Content, keep)
		totalTokens += len(truncated)/4 - oldTokens
		b.Content = truncated
		if totalTokens <= windowTokens {
			break
		}
	}

	return blocks
}

// truncateRespectingMarker truncates a rendered block's body while preserving the
// leading ⟨§N·type·Lx⟩ marker and any trailing structural closing tag
// (</constitutional> / [/facts]). It returns the original content unchanged when
// keep <= 0 or the body is already within budget.
func truncateRespectingMarker(content string, keep int) string {
	// Locate the marker prefix ⟨§N·type·Lx⟩ at the start.
	markerEnd := strings.Index(content, "⟩")
	if markerEnd < 0 {
		markerEnd = 0
	} else {
		markerEnd++ // include the closing ⟩
		// Skip the separator space following the marker.
		if markerEnd < len(content) && content[markerEnd] == ' ' {
			markerEnd++
		}
	}
	marker := content[:markerEnd]
	body := content[markerEnd:]

	// Detect trailing structural closing tags to re-add after truncation.
	suffix := ""
	lower := strings.ToLower(body)
	switch {
	case strings.HasSuffix(lower, "</constitutional>"):
		suffix = "</constitutional>"
		body = body[:len(body)-len("</constitutional>")]
	case strings.HasSuffix(lower, "[/facts]"):
		suffix = "[/facts]"
		body = body[:len(body)-len("[/facts]")]
	}

	// Only truncate when keep > 0 and the body exceeds the budget.
	if keep <= 0 || len(body) <= keep {
		return marker + body + suffix
	}

	return marker + body[:keep] + "...[compressed]" + suffix
}

// TriggerCompression performs batch compression (§6.4 8-step process).
// It is guarded by a compression lock: a concurrent second call fails with
// ErrCompressionBusy, and the run is bracketed by log-only compaction/start
// and compaction/end audit markers for crash/orphan detection.
func (h *HybridManager) TriggerCompression(ctx context.Context, opts CompressOpts) (*CompressResult, error) {
	if !h.tryAcquireCompactionLock("trigger") {
		return nil, ErrCompressionBusy
	}
	defer h.releaseCompactionLock()
	h.recordCompactionMarker("compaction/start")
	defer h.recordCompactionMarker("compaction/end")

	result := &CompressResult{
		NewLevels:    make(map[int]int),
		CompactionID: h.currentCompactionID(),
	}

	ids, err := h.store.AllActiveStepIDs()
	if err != nil {
		return nil, err
	}

	for _, id := range ids {
		ref, err := h.store.GetRef(id)
		if err != nil {
			continue
		}

		// Skip if already at max level for type, or locked at L0
		maxLvl := MaxLevelForType(h.stepType(id))
		if ref.Level >= maxLvl || ref.LockL0 {
			continue
		}

		// Compute decay
		rawDecay := h.sumTokens(0, int(atomic.LoadInt32(&h.currentStep)))
		decay, trace := ComputeDecay(*ref, rawDecay, false, int(h.taskGroupID), h.cfg.Decay)
		target := TargetLevel(decay, h.stepType(id), h.cfg.Thresholds)
		trace.TargetLevel = target
		if target > maxLvl {
			target = maxLvl
		}

		if target > ref.Level {
			ref.Level = target
			if err := h.store.UpsertRef(*ref); err != nil {
				continue
			}
			result.StepsCompressed++
			result.NewLevels[id] = target
			result.ShadowedStepIDs = append(result.ShadowedStepIDs, id)
		}
	}

	if len(result.ShadowedStepIDs) > 0 {
		result.ShadowedRange = ShadowRange{
			StartStep: result.ShadowedStepIDs[0],
			EndStep:   result.ShadowedStepIDs[len(result.ShadowedStepIDs)-1],
		}
	}

	return result, nil
}

// --- compression lock helpers ---

// tryAcquireCompactionLock takes the compression lock, creating a fresh
// transaction identity. It returns false when a run is already in flight.
func (h *HybridManager) tryAcquireCompactionLock(owner string) bool {
	h.compactionMu.Lock()
	defer h.compactionMu.Unlock()
	if h.compactionRun != nil {
		return false
	}
	h.compactionRun = &compactionRunInfo{
		ID:        newCompactionID(),
		StartedAt: time.Now(),
		Owner:     owner,
	}
	return true
}

// releaseCompactionLock releases the compression lock.
func (h *HybridManager) releaseCompactionLock() {
	h.compactionMu.Lock()
	defer h.compactionMu.Unlock()
	h.compactionRun = nil
}

// currentCompactionID returns the in-flight transaction ID, or "" when idle.
func (h *HybridManager) currentCompactionID() string {
	h.compactionMu.Lock()
	defer h.compactionMu.Unlock()
	if h.compactionRun == nil {
		return ""
	}
	return h.compactionRun.ID
}

// recordCompactionMarker appends a log-only compaction lifecycle marker to
// stores that support it (JSONL); other backends are skipped silently.
func (h *HybridManager) recordCompactionMarker(action string) {
	ms, ok := h.store.(interface {
		AppendCompactionMarker(action, compactionID string) error
	})
	if !ok {
		return
	}
	_ = ms.AppendCompactionMarker(action, h.currentCompactionID())
}

// checkOrphanCompactions warns about compaction/start markers without a
// matching compaction/end (crash-mid-compaction evidence) at startup.
func (h *HybridManager) checkOrphanCompactions() {
	os, ok := h.store.(interface{ OrphanCompactions() ([]string, error) })
	if !ok {
		return
	}
	orphans, err := os.OrphanCompactions()
	if err != nil {
		log.Printf("contextmgr: orphan compaction scan failed: %v", err)
		return
	}
	if len(orphans) > 0 {
		log.Printf("contextmgr: detected %d orphan compression lock(s): %v", len(orphans), orphans)
	}
}

// newCompactionID returns a fresh, collision-resistant compression
// transaction identity (contextmgr-local copy; compress has its own).
func newCompactionID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("compaction-%d", time.Now().UnixNano())
	}
	return "compaction-" + hex.EncodeToString(b[:])
}

// Search performs RAG search across context history.
func (h *HybridManager) Search(ctx context.Context, query SearchQuery) ([]SearchHit, error) {
	// A-7: three-way fusion path (gated). The legacy naive keyword search below
	// is retained as the 保壳 fallback when fusion is disabled or not built.
	if h.fusion != nil && h.cfg.Retrieval.EnableFusion {
		return h.fusionSearch(ctx, query)
	}

	// Simple keyword search (full RAG in retrieval/ package)
	ids, err := h.store.AllActiveStepIDs()
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
		ref, err := h.store.GetRef(id)
		if err != nil {
			continue
		}
		if query.LevelMax > 0 && ref.Level > query.LevelMax {
			continue
		}

		content, err := h.renderStepContent(id, ref.Level)
		if err != nil {
			continue
		}

		if strings.Contains(strings.ToLower(content), q) {
			snippet := content
			if len(snippet) > 200 {
				snippet = snippet[:200] + "..."
			}
			hits = append(hits, SearchHit{
				StepID:  id,
				Level:   ref.Level,
				Score:   1.0,
				Snippet: snippet,
				Type:    h.stepType(id),
			})
			if len(hits) >= limit {
				break
			}
		}
	}
	return hits, nil
}

// fusionSearch runs the three-way fusion retriever, lazily rebuilding the
// indexes from the store when dirty, and maps results onto SearchHit.
func (h *HybridManager) fusionSearch(ctx context.Context, query SearchQuery) ([]SearchHit, error) {
	h.fusionMu.Lock()
	if h.fusionDirty {
		h.reindex(ctx)
		h.fusionDirty = false
	}
	h.fusionMu.Unlock()

	limit := query.Limit
	if limit <= 0 {
		limit = 5
	}
	results, err := h.fusion.Search(ctx, query.Query, limit)
	if err != nil {
		return nil, err
	}

	hits := make([]SearchHit, 0, len(results))
	for _, r := range results {
		level := 0
		if ref, rerr := h.store.GetRef(r.StepID); rerr == nil {
			level = ref.Level
			if query.LevelMax > 0 && level > query.LevelMax {
				continue
			}
		}
		hits = append(hits, SearchHit{
			StepID:  r.StepID,
			Level:   level,
			Score:   r.Score,
			Snippet: r.Text,
			Type:    h.stepType(r.StepID),
		})
	}
	return hits, nil
}

// reindex rebuilds the keyword and recency indexes from the store (idempotent:
// indexes are reset first). The semantic index stays empty this wp (NoopEmbedder;
// real embeddings arrive with OT-4). Caller must hold h.fusionMu.
func (h *HybridManager) reindex(ctx context.Context) {
	ids, err := h.store.AllActiveStepIDs()
	if err != nil {
		return
	}
	h.bm25Index.Reset()
	h.recencyIndex.Reset()
	for _, id := range ids {
		ref, rerr := h.store.GetRef(id)
		if rerr != nil {
			continue
		}
		// A-12: skip transient steps (tool call fragments) from the indexes, matching
		// the BuildContext filter.
		step, serr := h.store.GetStep(id)
		if serr != nil {
			continue
		}
		if step.Transient {
			continue
		}
		content, cerr := h.renderStepContent(id, ref.Level)
		if cerr != nil {
			continue
		}
		lastRef := 0
		if ref.LastRefAtStep != nil {
			lastRef = *ref.LastRefAtStep
		}
		h.bm25Index.Add(id, content)
		h.recencyIndex.Add(id, lastRef, ref.Strength, content)
	}
}

// markFusionDirty flags the fusion indexes stale after a corpus mutation.
func (h *HybridManager) markFusionDirty() {
	if h.fusion == nil {
		return
	}
	h.fusionMu.Lock()
	h.fusionDirty = true
	h.fusionMu.Unlock()
}

// SearchLongMem searches long-term memory.
func (h *HybridManager) SearchLongMem(ctx context.Context, query string, category string, limit int) ([]LongMemRecord, error) {
	return h.store.SearchLongMem(query, category, limit)
}

// Expand retrieves content for a step (§8B.3).
// Default (full=false) returns L1 (denoised); full=true returns L0 (the
// ingest text — mechanically denoised for tool steps when the CLI
// tool_denoise flag is on).
// Side effect: updates refs (same as §N citation) — affects decay to slow L1→L2→L3.
func (h *HybridManager) Expand(stepID int, full bool) (*ExpandResult, error) {
	// Side effect: update refs (same as §N citation)
	ref, err := h.store.GetRef(stepID)
	if err == nil {
		ref.RefCount++
		ref.Strength += 0.1
		cur := int(atomic.LoadInt32(&h.currentStep))
		ref.LastRefAtStep = &cur
		_ = h.store.UpsertRef(*ref)
	}

	if full {
		// Full expand: return L0 (ingest text)
		step, err := h.store.GetStep(stepID)
		if err != nil {
			return nil, err
		}
		warning := ""
		if step.TokenCount > 4000 {
			warning = fmt.Sprintf("原文 %d tokens，注意窗口压力", step.TokenCount)
		}
		return &ExpandResult{
			StepID:  stepID,
			Level:   0,
			Content: step.Content,
			Tokens:  step.TokenCount,
			Warning: warning,
		}, nil
	}

	// Default expand: return L1 (denoised), fallback to L0 if L1 unavailable
	l1, err := h.store.GetL1(stepID)
	if err == nil {
		warning := ""
		if l1.TokenCount > 4000 {
			warning = fmt.Sprintf("L1 %d tokens，需要更多细节可用 full=true 展开原文", l1.TokenCount)
		}
		return &ExpandResult{
			StepID:  stepID,
			Level:   1,
			Content: l1.Content,
			Tokens:  l1.TokenCount,
			Warning: warning,
		}, nil
	}

	// Fallback to L0
	step, err := h.store.GetStep(stepID)
	if err != nil {
		return nil, err
	}
	warning := fmt.Sprintf("L1 不可用，返回 L0 原文 %d tokens", step.TokenCount)
	return &ExpandResult{
		StepID:  stepID,
		Level:   0,
		Content: step.Content,
		Tokens:  step.TokenCount,
		Warning: warning,
	}, nil
}

// Surround retrieves context around a step.
func (h *HybridManager) Surround(stepID int, before, after int) ([]RenderedBlock, error) {
	ids, err := h.store.AllActiveStepIDs()
	if err != nil {
		return nil, err
	}

	var blocks []RenderedBlock
	for _, id := range ids {
		if id >= stepID-before && id <= stepID+after {
			ref, err := h.store.GetRef(id)
			if err != nil {
				continue
			}
			content, err := h.renderStepContent(id, ref.Level)
			if err != nil {
				continue
			}
			step, _ := h.store.GetStep(id)
			typ := "reasoning"
			if step != nil {
				typ = step.Type
			}
			blocks = append(blocks, renderBlock(id, ref.Level, content, typ))
		}
	}
	return blocks, nil
}

// Stats returns current context statistics.
func (h *HybridManager) Stats() ContextStats {
	ids, _ := h.store.AllActiveStepIDs()
	levels := make(map[int]int)
	totalTokens := 0
	for _, id := range ids {
		ref, err := h.store.GetRef(id)
		if err != nil {
			continue
		}
		levels[ref.Level]++
		step, err := h.store.GetStep(id)
		if err != nil {
			continue
		}
		totalTokens += step.TokenCount
	}

	hotFacts, _ := h.store.HotFacts(h.cfg.HotFacts.MinRefCount, h.cfg.HotFacts.MinStrength)

	return ContextStats{
		TotalSteps:     int(atomic.LoadInt32(&h.currentStep)),
		ActiveSteps:    len(ids),
		TotalTokens:    totalTokens,
		LevelCounts:    levels,
		HotFacts:       len(hotFacts),
		WindowPressure: float64(totalTokens) / float64(h.cfg.WindowTokens),
	}
}

// estimateTotalTokens returns the approximate total token count across
// all active steps. Uses the sum of TokenCount from the store.
func (h *HybridManager) estimateTotalTokens() int {
	ids, err := h.store.AllActiveStepIDs()
	if err != nil {
		return 0
	}
	total := 0
	for _, id := range ids {
		step, err := h.store.GetStep(id)
		if err != nil {
			continue
		}
		total += step.TokenCount
	}
	return total
}

// maybeWarmup triggers async pre-compression when total tokens
// approach the sweet-spot threshold.
func (h *HybridManager) maybeWarmup() {
	if h.sweetSpotTokens <= 0 {
		return
	}
	if h.warmupPending.Load() {
		return
	}
	ratio := float64(h.estimateTotalTokens()) / float64(h.sweetSpotTokens)
	if ratio < h.cfg.WarmupRatio {
		return
	}
	h.warmupPending.Store(true)
	go h.warmupCompress()
}

// warmupCompress pre-compresses old steps (outside the tail zone) to L1.
func (h *HybridManager) warmupCompress() {
	defer h.warmupPending.Store(false)

	ids, err := h.store.AllActiveStepIDs()
	if err != nil || len(ids) <= h.cfg.TailKeepSteps {
		return
	}
	tailStart := len(ids) - h.cfg.TailKeepSteps
	for _, id := range ids[:tailStart] {
		ref, err := h.store.GetRef(id)
		if err != nil || ref.Level >= 1 {
			continue
		}
		ref.Level = 1
		_ = h.store.UpsertRef(*ref)
	}
}

// decayTolerance gradually reduces sweet-spot tolerance back to 1.0.
func (h *HybridManager) decayTolerance() {
	h.toleranceMu.Lock()
	defer h.toleranceMu.Unlock()

	if h.sweetSpotTolerance > 1.05 {
		h.sweetSpotTolerance *= h.cfg.ToleranceDecayRate
		h.sweetSpotTokens = int(float64(h.sweetSpotOriginal) * h.sweetSpotTolerance)
	} else if h.sweetSpotTolerance > 1.0 {
		h.sweetSpotTolerance = 1.0
		h.sweetSpotTokens = h.sweetSpotOriginal
	}
}

// UpdateCacheBudget adjusts the sweet-spot threshold based on actual
// cached_tokens reported by the LLM provider. When cachedTokens > 0,
// the sweet spot is raised to protect the cached prefix from decay.
// The increase is capped at 1.5× the original sweet-spot value to
// prevent window pressure. This is an additive-only method; the
// ContextManager interface is not changed.
func (h *HybridManager) UpdateCacheBudget(cachedTokens int) {
	if cachedTokens <= 0 {
		return
	}
	h.toleranceMu.Lock()
	defer h.toleranceMu.Unlock()

	effective := cachedTokens + h.estimateTotalTokens()
	cap := int(float64(h.sweetSpotOriginal) * 1.5)
	if effective > cap {
		effective = cap
	}
	if effective > h.sweetSpotTokens {
		h.sweetSpotTokens = effective
		// Also raise tolerance so decayTolerance starts from the new level.
		if h.sweetSpotOriginal > 0 {
			h.sweetSpotTolerance = float64(effective) / float64(h.sweetSpotOriginal)
		}
	}
}

// Close releases resources.
func (h *HybridManager) Close() error {
	return nil
}

// --- A-12: C-track transient step management ---

// MarkTransient marks a step as transient (excluded from BuildContext).
// Additive-only method; ContextManager interface unchanged.
func (h *HybridManager) MarkTransient(stepID int, scope string, round int) error {
	step, err := h.store.GetStep(stepID)
	if err != nil {
		return err
	}
	step.Transient = true
	step.TransientScope = scope
	step.TransientRound = round
	if err := h.store.AppendStep(*step); err != nil {
		return err
	}
	// Transient updates the corpus, so the fusion indexes (kw/recency) are stale.
	h.markFusionDirty()
	return nil
}

// ClearStaleTransients removes transient steps from the active set.
// Returns the number of steps removed.
func (h *HybridManager) ClearStaleTransients(currentRound int) (int, error) {
	ids, err := h.store.AllActiveStepIDs()
	if err != nil {
		return 0, err
	}
	count := 0
	for _, id := range ids {
		step, err := h.store.GetStep(id)
		if err != nil || !step.Transient {
			continue
		}
		if currentRound-step.TransientRound < 1 {
			continue // still fresh
		}
		_ = h.store.RemoveRef(id)
		count++
	}
	return count, nil
}

// --- Citation processing (§4.7.3) ---

var refCiteRe = regexp.MustCompile(`§(\d+)`)

// ProcessCitations parses §N references from LLM output and updates refs.
func (h *HybridManager) ProcessCitations(output string) {
	matches := refCiteRe.FindAllStringSubmatch(output, -1)
	currentStep := int(atomic.LoadInt32(&h.currentStep))

	for _, m := range matches {
		stepID := 0
		fmt.Sscanf(m[1], "%d", &stepID)
		if stepID <= 0 {
			continue
		}

		ref, err := h.store.GetRef(stepID)
		if err != nil {
			continue
		}
		ref.RefCount++
		ref.LastRefAtStep = &currentStep
		ref.Strength += 0.1
		applyRecallBoost(ref, int(h.taskGroupID), h.cfg.Decay)
		_ = h.store.UpsertRef(*ref)
	}
	if len(matches) > 0 {
		h.markFusionDirty()
	}
}

// SanitizeOutput replaces accidental ⟨§ patterns in LLM output (§4.7.1).
func SanitizeOutput(text string) string {
	return strings.ReplaceAll(text, "\u27E8\u00A7", "[\u00A7]")
}

// NextStepID returns the next step ID (for testing).
func (h *HybridManager) NextStepID() int {
	return int(atomic.LoadInt32(&h.currentStep)) + 1
}
