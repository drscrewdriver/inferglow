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
// AVERROR OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package compress

import (
	"context"
	"strings"
	"testing"

	contextmgr "github.com/inferglow/context"
)

func TestMaskHeaderRegex_Valid(t *testing.T) {
	validCases := []string{
		"[掩码 step_1|原5t|tool|params]",
		"[掩码 step_42|原128t|read_file|path=main.go]",
		"[掩码 step_0|原0t|tool|p]",
		"[掩码 step_999|原1000t|write_file|path=test.go,content=hello]",
	}

	for _, c := range validCases {
		if !maskHeaderRegex.MatchString(c) {
			t.Errorf("expected valid mask header %q to match regex", c)
		}
	}
}

func TestMaskHeaderRegex_Invalid(t *testing.T) {
	invalidCases := []string{
		"",
		"普通文本 without mask",
		"[掩码 step_|原5t|tool|params]",       // missing step number
		"[掩码 step_1|原t|tool|params]",       // missing token count
		"[掩码 step_1||tool|params]",         // empty token count field
		"掩码 step_1|原5t|tool|params]",       // missing opening bracket
		"[掩码 step_1|原5t|tool|params",       // missing closing bracket
		"[Mask step_1|原5t|tool|params]",    // wrong prefix
		"先有文本 [掩码 step_1|原5t|tool|params]", // text before mask
	}

	for _, c := range invalidCases {
		if maskHeaderRegex.MatchString(c) {
			t.Errorf("expected invalid mask header %q to NOT match regex", c)
		}
	}
}

// mockCompressClient implements CompressModelClient for testing.
type mockCompressClient struct {
	compressFn func(ctx context.Context, level int, prompt string) (string, error)
	available  bool
}

func (m *mockCompressClient) Compress(ctx context.Context, level int, prompt string) (string, error) {
	if m.compressFn != nil {
		return m.compressFn(ctx, level, prompt)
	}
	return "", nil
}

func (m *mockCompressClient) Available() bool {
	return m.available
}

func TestCompressModelChain_Interface(t *testing.T) {
	// Test 1: small model succeeds
	small := &mockCompressClient{
		available: true,
		compressFn: func(ctx context.Context, level int, prompt string) (string, error) {
			return "small result", nil
		},
	}
	main := &mockCompressClient{
		available: true,
		compressFn: func(ctx context.Context, level int, prompt string) (string, error) {
			return "main result", nil
		},
	}
	chain := NewCompressModelChain(small, main, 0, 0)
	ctx := context.Background()
	result, err := chain.Compress(ctx, 1, "test prompt with enough length to pass validation")
	if err != nil {
		t.Fatalf("Compress returned error: %v", err)
	}
	if result != "small result" {
		t.Errorf("expected small model result, got %q", result)
	}

	// Test 2: small model unavailable, main model succeeds
	small2 := &mockCompressClient{available: false}
	main2 := &mockCompressClient{
		available: true,
		compressFn: func(ctx context.Context, level int, prompt string) (string, error) {
			return "main result", nil
		},
	}
	chain2 := NewCompressModelChain(small2, main2, 0, 0)
	result2, err2 := chain2.Compress(ctx, 1, "test prompt")
	if err2 != nil {
		t.Fatalf("Compress returned error: %v", err2)
	}
	if result2 != "main result" {
		t.Errorf("expected main model result, got %q", result2)
	}

	// Test 3: both models unavailable, falls back to mechanical
	small3 := &mockCompressClient{available: false}
	main3 := &mockCompressClient{available: false}
	chain3 := NewCompressModelChain(small3, main3, 0, 0)
	result3, err3 := chain3.Compress(ctx, 1, "test prompt with some content")
	if err3 != nil {
		t.Fatalf("Compress returned error: %v", err3)
	}
	if !strings.Contains(result3, "test prompt") {
		t.Errorf("expected mechanical fallback output, got %q", result3)
	}

	// Test 4: validate L2/L3 requires mask header, falls back if invalid
	small4 := &mockCompressClient{
		available: true,
		compressFn: func(ctx context.Context, level int, prompt string) (string, error) {
			return "invalid output without mask header", nil
		},
	}
	main4 := &mockCompressClient{
		available: true,
		compressFn: func(ctx context.Context, level int, prompt string) (string, error) {
			return "also invalid", nil
		},
	}
	chain4 := NewCompressModelChain(small4, main4, 0, 0)
	result4, err4 := chain4.Compress(ctx, 2, "some content")
	if err4 != nil {
		t.Fatalf("Compress returned error: %v", err4)
	}
	// Should fall back to mechanical L2 which doesn't produce mask header from raw content
	// but still returns something
	if result4 == "" {
		t.Errorf("expected non-empty result from mechanical fallback")
	}
}

// newPairingTestEngine builds an engine over a store with two paired tool steps.
func newPairingTestEngine(t *testing.T, callTokens, resultTokens int) (*Engine, *mockStepStore) {
	t.Helper()
	store := newMockStepStore()
	store.addStep(contextmgr.StepRecord{StepID: 1, Type: "tool", Role: "assistant", CallID: "c1", Content: strings.Repeat("c", 100), TokenCount: callTokens, ToolName: "bash"})
	store.addStep(contextmgr.StepRecord{StepID: 2, Type: "tool", Role: "tool", CallID: "c1", Content: strings.Repeat("r", 100), TokenCount: resultTokens, ToolName: "bash"})
	chain := NewCompressModelChain(&mockCompressClient{available: true, compressFn: func(ctx context.Context, level int, prompt string) (string, error) {
		return "[", nil // invalid on purpose: never used when pairing drops the batch
	}}, nil, 0, 0)
	return NewEngine(chain, store, contextmgr.Config{}), store
}

func TestPairingBalanced_ToolWithResult(t *testing.T) {
	_, store := newPairingTestEngine(t, 10000, 10000)
	e := &Engine{store: store}
	include := map[int]bool{1: true, 2: true}
	got, removed := e.pairingBalanced([]int{1, 2}, include)
	if !got[1] || !got[2] {
		t.Errorf("both paired steps must stay in batch, got %v", got)
	}
	if len(removed) != 0 {
		t.Errorf("no step should be removed, got %v", removed)
	}
}

func TestPairingBalanced_OrphanResultSkipped(t *testing.T) {
	_, store := newPairingTestEngine(t, 100, 10000)
	e := &Engine{store: store}
	// Step 2 is the selected candidate; step 1 (its call) is not selected.
	include := map[int]bool{2: true}
	_, removed := e.pairingBalanced([]int{1, 2}, include)
	if len(removed) != 1 || removed[0] != 2 {
		t.Errorf("orphan result step 2 should be skipped, got removed=%v", removed)
	}
}

func TestPairingBalanced_PartialGroupDropped(t *testing.T) {
	_, store := newPairingTestEngine(t, 10000, 100)
	e := &Engine{store: store}
	// Step 1 selected, step 2 not: the whole group must leave the batch.
	include := map[int]bool{1: true}
	got, removed := e.pairingBalanced([]int{1, 2}, include)
	if got[1] {
		t.Errorf("step 1 must be dropped with its unpaired group, got %v", got)
	}
	if len(removed) != 1 || removed[0] != 1 {
		t.Errorf("expected step 1 removed, got %v", removed)
	}
}

func TestBatchCompress_KeepsPairing(t *testing.T) {
	e, store := newPairingTestEngine(t, 10000, 100)
	result, err := e.BatchCompress(context.Background())
	if err != nil {
		t.Fatalf("BatchCompress error: %v", err)
	}
	if result.StepsCompressed != 0 {
		t.Errorf("StepsCompressed = %d, want 0 (pairing guard must drop the call)", result.StepsCompressed)
	}
	ref1, _ := store.GetRef(1)
	ref2, _ := store.GetRef(2)
	if ref1.Level != 0 || ref2.Level != 0 {
		t.Errorf("levels must stay L0, got %d/%d", ref1.Level, ref2.Level)
	}
	if result.CompactionID == "" {
		t.Errorf("expected a compaction transaction id")
	}
	if len(result.ShadowedStepIDs) != 0 {
		t.Errorf("no steps should be shadowed, got %v", result.ShadowedStepIDs)
	}
}

func TestCompressStep_SetsCompactionID(t *testing.T) {
	store := newMockStepStore()
	store.addStep(contextmgr.StepRecord{StepID: 1, Type: "tool", Role: "assistant", CallID: "c1", Content: "content one", TokenCount: 100, ToolName: "bash"})
	store.addStep(contextmgr.StepRecord{StepID: 2, Type: "tool", Role: "tool", CallID: "c1", Content: "result two", TokenCount: 100, ToolName: "bash"})
	chain := NewCompressModelChain(&mockCompressClient{available: true, compressFn: func(ctx context.Context, level int, prompt string) (string, error) {
		return "compressed", nil
	}}, nil, 0, 0)
	e := NewEngine(chain, store, contextmgr.Config{})

	// Standalone call starts its own transaction.
	if err := e.CompressStep(context.Background(), 1, 1); err != nil {
		t.Fatalf("CompressStep error: %v", err)
	}
	l1, err := store.GetL1(1)
	if err != nil {
		t.Fatalf("GetL1 error: %v", err)
	}
	if l1.CompactionID == "" {
		t.Errorf("standalone CompressStep must set a CompactionID")
	}

	// In-batch calls share the transaction id.
	if err := e.CompressStepInBatch(context.Background(), 2, 1, "batch-42"); err != nil {
		t.Fatalf("CompressStepInBatch error: %v", err)
	}
	l1b, _ := store.GetL1(2)
	if l1b.CompactionID != "batch-42" {
		t.Errorf("CompactionID = %q, want %q", l1b.CompactionID, "batch-42")
	}
}

func TestCompressStepInBatch_WritesL2WithCompactionID(t *testing.T) {
	store := newMockStepStore()
	store.addStep(contextmgr.StepRecord{StepID: 1, Type: "tool", Role: "tool", CallID: "c1", Content: "some content", TokenCount: 100, ToolName: "bash"})
	chain := NewCompressModelChain(&mockCompressClient{available: true, compressFn: func(ctx context.Context, level int, prompt string) (string, error) {
		return "[掩码 step_1|原100t|tool|p]\nfacts", nil
	}}, nil, 0, 0)
	e := NewEngine(chain, store, contextmgr.Config{})
	if err := e.CompressStepInBatch(context.Background(), 1, 2, "batch-9"); err != nil {
		t.Fatalf("CompressStepInBatch error: %v", err)
	}
	l2, err := store.GetL2(1)
	if err != nil {
		t.Fatalf("GetL2 error: %v", err)
	}
	if l2.CompactionID != "batch-9" {
		t.Errorf("L2 CompactionID = %q, want %q", l2.CompactionID, "batch-9")
	}
}

func TestCapSummary_TruncatesOverBudget(t *testing.T) {
	long := strings.Repeat("x", 100)
	got := capSummary(long, 5) // 5 tokens ≈ 20 bytes
	if len(got) >= len(long) {
		t.Errorf("expected truncation, got %d bytes", len(got))
	}
	if !strings.Contains(got, "[truncated]") {
		t.Errorf("expected truncation marker, got %q", got)
	}
}

func TestCapSummary_KeepsWithinBudget(t *testing.T) {
	short := "short"
	if got := capSummary(short, 8192); got != short {
		t.Errorf("small summaries must pass through, got %q", got)
	}
	if got := capSummary(short, 0); got != short {
		t.Errorf("zero budget disables truncation, got %q", got)
	}
}

func TestCompressStepInBatch_AppliesMaxSummaryTokens(t *testing.T) {
	store := newMockStepStore()
	store.addStep(contextmgr.StepRecord{StepID: 1, Type: "tool", Role: "tool", CallID: "c1", Content: "content", TokenCount: 100, ToolName: "bash"})
	chain := NewCompressModelChain(&mockCompressClient{available: true, compressFn: func(ctx context.Context, level int, prompt string) (string, error) {
		return strings.Repeat("z", 500), nil
	}}, nil, 0, 0)
	e := NewEngine(chain, store, contextmgr.Config{MaxSummaryTokens: 10}) // 10 tokens ≈ 40 bytes
	if err := e.CompressStepInBatch(context.Background(), 1, 1, "b"); err != nil {
		t.Fatalf("CompressStepInBatch error: %v", err)
	}
	l1, _ := store.GetL1(1)
	if len(l1.Content) > 40+len("\n...[truncated]") {
		t.Errorf("L1 content not capped: %d bytes", len(l1.Content))
	}
}
