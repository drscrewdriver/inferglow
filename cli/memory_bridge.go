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
	"strings"
	"time"

	contextmgr "github.com/inferglow/context"
	"github.com/inferglow/context/retrieval"
	"github.com/inferglow/context/store/jsonl"
	"github.com/inferglow/memory"
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
	memStore  memory.Store // file-based auto-memory store
	topK      int
	stepSeq   int
	sessionID string
}

// NewMemoryBridge creates a fully wired memory bridge for the given session.
func NewMemoryBridge(cfg CLIConfig, sessionID string) (*MemoryBridge, error) {
	dataDir := cfg.DataDir + "/sessions"

	// 1. JSONL store — persistence backend.
	store, err := jsonl.New(dataDir, sessionID)
	if err != nil {
		return nil, fmt.Errorf("memory bridge: create store: %w", err)
	}

	// 2. Hybrid context manager — compression engine.
	ctxCfg := contextmgr.DefaultConfig()
	ctxCfg.WindowTokens = cfg.WindowTokens
	ctxCfg.LongMem.Enabled = true

	mgr, err := contextmgr.NewHybridManager(ctxCfg, store)
	if err != nil {
		return nil, fmt.Errorf("memory bridge: create hybrid manager: %w", err)
	}

	// Keep concrete type for AppendConstitutional.
	hybrid, _ := mgr.(*contextmgr.HybridManager)

	// 3. BM25 keyword index.
	bm25 := retrieval.NewBM25Index()

	// 4. Fusion retriever — degrades to BM25 + recency (no semantic).
	fusion := retrieval.NewFusionRetriever(nil, bm25, nil, &retrieval.NoopEmbedder{})

	// 5. Long-term memory promoter.
	promoter := contextmgr.NewLongMemPromoter(store, ctxCfg.LongMem, sessionID)

	// 6. File-based auto-memory store (Reasonix-compatible).
	memStore := memory.StoreFor(cfg.DataDir, ".")

	return &MemoryBridge{
		mgr:       mgr,
		hybrid:    hybrid,
		retriever: fusion,
		bm25:      bm25,
		promoter:  promoter,
		store:     store,
		memStore:  memStore,
		topK:      cfg.TopK,
		sessionID: sessionID,
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

// SearchMemory searches long-term memory.
func (b *MemoryBridge) SearchMemory(ctx context.Context, query string) ([]contextmgr.LongMemRecord, error) {
	return b.mgr.SearchLongMem(ctx, query, "", b.topK*2)
}

// OnSessionEnd performs end-of-session cleanup: final promotion + close store.
func (b *MemoryBridge) OnSessionEnd(ctx context.Context) {
	_ = b.promoter.OnSessionEnd(ctx)
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

// AppendConstitutional adds constitutional entries to the hybrid manager.
func (b *MemoryBridge) AppendConstitutional(entries []string) {
	if b.hybrid != nil {
		b.hybrid.AppendConstitutional(entries)
	}
}

// truncate limits content to maxBytes.
func truncate(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	return s[:maxBytes] + "...[truncated]"
}

// estimateTokens provides a rough token count (4 chars ≈ 1 token).
func estimateTokens(s string) int {
	n := len(s) / 4
	if n < 1 {
		return 1
	}
	return n
}
