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
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
// MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO
// EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES
// OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
// ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
// DEALINGS IN THE SOFTWARE.

package cli

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	contextmgr "github.com/inferglow/context"
	"github.com/inferglow/context/compress"
	"github.com/inferglow/context/retrieval"
	"github.com/inferglow/context/store/jsonl"
	"github.com/inferglow/context/toolclean"
	"github.com/inferglow/builtins/actions"
	"github.com/inferglow/memory"
	"github.com/inferglow/skill"
)

// MemoryBridge connects the CLI agent with the context management and
// long-term memory systems. It handles recall (search → inject) and
// ingest (store → index → promote) in a single abstraction.
type MemoryBridge struct {
	mgr       contextmgr.ContextManager
	hybrid    *contextmgr.HybridManager // concrete type for AppendConstitutional
	retriever *retrieval.FusionRetriever
	bm25      *retrieval.BM25Index
	promoter  *contextmgr.LongMemPromoter
	store     *jsonl.Store
	memStore  memory.Store  // file-based auto-memory store
	skillStore *skill.Store // procedural memory (skill store)
	projectRoot string      // project root for meta-memory scanning
	topK      int
	stepSeq   int
	sessionID string

	// Auto-background trigger state (CM-2).
	autoBgThreshold int  // fire after this many steps (0 = disabled)
	autoBgTriggered bool // once-only flag

	// Optional injected backends (nil = use defaults).
	externalStore   StepStoreInjector
	vectorBackend   VectorBackendInjector

	// MC-2: compression model chain (nil = no dedicated compress model).
	compressChain *compress.CompressModelChain

	// Task tracker store (T1).
	taskStore *actions.TaskStore
	// BM25 index persistence path (B4).
	bm25File string

	// Orthogonal tool-output denoise gate (FeatureFlags.ToolDenoise).
	// Independent of ContextMode: applied in IngestTool before any
	// context manager — in any mode — sees the content.
	toolDenoise bool
}

// maxDenoiseInput bounds the denoise scan. trusted_local bash output is
// unbounded (cli/local_bash.go collects stdout into an unlimited buffer),
// so the gate clamps before scanning; the 8192-byte ingest truncation
// downstream makes anything beyond this bound moot anyway.
const maxDenoiseInput = 1 << 20 // 1 MiB

// NewMemoryBridge creates a fully wired memory bridge for the given session.
func NewMemoryBridge(cfg CLIConfig, sessionID string) (*MemoryBridge, error) {
	dataDir := cfg.DataDir + "/sessions"

	// 1. JSONL store — persistence backend.
	store, err := jsonl.New(dataDir, sessionID)
	if err != nil {
		return nil, fmt.Errorf("memory bridge: create store: %w", err)
	}

	// 2. Context manager — selected by cfg.ContextMode.
	ctxCfg := contextmgr.DefaultConfig()
	ctxCfg.WindowTokens = cfg.WindowTokens
	ctxCfg.LongMem.Enabled = true

	var mgr contextmgr.ContextManager
	switch cfg.ContextMode {
	case "passthrough":
		mgr, err = contextmgr.NewPassthroughManager(ctxCfg, store)
	case "three_zone":
		mgr, err = contextmgr.NewThreeZoneAdapter(ctxCfg, store)
	case "summary":
		// Summary mode requires RewritableSession + Summarizer; fallback to hybrid.
		fmt.Fprintln(os.Stderr, "Warning: summary mode not yet supported, using hybrid")
		mgr, err = contextmgr.NewHybridManager(ctxCfg, store)
	default: // "hybrid" or empty
		mgr, err = contextmgr.NewHybridManager(ctxCfg, store)
	}
	if err != nil {
		return nil, fmt.Errorf("memory bridge: create context manager: %w", err)
	}

	// Keep concrete type for Zone 1 operations (nil for non-hybrid modes).
	hybrid, _ := mgr.(*contextmgr.HybridManager)

	// CM-3: inject meta-operation instructions into Zone 0.5.
	if cfg.Features.Constitutional && cfg.Features.MetaInstructions && hybrid != nil {
		entries := contextmgr.MetaInstructionEntries(ctxCfg)
		hybrid.AppendConstitutional(entries)
	}

	// 3. BM25 keyword index (with persistence restore).
	bm25 := retrieval.NewBM25Index()
	bm25File := dataDir + "/" + sessionID + ".bm25.json"
	if f, err := os.Open(bm25File); err == nil {
		_ = bm25.Load(f)
		f.Close()
	}

	// 4. Fusion retriever — degrades to BM25 + recency (no semantic).
	fusion := retrieval.NewFusionRetriever(nil, bm25, nil, &retrieval.NoopEmbedder{})

	// 5. Long-term memory promoter.
	promoter := contextmgr.NewLongMemPromoter(store, ctxCfg.LongMem, sessionID)

	// 6. File-based auto-memory store (Reasonix-compatible).
	memStore := memory.StoreFor(cfg.DataDir, ".")

	// 7. Skill store (procedural memory).
	skillDir := cfg.DataDir + "/projects/default/skills"
	globalSkillDir := cfg.DataDir + "/skills/global"
	skillStore := skill.NewStore(skillDir, globalSkillDir)

	// 8. MC-2: build compress model chain if configured.
	var compressChain *compress.CompressModelChain
	if cfg.CompressModel != nil {
		smallClient, _ := buildCompressModelClient(cfg.CompressModel)
		mainClient, _ := buildCompressModelClient(&cfg.LLM)
		if smallClient != nil || mainClient != nil {
			compressChain = compress.NewCompressModelChain(smallClient, mainClient, 5*time.Second, 1)
		}
	}

	// 9. Task tracker store.
	taskFile := dataDir + "/" + sessionID + ".tasks.json"
	taskStore := actions.NewTaskStore(taskFile)

	return &MemoryBridge{
		mgr:        mgr,
		hybrid:     hybrid,
		retriever:  fusion,
		bm25:       bm25,
		promoter:   promoter,
		store:      store,
		memStore:   memStore,
		skillStore: skillStore,
		projectRoot: ".",
		topK:            cfg.TopK,
		sessionID:       sessionID,
		autoBgThreshold: autoBgStepThreshold(cfg.Features.AutoBackground),
		compressChain:   compressChain,
		taskStore:       taskStore,
		bm25File:        bm25File,
		toolDenoise:     cfg.Features.ToolDenoise,
	}, nil
}

// Recall searches for relevant memories and returns formatted text + cited IDs.
func (b *MemoryBridge) Recall(ctx context.Context, query string) (string, []string) {
	if b.topK <= 0 {
		return "", nil
	}

	var memIDs []string
	var sb strings.Builder

	// BM25 keyword search via FusionRetriever.
	results, err := b.retriever.Search(ctx, query, b.topK)
	if err == nil {
		for _, r := range results {
			memID := fmt.Sprintf("step_%d", r.StepID)
			memIDs = append(memIDs, memID)
			sb.WriteString(fmt.Sprintf("- [step %d] %s\n", r.StepID, r.Text))
		}
	}

	// Long-term memory search.
	longMemResults, err := b.mgr.SearchLongMem(ctx, query, "", b.topK)
	if err == nil {
		for _, lm := range longMemResults {
			memIDs = append(memIDs, lm.MemID)
			sb.WriteString(fmt.Sprintf("- [mem %s] %s\n", lm.MemID, strings.Join(lm.Facts, "; ")))
		}
	}

	text := sb.String()
	if text == "" {
		return "", nil
	}

	formatted := "<relevant_memories>\n" + text + "</relevant_memories>\n"
	return formatted, memIDs
}

// nextStepID returns a monotonically increasing step ID for indexing.
func (b *MemoryBridge) nextStepID() int {
	b.stepSeq++
	return b.stepSeq
}

// IngestTool stores a tool result into the context manager and BM25 index.
func (b *MemoryBridge) IngestTool(toolName, content string) {
	content = b.denoiseToolOutput(toolName, content)
	content = truncate(content, 8192)
	step := contextmgr.StepRecord{
		Type:       "tool",
		Role:       "tool",
		Content:    content,
		ToolName:   toolName,
		TokenCount: estimateTokens(content),
		CreatedAt:  time.Now().Unix(),
	}
	if err := b.mgr.Ingest(step); err != nil {
		return
	}
	stepID := b.nextStepID()
	b.bm25.Add(stepID, content)
	// Best-effort promotion check.
	_ = b.promoter.EvaluateAndPromote(context.Background())
}

// denoiseToolOutput applies the orthogonal mechanical denoise gate to tool
// output before it reaches any context manager or the BM25 index. It is
// independent of ContextMode (the gate sits upstream of mode selection and
// survives /mode switches) and fails open: when the flag is off, or when
// the cleaner reports no change, the content passes through untouched.
// TokenCount and the BM25 index below are computed from the returned
// (cleaned) content, keeping measurements consistent with what is stored.
func (b *MemoryBridge) denoiseToolOutput(toolName, content string) string {
	if !b.toolDenoise {
		return content
	}
	if len(content) > maxDenoiseInput {
		content = content[:maxDenoiseInput]
	}
	cleaned, rep := toolclean.Clean(content)
	if !rep.Changed {
		return content
	}
	log.Printf("[tool_denoise] %s: %d -> %d bytes (ansi=%d cr=%d dup=%d err_kept=%d)",
		toolName, rep.InputBytes, rep.OutputBytes,
		rep.ANSIRemoved, rep.CRFolded, rep.DupLinesRemoved, rep.ErrorLinesKept)
	return cleaned
}

// IngestUser stores a user message.
func (b *MemoryBridge) IngestUser(content string) {
	step := contextmgr.StepRecord{
		Type:       "user",
		Role:       "user",
		Content:    content,
		TokenCount: estimateTokens(content),
		CreatedAt:  time.Now().Unix(),
	}
	_ = b.mgr.Ingest(step)
	stepID := b.nextStepID()
	b.bm25.Add(stepID, content)
}

// IngestAssistant stores an assistant response.
func (b *MemoryBridge) IngestAssistant(content string) {
	step := contextmgr.StepRecord{
		Type:       "reasoning",
		Role:       "assistant",
		Content:    content,
		TokenCount: estimateTokens(content),
		CreatedAt:  time.Now().Unix(),
	}
	_ = b.mgr.Ingest(step)
	stepID := b.nextStepID()
	b.bm25.Add(stepID, content)
}

// ValidateCited boosts confidence for cited memories.
func (b *MemoryBridge) ValidateCited(memIDs []string) {
	for _, id := range memIDs {
		_ = b.promoter.ValidateMemory(id, b.stepSeq)
	}
}

// Stats returns context statistics.
func (b *MemoryBridge) Stats() contextmgr.ContextStats {
	return b.mgr.Stats()
}

// Compact triggers manual compression.
func (b *MemoryBridge) Compact(ctx context.Context) error {
	_, err := b.mgr.TriggerCompression(ctx, contextmgr.CompressOpts{})
	return err
}

// ForceAsyncCompress triggers forced compression, bypassing sweet-spot threshold.
// This is used by /async-compress TUI command for manual intervention.
func (b *MemoryBridge) ForceAsyncCompress(ctx context.Context) (*contextmgr.CompressResult, error) {
	opts := contextmgr.CompressOpts{
		Force:       true,
		TargetLevel: 2, // compress to L2 (fact retention)
	}
	return b.mgr.TriggerCompression(ctx, opts)
}

// SearchMemory searches long-term memory.
func (b *MemoryBridge) SearchMemory(ctx context.Context, query string) ([]contextmgr.LongMemRecord, error) {
	return b.mgr.SearchLongMem(ctx, query, "", b.topK*2)
}

// OnSessionEnd performs end-of-session cleanup: final promotion + close store.
func (b *MemoryBridge) OnSessionEnd(ctx context.Context) {
	_ = b.promoter.OnSessionEnd(ctx)
	// Persist BM25 index.
	if b.bm25File != "" {
		if f, err := os.Create(b.bm25File); err == nil {
			_ = b.bm25.Save(f)
			f.Close()
		}
	}
	_ = b.store.Close()
}

// MemoryIndex returns the MEMORY.md index content for injection into
// the system prompt. Returns "" if no memories are stored.
func (b *MemoryBridge) MemoryIndex() string {
	return b.memStore.Index()
}

// StandingFacts returns the memory index formatted as standing facts
// for injection into compaction summaries.
func (b *MemoryBridge) StandingFacts() string {
	idx := b.memStore.Index()
	if idx == "" {
		return ""
	}
	return "<standing_facts>\n" + idx + "</standing_facts>"
}

// MemStore returns the underlying file-based memory store.
func (b *MemoryBridge) MemStore() memory.Store {
	return b.memStore
}

// ContextManager returns the context manager.
func (b *MemoryBridge) ContextManager() contextmgr.ContextManager {
	return b.mgr
}

// SkillStore returns the skill store for procedural memory.
func (b *MemoryBridge) SkillStore() *skill.Store {
	return b.skillStore
}

// TaskStore returns the task tracker store.
func (b *MemoryBridge) TaskStore() *actions.TaskStore {
	return b.taskStore
}

// TaskSummary returns the current task progress summary for context injection.
func (b *MemoryBridge) TaskSummary() string {
	if b.taskStore == nil {
		return ""
	}
	return b.taskStore.Summary()
}

// ProjectRoot returns the project root directory used for meta-memory scanning.
func (b *MemoryBridge) ProjectRoot() string {
	return b.projectRoot
}

// BuildSystemPrompt assembles the complete system prompt with memory layers.
// Order: base prompt + semantic memory + skills index + project instructions.
func (b *MemoryBridge) BuildSystemPrompt(base, query string) string {
	prompt := base

	// Semantic memory injection
	if memText, _ := b.Recall(context.Background(), query); memText != "" {
		prompt += "\n\n" + memText
	}

	// Procedural memory: skills index injection
	if b.skillStore != nil {
		if skills := b.skillStore.IndexBlock(); skills != "" {
			prompt += "\n\n" + skills
		}

		// Meta-memory: project instructions (AGENTS.md etc.)
		if instructions := b.skillStore.ProjectInstructions(b.projectRoot); instructions != "" {
			prompt += "\n\n" + instructions
		}
	}

	// Task progress injection
	if b.taskStore != nil {
		if summary := b.taskStore.Summary(); summary != "" {
			prompt += "\n\n<task_progress>\n" + summary + "\n</task_progress>"
		}
	}

	return prompt
}

// AppendConstitutional adds constitutional entries to the hybrid manager.
func (b *MemoryBridge) AppendConstitutional(entries []string) {
	if b.hybrid != nil {
		b.hybrid.AppendConstitutional(entries)
	}
}

// RewriteHeadBuffer replaces the Zone 1 head buffer with new content.
// This is the correct target for /rebackground (replaces AppendConstitutional).
func (b *MemoryBridge) RewriteHeadBuffer(blocks []contextmgr.RenderedBlock, version string) {
	if b.hybrid != nil {
		b.hybrid.RewriteHeadBuffer(blocks, version)
	}
}

// IsHeadBufferEmpty reports whether Zone 1 (head buffer) has been populated.
func (b *MemoryBridge) IsHeadBufferEmpty() bool {
	if b.hybrid == nil {
		return true
	}
	return b.hybrid.IsHeadBufferEmpty()
}

// NeedsAutoBackground returns true if the session has passed threshold steps
// but Zone 1 (head buffer) is still empty — signaling that auto-rebackground
// should fire.
func (b *MemoryBridge) NeedsAutoBackground(threshold int) bool {
	if b.hybrid == nil {
		return false
	}
	// Use CurrentStepAtomic from introspect.go for the step counter.
	step := contextmgr.CurrentStepAtomic(b.hybrid)
	return int(step) >= threshold && b.hybrid.IsHeadBufferEmpty()
}

// autoBgStepThreshold returns the auto-rebackground step threshold for a
// session. 0 disables the feature (features.auto_background=false), so
// CheckAutoBackground never fires and the project-analysis tool loop
// (list_dir / bash_executor) is not run automatically. When enabled, the
// historical default of 3 steps applies.
func autoBgStepThreshold(enabled bool) int {
	if !enabled {
		return 0
	}
	return 3
}

// CheckAutoBackground is a once-only check that returns true when the session
// has passed the auto-background threshold and Zone 1 is still empty.
// Callers (TUI submitTurn, REPL chatOnce) should trigger rebackground when
// this returns true. Subsequent calls return false.
func (b *MemoryBridge) CheckAutoBackground() bool {
	if b.autoBgTriggered || b.autoBgThreshold <= 0 {
		return false
	}
	if !b.NeedsAutoBackground(b.autoBgThreshold) {
		return false
	}
	b.autoBgTriggered = true
	return true
}

// AppendPlanToHeadBuffer appends a plan summary block to Zone 1 (CM-5).
// The plan block uses pseudo-step -6 and is appended alongside existing
// background blocks rather than replacing them.
func (b *MemoryBridge) AppendPlanToHeadBuffer(title, summary string) {
	if b.hybrid == nil {
		return
	}
	block := contextmgr.PlanSummaryBlock(title, summary)
	existing := b.hybrid.HeadBlocks()
	existing = append(existing, block)
	b.hybrid.RewriteHeadBuffer(existing, "plan-"+title)
}

// CurrentMode returns the active context management mode name (MC-3).
func (b *MemoryBridge) CurrentMode() string {
	if b.mgr == nil {
		return "none"
	}
	return string(b.mgr.Mode())
}

// SwitchMode performs a runtime context mode switch (MC-3).
// Uses contextmgr.Registry.SwitchMode to share the underlying StepStore.
func (b *MemoryBridge) SwitchMode(mode string) error {
	targetMode := contextmgr.Mode(mode)
	currentMode := b.mgr.Mode()
	if currentMode == targetMode {
		return nil // already in target mode
	}

	// Build a registry with all mode factories.
	reg := contextmgr.NewRegistry()
	reg.Register(contextmgr.ModePassthrough, func(cfg contextmgr.Config, store contextmgr.StepStoreLike) (contextmgr.ContextManager, error) {
		return contextmgr.NewPassthroughManager(cfg, store)
	})
	reg.Register(contextmgr.ModeThreeZone, func(cfg contextmgr.Config, store contextmgr.StepStoreLike) (contextmgr.ContextManager, error) {
		return contextmgr.NewThreeZoneAdapter(cfg, store)
	})
	reg.Register(contextmgr.ModeHybrid, func(cfg contextmgr.Config, store contextmgr.StepStoreLike) (contextmgr.ContextManager, error) {
		return contextmgr.NewHybridManager(cfg, store)
	})
	// summary mode falls back to hybrid (not yet fully supported).
	reg.Register(contextmgr.ModeSummary, func(cfg contextmgr.Config, store contextmgr.StepStoreLike) (contextmgr.ContextManager, error) {
		return contextmgr.NewHybridManager(cfg, store)
	})
	// assembly mode (线A/C轨) — hot-switchable via the shared Registry.
	reg.Register(contextmgr.ModeAssembly, func(cfg contextmgr.Config, store contextmgr.StepStoreLike) (contextmgr.ContextManager, error) {
		return contextmgr.NewAssemblyManager(cfg, store)
	})

	// Get the config and store from the current manager.
	cfg := contextmgr.DefaultConfig()
	store := b.store

	newMgr, err := reg.SwitchMode(currentMode, targetMode, cfg, store)
	if err != nil {
		return fmt.Errorf("switch mode to %s: %w", mode, err)
	}

	b.mgr = newMgr
	b.hybrid, _ = newMgr.(*contextmgr.HybridManager)
	return nil
}

// StepStoreInjector is an optional external step store (e.g. Postgres)
// that can replace the default JSONL backend.
type StepStoreInjector interface {
	IngestStep(rec contextmgr.StepRecord) error
	SearchSteps(ctx context.Context, query string, limit int) ([]contextmgr.StepRecord, error)
}

// WithStepStore replaces the JSONL store with an external store.
// When called, the bridge skips JSONL persistence and delegates to the
// injected store for all ingest/search operations.
func (b *MemoryBridge) WithStepStore(s StepStoreInjector) {
	b.externalStore = s
}

// VectorBackendInjector is an optional vector search backend that can
// replace the in-memory BM25 + noop-embedder default.
type VectorBackendInjector interface {
	VectorSearch(ctx context.Context, query string, limit int) ([]retrieval.SearchResult, error)
}

// WithVectorBackend replaces the default BM25-only search with an
// external vector backend (e.g. pgvector, Redis VSS).
func (b *MemoryBridge) WithVectorBackend(vb VectorBackendInjector) {
	b.vectorBackend = vb
}

// truncate limits content to maxBytes.
func truncate(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	return s[:maxBytes] + "...[truncated]"
}

// estimateTokens provides a rough token count.
// English: ~4 chars/token, CJK: ~1-2 chars/token.
// Uses rune count for better accuracy with mixed content.
func estimateTokens(s string) int {
	if s == "" {
		return 0
	}
	runes := []rune(s)
	n := len(runes)
	// Rough estimate: 1 token ≈ 3 runes for mixed content
	tokens := n / 3
	if tokens < 1 {
		return 1
	}
	return tokens
}
