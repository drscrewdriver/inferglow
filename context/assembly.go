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
	"sync"
	"time"
)

// AssemblyManager implements ModeAssembly — the 9-layer context assembly engine
// (A-1/A-2/A-11). It embeds a HybridManager as the assembly engine and adds a
// layer-based view + two-phase Setup/Execute API + audit trail on top.
// The frozen ContextManager interface methods are fully delegated to the engine.
type AssemblyManager struct {
	engine *HybridManager // embedded assembly engine (all zone logic)
	cfg    Config
	store  StepStoreLike

	mu sync.RWMutex

	// A-11: assembly audit trail.
	audit *AssemblyAudit

	// Layer state (A-1): cached stable layers from Setup.
	layers   [10]LayerContent // index 1-9
	layerVer [10]int64
	setup    SetupRequest
}

// AssemblyAudit records a bounded ring of assembly entry audits.
type AssemblyAudit struct {
	mu         sync.Mutex
	entries    []AuditEntry
	maxEntries int
}

// NewAssemblyAudit creates an empty audit trail with a bounded capacity.
func NewAssemblyAudit() *AssemblyAudit {
	return &AssemblyAudit{maxEntries: 32}
}

// Record appends an audit entry, dropping the oldest beyond capacity.
func (a *AssemblyAudit) Record(entry AuditEntry) {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.entries = append(a.entries, entry)
	if len(a.entries) > a.maxEntries {
		a.entries = a.entries[len(a.entries)-a.maxEntries:]
	}
}

// AuditEntry is a single round's assembly audit summary.
type AuditEntry struct {
	RoundID     string
	Phase       string
	Layers      []LayerAudit
	TotalTokens int
	CacheHits   int
	CacheMisses int
	Duration    time.Duration
}

// LayerAudit describes one layer's participation in an assembly round.
type LayerAudit struct {
	Layer      string
	BlockCount int
	Tokens     int
}

// NewAssemblyManager creates an assembly mode context manager.
// It embeds a HybridManager engine for full zone assembly capability.
func NewAssemblyManager(cfg Config, store StepStoreLike) (ContextManager, error) {
	engine, err := NewHybridManager(cfg, store)
	if err != nil {
		return nil, err
	}
	return &AssemblyManager{
		engine: engine.(*HybridManager),
		cfg:    cfg,
		store:  store,
		audit:  NewAssemblyAudit(),
	}, nil
}

func (a *AssemblyManager) Mode() Mode { return ModeAssembly }

// --- ContextManager interface delegation (all 10 methods) ---

func (a *AssemblyManager) Ingest(step StepRecord) error { return a.engine.Ingest(step) }

func (a *AssemblyManager) BuildContext(ctx context.Context, windowTokens int) ([]RenderedBlock, error) {
	return a.engine.BuildContext(ctx, windowTokens)
}

func (a *AssemblyManager) TriggerCompression(ctx context.Context, opts CompressOpts) (*CompressResult, error) {
	return a.engine.TriggerCompression(ctx, opts)
}

func (a *AssemblyManager) Search(ctx context.Context, query SearchQuery) ([]SearchHit, error) {
	return a.engine.Search(ctx, query)
}

func (a *AssemblyManager) SearchLongMem(ctx context.Context, query string, category string, limit int) ([]LongMemRecord, error) {
	return a.engine.SearchLongMem(ctx, query, category, limit)
}

func (a *AssemblyManager) Expand(stepID int) (*ExpandResult, error) {
	return a.engine.Expand(stepID)
}

func (a *AssemblyManager) Surround(stepID int, before, after int) ([]RenderedBlock, error) {
	return a.engine.Surround(stepID, before, after)
}

func (a *AssemblyManager) Stats() ContextStats { return a.engine.Stats() }

func (a *AssemblyManager) Close() error { return a.engine.Close() }

// --- A-2: Two-phase assembly API (additive, not on frozen interface) ---

// SetupRequest specifies the stable layer content for AssembleSetup.
type SetupRequest struct {
	SafetyPolicy    string
	Identity        string
	Protocol        string
	TaskDescription string
	Prohibitions    []string
}

// ExecuteRequest specifies per-round parameters for AssembleExecute.
type ExecuteRequest struct {
	RoundID string
}

// AssemblyOutput is the result of a two-phase assembly call.
type AssemblyOutput struct {
	Layers []LayerContent
	Prompt string
	Tokens int
}

// AssembleSetup builds stable layers L1-L5 (called on session init / task switch).
func (a *AssemblyManager) AssembleSetup(ctx context.Context, req *SetupRequest) (*AssemblyOutput, error) {
	start := time.Now()

	// L4: delegate to engine head buffer.
	if req.TaskDescription != "" {
		a.engine.RewriteHeadBuffer([]RenderedBlock{{
			StepID:  0,
			Level:   0,
			Content: req.TaskDescription,
		}}, fmt.Sprintf("setup-%d", time.Now().UnixNano()))
	}

	// L5: delegate to engine constitutional.
	if len(req.Prohibitions) > 0 {
		a.engine.AppendConstitutional(req.Prohibitions)
	}

	// Build L1-L5 layer content.
	a.mu.Lock()
	a.setup = *req
	a.setLayer(LayerSystemSafety, req.SafetyPolicy, true)
	a.setLayer(LayerIdentity, req.Identity, true)
	a.setLayer(LayerProtocol, req.Protocol, true)
	a.setLayer(LayerTaskBackground, req.TaskDescription, true)
	prohib := ""
	for _, p := range req.Prohibitions {
		prohib += "- " + p + "\n"
	}
	a.setLayer(LayerProhibitions, prohib, true)
	a.mu.Unlock()

	// Collect output.
	layers := a.cachedLayers(1, 5)
	audit := AuditEntry{
		Phase:    "setup",
		Duration: time.Since(start),
	}
	for _, l := range layers {
		audit.Layers = append(audit.Layers, LayerAudit{
			Layer:  fmt.Sprintf("L%d", l.ID),
			Tokens: len(l.Content) / 4,
		})
		audit.TotalTokens += len(l.Content) / 4
	}
	audit.CacheHits = 5
	a.audit.Record(audit)

	return &AssemblyOutput{Layers: layers, Prompt: joinLayers(layers), Tokens: audit.TotalTokens}, nil
}

// AssembleExecute builds full L1-L9 (called every LLM round).
func (a *AssemblyManager) AssembleExecute(ctx context.Context, req *ExecuteRequest) (*AssemblyOutput, error) {
	start := time.Now()

	// L6-L9: delegate to engine BuildContext, then classify.
	blocks, err := a.engine.BuildContext(ctx, a.cfg.WindowTokens)
	if err != nil {
		return nil, err
	}

	// Build headStepIDs set for L4 classification.
	headBlocks := a.engine.HeadBlocks()
	headStepIDs := make(map[int]bool, len(headBlocks))
	for _, hb := range headBlocks {
		headStepIDs[hb.StepID] = true
	}

	volatileLayers := groupIntoLayers(blocks, headStepIDs)

	// Merge: L1-L5 from cache + L6-L9 from fresh build.
	a.mu.RLock()
	stableLayers := a.cachedLayersLocked(1, 5)
	a.mu.RUnlock()

	// Combine: stable first, then volatile (skip any stable duplicates from BuildContext).
	allLayers := make([]LayerContent, 0, 9)
	allLayers = append(allLayers, stableLayers...)
	for _, vl := range volatileLayers {
		if vl.ID >= 6 {
			allLayers = append(allLayers, vl)
		}
	}

	// Audit.
	audit := AuditEntry{
		RoundID:   req.RoundID,
		Phase:     "execute",
		Duration:  time.Since(start),
		CacheHits: len(stableLayers),
	}
	for _, l := range allLayers {
		audit.Layers = append(audit.Layers, LayerAudit{
			Layer:  fmt.Sprintf("L%d", l.ID),
			Tokens: len(l.Content) / 4,
		})
		audit.TotalTokens += len(l.Content) / 4
	}
	a.audit.Record(audit)

	return &AssemblyOutput{Layers: allLayers, Prompt: joinLayers(allLayers), Tokens: audit.TotalTokens}, nil
}

// --- A-11: Layer access + invalidation + stats ---

// GetLayer returns the cached content for a layer.
func (a *AssemblyManager) GetLayer(layer LayerID) (*LayerContent, error) {
	if layer < 1 || layer > 9 {
		return nil, fmt.Errorf("invalid layer %d", layer)
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.layers[layer].Content == "" && a.layers[layer].ID == 0 {
		return nil, fmt.Errorf("layer %d not populated", layer)
	}
	l := a.layers[layer]
	return &l, nil
}

// InvalidateLayer clears a cached layer, forcing rebuild on next access.
func (a *AssemblyManager) InvalidateLayer(layer LayerID) {
	if layer < 1 || layer > 9 {
		return
	}
	a.mu.Lock()
	a.layers[layer] = LayerContent{}
	a.mu.Unlock()
}

// GetLayerCacheStats returns per-layer hit/miss statistics.
func (a *AssemblyManager) GetLayerCacheStats() map[LayerID]LayerCacheStat {
	a.mu.RLock()
	defer a.mu.RUnlock()
	stats := make(map[LayerID]LayerCacheStat)
	for i := 1; i <= 9; i++ {
		if a.layers[i].ID != 0 {
			stats[LayerID(i)] = LayerCacheStat{Hits: a.layerVer[i]}
		}
	}
	return stats
}

// --- internal helpers ---

func (a *AssemblyManager) setLayer(id LayerID, content string, stable bool) {
	a.layerVer[id]++
	a.layers[id] = LayerContent{
		ID:      id,
		Content: content,
		Sha256:  computeHash(content),
		Version: a.layerVer[id],
		Stable:  stable,
	}
}

func (a *AssemblyManager) cachedLayers(from, to int) []LayerContent {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cachedLayersLocked(from, to)
}

func (a *AssemblyManager) cachedLayersLocked(from, to int) []LayerContent {
	var out []LayerContent
	for i := from; i <= to; i++ {
		if a.layers[i].ID != 0 {
			out = append(out, a.layers[i])
		}
	}
	return out
}

func joinLayers(layers []LayerContent) string {
	prompt := ""
	for _, l := range layers {
		if l.Content != "" {
			prompt += l.Content + "\n\n"
		}
	}
	return prompt
}

// compile-time assertion that AssemblyManager satisfies ContextManager.
var _ ContextManager = (*AssemblyManager)(nil)