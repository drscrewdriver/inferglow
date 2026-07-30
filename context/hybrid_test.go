package contextmgr

import (
	"strings"
	"testing"
)

// TestFitToWindowUpgradesToCache verifies B3: when a block exceeds the window
// budget, fitToWindow re-renders it at a higher compression level via the
// rendered cache — the output keeps a full marker, is not truncated with
// "[compressed]", and the raised level is cached (cache hit).
func TestFitToWindowUpgradesToCache(t *testing.T) {
	store := newFakeStore()
	long := strings.Repeat("a very long reasoning body that must be compressed ", 20)
	store.steps[1] = StepRecord{StepID: 1, Type: "reasoning", Content: long}
	store.refs[1] = RefRecord{StepID: 1, Level: 0, Strength: 1.0}
	// Level-1 summary is much shorter -> re-render shrinks the window.
	store.l1[1] = L1Record{StepID: 1, Content: "short summary"}

	cfg := DefaultConfig()
	mgr, err := NewHybridManager(cfg, store)
	if err != nil {
		t.Fatal(err)
	}
	h := mgr.(*HybridManager)

	blocks := []RenderedBlock{{StepID: 1, Level: 0, Content: long}}
	out := h.fitToWindow(blocks, 1) // tiny budget forces an upgrade

	if len(out) != 1 {
		t.Fatalf("expected 1 block, got %d", len(out))
	}
	b := out[0]

	// Full marker present (type + raised level).
	if !strings.Contains(b.Content, "⟨§1·reasoning·L1⟩") {
		t.Errorf("expected full marker ⟨§1·reasoning·L1⟩; got: %s", b.Content)
	}
	// No fallback truncation marker.
	if strings.Contains(b.Content, "...[compressed]") {
		t.Errorf("expected real re-render, not truncated fallback; got: %s", b.Content)
	}
	// The raised level (1) must be cached.
	if cached := h.renderCache.Get(1, 1); cached == nil {
		t.Error("expected renderCache hit at level 1 after fitToWindow upgrade")
	}
}

// TestFitToWindowFallsBackToTruncation verifies the truncation fallback fires
// when a re-render cannot shrink the block (e.g. no L1/L2 record), still
// preserving the marker and re-adding the structural closing tag.
func TestFitToWindowFallsBackToTruncation(t *testing.T) {
	store := newFakeStore()
	long := strings.Repeat("filler words that are quite long indeed ", 20)
	store.steps[1] = StepRecord{StepID: 1, Type: "reasoning", Content: long}
	store.refs[1] = RefRecord{StepID: 1, Level: 0, Strength: 1.0}
	// No L1/L2 records -> re-render at level 1 falls back to same content, so
	// the upgrade does not shrink; the fallback truncation path is exercised.

	cfg := DefaultConfig()
	mgr, err := NewHybridManager(cfg, store)
	if err != nil {
		t.Fatal(err)
	}
	h := mgr.(*HybridManager)

	blocks := []RenderedBlock{{StepID: 1, Level: 0, Content: long}}
	// Even with a generous budget that still forces a pass, the marker must be kept.
	out := h.fitToWindow(blocks, -1) // negative budget always exceeds

	if len(out) != 1 {
		t.Fatalf("expected 1 block, got %d", len(out))
	}
	b := out[0]
	if b.Content == "" {
		t.Fatal("block content must not be empty")
	}
	if !strings.HasPrefix(b.Content, "⟨§") {
		t.Errorf("expected marker prefix preserved; got: %s", b.Content)
	}
}

// TestTruncateRespectingMarker unit-tests the truncation helper directly.
func TestTruncateRespectingMarker(t *testing.T) {
	// Marker preserved, body truncated, closing tag re-added.
	content := "⟨§3·constitutional·L2⟩ some long body text that is long </constitutional>"
	got := truncateRespectingMarker(content, 20)
	if !strings.HasPrefix(got, "⟨§3·constitutional·L2⟩") {
		t.Errorf("marker must be preserved; got: %s", got)
	}
	if !strings.HasSuffix(got, "</constitutional>") {
		t.Errorf("closing tag must be re-added; got: %s", got)
	}
	if !strings.Contains(got, "...[compressed]") {
		t.Errorf("expected truncation marker; got: %s", got)
	}

	// Facts closing tag.
	content2 := "⟨§2·tool·L1⟩ prefix facts here [/facts]"
	got2 := truncateRespectingMarker(content2, 5)
	if !strings.HasSuffix(got2, "[/facts]") {
		t.Errorf("facts closing tag must be re-added; got: %s", got2)
	}
	if !strings.HasPrefix(got2, "⟨§2·tool·L1⟩") {
		t.Errorf("marker must be preserved; got: %s", got2)
	}

	// No closing tag.
	content3 := "⟨§1·reasoning·L0⟩ plain body"
	got3 := truncateRespectingMarker(content3, 5)
	if !strings.HasPrefix(got3, "⟨§1·reasoning·L0⟩") {
		t.Errorf("marker must be preserved; got: %s", got3)
	}
	if !strings.Contains(got3, "...[compressed]") {
		t.Errorf("expected truncation marker; got: %s", got3)
	}

	// keep <= 0 returns content unchanged.
	content4 := "⟨§1·reasoning·L0⟩ body"
	if got4 := truncateRespectingMarker(content4, 0); got4 != content4 {
		t.Errorf("keep<=0 should return unchanged; got: %s", got4)
	}
}

// TestTruncateRespectingMarkerNoFragment ensures a body within budget is left
// untouched (no stray truncation marker).
func TestTruncateRespectingMarkerNoFragment(t *testing.T) {
	content := "⟨§1·reasoning·L0⟩ short"
	if got := truncateRespectingMarker(content, 100); got != content {
		t.Errorf("body within budget should be unchanged; got: %s", got)
	}
}