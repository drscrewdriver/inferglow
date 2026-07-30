package contextmgr

import (
	"context"
	"strings"
	"testing"
)

// --- A-9 Backtrack tests ---

func TestBacktrackBlockSameGroup(t *testing.T) {
	store := newFakeStore()
	lastRef1, lastRef2 := 2, 3
	store.steps[1] = StepRecord{StepID: 1, Type: "reasoning", Content: "step one content"}
	store.steps[2] = StepRecord{StepID: 2, Type: "tool", Content: "step two content"}
	store.steps[3] = StepRecord{StepID: 3, Type: "reasoning", Content: "step three content"}
	store.refs[1] = RefRecord{StepID: 1, Level: 0, Strength: 1.0, TaskGroupID: 1, LastRefAtStep: &lastRef1}
	store.refs[2] = RefRecord{StepID: 2, Level: 0, Strength: 2.0, TaskGroupID: 1, LastRefAtStep: &lastRef2}
	store.refs[3] = RefRecord{StepID: 3, Level: 0, Strength: 1.5, TaskGroupID: 1}

	cfg := DefaultConfig()
	cfg.TailKeepSteps = 0
	cfg.LongMem.Enabled = false
	cfg.Backtrack.Enabled = true
	cfg.Backtrack.TopK = 2
	mgr, _ := NewHybridManager(cfg, store)
	h := mgr.(*HybridManager)

	blocks, err := h.BuildContext(context.Background(), 128000)
	if err != nil {
		t.Fatal(err)
	}

	var bt *RenderedBlock
	for i := range blocks {
		if blocks[i].StepID == -5 {
			bt = &blocks[i]
			break
		}
	}
	if bt == nil {
		t.Fatal("expected backtrack block (StepID -5)")
	}
	if !strings.Contains(bt.Content, "[backtrack | group #1 | top-2]") {
		t.Errorf("expected top-2 header; got:\n%s", bt.Content)
	}
	// Top-K = 2, so only 2 entries.
	if strings.Count(bt.Content, "(§") != 2 {
		t.Errorf("expected exactly 2 entries; got:\n%s", bt.Content)
	}
}

func TestBacktrackBlockDifferentGroup(t *testing.T) {
	store := newFakeStore()
	store.steps[1] = StepRecord{StepID: 1, Type: "reasoning", Content: "a"}
	store.steps[2] = StepRecord{StepID: 2, Type: "reasoning", Content: "b"}
	store.refs[1] = RefRecord{StepID: 1, Level: 0, Strength: 1.0, TaskGroupID: 1}
	store.refs[2] = RefRecord{StepID: 2, Level: 0, Strength: 1.0, TaskGroupID: 2} // different group

	cfg := DefaultConfig()
	cfg.TailKeepSteps = 0
	cfg.LongMem.Enabled = false
	cfg.Backtrack.Enabled = true
	mgr, _ := NewHybridManager(cfg, store)
	h := mgr.(*HybridManager)

	blocks, _ := h.BuildContext(context.Background(), 128000)
	for _, b := range blocks {
		if b.StepID == -5 {
			t.Error("backtrack block must NOT appear when last two steps are different groups")
		}
	}
}

func TestBacktrackDisabled(t *testing.T) {
	store := newFakeStore()
	store.steps[1] = StepRecord{StepID: 1, Type: "reasoning", Content: "a"}
	store.steps[2] = StepRecord{StepID: 2, Type: "reasoning", Content: "b"}
	store.refs[1] = RefRecord{StepID: 1, Level: 0, Strength: 1.0, TaskGroupID: 1}
	store.refs[2] = RefRecord{StepID: 2, Level: 0, Strength: 1.0, TaskGroupID: 1}

	cfg := DefaultConfig()
	cfg.TailKeepSteps = 0
	cfg.LongMem.Enabled = false
	cfg.Backtrack.Enabled = false // disabled
	mgr, _ := NewHybridManager(cfg, store)
	h := mgr.(*HybridManager)

	blocks, _ := h.BuildContext(context.Background(), 128000)
	for _, b := range blocks {
		if b.StepID == -5 {
			t.Error("backtrack block must NOT appear when disabled")
		}
	}
}

func TestBacktrackSkipsTransient(t *testing.T) {
	store := newFakeStore()
	store.steps[1] = StepRecord{StepID: 1, Type: "tool", Content: "transient content", Transient: true}
	store.steps[2] = StepRecord{StepID: 2, Type: "reasoning", Content: "real content"}
	store.steps[3] = StepRecord{StepID: 3, Type: "reasoning", Content: "another"}
	store.refs[1] = RefRecord{StepID: 1, Level: 0, Strength: 5.0, TaskGroupID: 1}
	store.refs[2] = RefRecord{StepID: 2, Level: 0, Strength: 1.0, TaskGroupID: 1}
	store.refs[3] = RefRecord{StepID: 3, Level: 0, Strength: 1.0, TaskGroupID: 1}

	cfg := DefaultConfig()
	cfg.TailKeepSteps = 0
	cfg.LongMem.Enabled = false
	cfg.Backtrack.Enabled = true
	cfg.Backtrack.TopK = 5
	mgr, _ := NewHybridManager(cfg, store)
	h := mgr.(*HybridManager)

	blocks, _ := h.BuildContext(context.Background(), 128000)
	for _, b := range blocks {
		if b.StepID == -5 {
			if strings.Contains(b.Content, "transient content") {
				t.Error("backtrack must NOT include transient step content")
			}
			return
		}
	}
	t.Error("expected backtrack block")
}

// --- A-3 Rebackground / version chain tests ---

func TestGetLayerVersion(t *testing.T) {
	store := newFakeStore()
	cfg := DefaultConfig()
	mgr, _ := NewHybridManager(cfg, store)
	h := mgr.(*HybridManager)

	// Set initial head, then rewrite twice.
	h.SetHeadBuffer([]RenderedBlock{{Content: "v1"}}, "v1")
	h.RewriteHeadBuffer([]RenderedBlock{{Content: "v2"}}, "v2")
	h.RewriteHeadBuffer([]RenderedBlock{{Content: "v3"}}, "v3")

	// Seq 1 = first archived (v1), Seq 2 = second archived (v2).
	ah, ok := h.GetLayerVersion(4, 1)
	if !ok {
		t.Fatal("expected version seq=1")
	}
	if ah.Version != "v1" || ah.Content[0].Content != "v1" {
		t.Errorf("seq=1 should be v1; got version=%q content=%q", ah.Version, ah.Content[0].Content)
	}

	ah2, ok := h.GetLayerVersion(4, 2)
	if !ok {
		t.Fatal("expected version seq=2")
	}
	if ah2.Version != "v2" {
		t.Errorf("seq=2 should be v2; got %q", ah2.Version)
	}

	// Non-L4 layer returns false.
	if _, ok := h.GetLayerVersion(5, 1); ok {
		t.Error("layer 5 should return false")
	}
}

func TestRebackgroundNarrow(t *testing.T) {
	store := newFakeStore()
	cfg := DefaultConfig()
	mgr, _ := NewHybridManager(cfg, store)
	h := mgr.(*HybridManager)

	h.SetHeadBuffer([]RenderedBlock{{Content: "old bg"}}, "init")
	h.AppendConstitutional([]string{"rule1"})

	// Rebackground with prohibition change.
	h.Rebackground(RebackgroundRequest{
		NewTaskDescription:     "new task background",
		NewProhibitions:        []string{"no rm", "use Chinese"},
		CheckProhibitionChange: true,
	})

	// L4 updated.
	heads := h.HeadBlocks()
	if len(heads) != 1 || heads[0].Content != "new task background" {
		t.Errorf("expected new background; got %v", heads)
	}

	// L5 updated.
	entries := h.GetConstitutionalContent()
	if len(entries) != 2 || entries[0] != "no rm" {
		t.Errorf("expected 2 prohibitions; got %v", entries)
	}

	// Archived heads grew (old "init" version archived).
	archived := h.GetArchivedHeads()
	if len(archived) < 1 {
		t.Error("expected at least 1 archived head after Rebackground")
	}

	// Rebackground without prohibition change.
	h.Rebackground(RebackgroundRequest{
		NewTaskDescription:     "another bg",
		CheckProhibitionChange: false,
	})
	entries2 := h.GetConstitutionalContent()
	if len(entries2) != 2 {
		t.Errorf("prohibitions should NOT change when CheckProhibitionChange=false; got %v", entries2)
	}
}

// TestRebackgroundEmptyDescriptionKeepsHead verifies B2: an empty task
// description must NOT clobber the existing L4 head buffer nor archive a new
// version, while L5 prohibition changes still apply.
func TestRebackgroundEmptyDescriptionKeepsHead(t *testing.T) {
	store := newFakeStore()
	cfg := DefaultConfig()
	mgr, _ := NewHybridManager(cfg, store)
	h := mgr.(*HybridManager)

	h.SetHeadBuffer([]RenderedBlock{{Content: "original L4"}}, "init")

	// Empty description + prohibition change.
	h.Rebackground(RebackgroundRequest{
		NewTaskDescription:     "",
		NewProhibitions:        []string{"no rm"},
		CheckProhibitionChange: true,
	})

	// L4 must be preserved as-is.
	heads := h.HeadBlocks()
	if len(heads) != 1 || heads[0].Content != "original L4" {
		t.Errorf("empty description must not overwrite L4; got %v", heads)
	}

	// No new archive version should be created for an empty rewrite.
	if a := h.GetArchivedHeads(); len(a) != 0 {
		t.Errorf("empty description must not archive a version; got %d", len(a))
	}

	// L5 prohibition change still applies.
	entries := h.GetConstitutionalContent()
	if len(entries) != 1 || entries[0] != "no rm" {
		t.Errorf("L5 prohibition change should still apply; got %v", entries)
	}
}
