package contextmgr

import "testing"

func TestClassifyBlock(t *testing.T) {
	headIDs := map[int]bool{0: true} // head buffer blocks have StepID 0

	tests := []struct {
		block RenderedBlock
		want  LayerID
	}{
		{RenderedBlock{StepID: 0, Content: "head"}, LayerTaskBackground},   // head buffer
		{RenderedBlock{StepID: -3, Content: "const"}, LayerProhibitions},   // constitutional
		{RenderedBlock{StepID: -4, Content: "hot"}, LayerLTMFacts},         // hot facts
		{RenderedBlock{StepID: -2, Content: "long"}, LayerLTMFacts},        // longmem
		{RenderedBlock{StepID: -5, Content: "bt"}, LayerHotIndex},          // backtrack
		{RenderedBlock{StepID: -1, Content: "hint"}, LayerHintBlock},       // hint
		{RenderedBlock{StepID: 1, Content: "step"}, LayerCompressedHist},   // positive = history
		{RenderedBlock{StepID: 42, Content: "s"}, LayerCompressedHist},     // positive = history
	}

	for _, tt := range tests {
		got := classifyBlock(tt.block, headIDs)
		if got != tt.want {
			t.Errorf("classifyBlock(StepID=%d) = %d, want %d", tt.block.StepID, got, tt.want)
		}
	}
}

func TestGroupIntoLayers(t *testing.T) {
	headIDs := map[int]bool{0: true}
	blocks := []RenderedBlock{
		{StepID: 0, Level: 0, Content: "task background"},
		{StepID: -3, Level: 0, Content: "prohibitions"},
		{StepID: -4, Level: 2, Content: "hot facts"},
		{StepID: 1, Level: 0, Content: "step one"},
		{StepID: 2, Level: 1, Content: "step two"},
		{StepID: -5, Level: 0, Content: "backtrack"},
		{StepID: -1, Level: 0, Content: "hint"},
	}

	layers := groupIntoLayers(blocks, headIDs)

	// Should produce layers: L4, L5, L6, L7, L8, L9 (6 layers).
	if len(layers) != 6 {
		t.Fatalf("expected 6 layers, got %d: %+v", len(layers), layers)
	}

	// Verify layer IDs present.
	ids := make(map[LayerID]bool)
	for _, l := range layers {
		ids[l.ID] = true
		if l.Sha256 == "" {
			t.Errorf("layer %d has empty Sha256", l.ID)
		}
	}
	for _, expected := range []LayerID{LayerTaskBackground, LayerProhibitions, LayerLTMFacts, LayerCompressedHist, LayerHotIndex, LayerHintBlock} {
		if !ids[expected] {
			t.Errorf("missing layer %d", expected)
		}
	}

	// L7 should contain both steps.
	for _, l := range layers {
		if l.ID == LayerCompressedHist {
			if l.Content != "step one\nstep two" {
				t.Errorf("L7 content = %q", l.Content)
			}
		}
	}

	// Stability: L4, L5 stable; L6+ not.
	for _, l := range layers {
		if l.ID <= 5 && !l.Stable {
			t.Errorf("layer %d should be stable", l.ID)
		}
		if l.ID >= 6 && l.Stable {
			t.Errorf("layer %d should NOT be stable", l.ID)
		}
	}
}

func TestGroupIntoLayersIndependence(t *testing.T) {
	headIDs := map[int]bool{}
	blocks1 := []RenderedBlock{
		{StepID: -1, Level: 0, Content: "hint A"},
	}
	blocks2 := []RenderedBlock{
		{StepID: -1, Level: 0, Content: "hint B"},
		{StepID: 1, Level: 0, Content: "step"},
	}

	l1 := groupIntoLayers(blocks1, headIDs)
	l2 := groupIntoLayers(blocks2, headIDs)

	// Modifying blocks2 doesn't affect l1.
	if len(l1) != 1 || l1[0].Content != "hint A" {
		t.Errorf("l1 should be independent: %+v", l1)
	}
	if len(l2) != 2 {
		t.Errorf("l2 should have 2 layers: %+v", l2)
	}
}
