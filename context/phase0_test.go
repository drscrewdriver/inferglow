package contextmgr

import (
	"context"
	"sort"
	"testing"
)

type fakeStore struct {
	steps   map[int]StepRecord
	refs    map[int]RefRecord
	l1      map[int]L1Record
	l2      map[int]L2Record
	l3      map[int]L3Record
	longmem map[string]LongMemRecord
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		steps: make(map[int]StepRecord), refs: make(map[int]RefRecord),
		l1: make(map[int]L1Record), l2: make(map[int]L2Record),
		l3: make(map[int]L3Record), longmem: make(map[string]LongMemRecord),
	}
}

func (f *fakeStore) AppendStep(step StepRecord) error { f.steps[step.StepID] = step; return nil }
func (f *fakeStore) GetStep(id int) (*StepRecord, error) {
	s, ok := f.steps[id]; if !ok { return nil, &sErr{id} }; return &s, nil
}
func (f *fakeStore) RangeSteps(from, to int) ([]StepRecord, error) {
	var out []StepRecord
	for i := from; i <= to; i++ { if s, ok := f.steps[i]; ok { out = append(out, s) } }
	return out, nil
}
func (f *fakeStore) UpsertRef(ref RefRecord) error { f.refs[ref.StepID] = ref; return nil }
func (f *fakeStore) GetRef(id int) (*RefRecord, error) {
	r, ok := f.refs[id]; if !ok { return nil, &sErr{id} }; return &r, nil
}
func (f *fakeStore) AllActiveStepIDs() ([]int, error) {
	var ids []int; for id := range f.refs { ids = append(ids, id) }; sort.Ints(ids); return ids, nil
}
func (f *fakeStore) RemoveRef(id int) error { delete(f.refs, id); return nil }
func (f *fakeStore) AppendL1(r L1Record) error { f.l1[r.StepID] = r; return nil }
func (f *fakeStore) GetL1(id int) (*L1Record, error) {
	r, ok := f.l1[id]; if !ok { return nil, &sErr{id} }; return &r, nil
}
func (f *fakeStore) AppendL2(r L2Record) error { f.l2[r.StepID] = r; return nil }
func (f *fakeStore) GetL2(id int) (*L2Record, error) {
	r, ok := f.l2[id]; if !ok { return nil, &sErr{id} }; return &r, nil
}
func (f *fakeStore) HotFacts(int, float64) ([]L2Record, error) { return nil, nil }
func (f *fakeStore) AppendL3(r L3Record) error { f.l3[r.StepID] = r; return nil }
func (f *fakeStore) GetL3(id int) (*L3Record, error) {
	r, ok := f.l3[id]; if !ok { return nil, &sErr{id} }; return &r, nil
}
func (f *fakeStore) UpsertLongMem(m LongMemRecord) error { f.longmem[m.MemID] = m; return nil }
func (f *fakeStore) GetLongMem(id string) (*LongMemRecord, error) {
	r, ok := f.longmem[id]; if !ok { return nil, &sErr{0} }; return &r, nil
}
func (f *fakeStore) SearchLongMem(string, string, int) ([]LongMemRecord, error) { return nil, nil }
func (f *fakeStore) RemoveLongMem(string) error { return nil }
func (f *fakeStore) AppendAudit(AuditRecord) error { return nil }
func (f *fakeStore) Close() error { return nil }

type sErr struct{ id int }
func (e *sErr) Error() string { return "not found" }

func TestDefaultConfig_SweetSpot(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.SweetSpotTokens != 0 { t.Errorf("want 0, got %d", cfg.SweetSpotTokens) }
	if cfg.WarmupRatio != 0.8 { t.Errorf("want 0.8, got %f", cfg.WarmupRatio) }
}

func TestSweetSpotPassthrough(t *testing.T) {
	store := newFakeStore()
	cfg := DefaultConfig()
	cfg.SweetSpotTokens = 100000
	cfg.TailKeepSteps = 3
	mgr, _ := NewHybridManager(cfg, store)
	h := mgr.(*HybridManager)
	_ = h.Ingest(StepRecord{StepID: 1, Type: "reasoning", TokenCount: 10})
	_ = h.Ingest(StepRecord{StepID: 2, Type: "tool", TokenCount: 20})
	ref, _ := store.GetRef(1)
	if ref.Level != 0 { t.Errorf("want L0, got L%d", ref.Level) }
}

func TestConstitutionalZone(t *testing.T) {
	store := newFakeStore()
	cfg := DefaultConfig()
	mgr, _ := NewHybridManager(cfg, store)
	h := mgr.(*HybridManager)
	h.AppendConstitutional([]string{"no rm", "use Chinese"})
	if len(h.GetConstitutionalContent()) != 2 { t.Fatal("want 2") }
	blocks, _ := h.BuildContext(context.Background(), 128000)
	found := false
	for _, b := range blocks { if b.StepID == -3 { found = true } }
	if !found { t.Error("missing Zone 0.5") }
}

func TestRewriteHeadBuffer(t *testing.T) {
	store := newFakeStore()
	cfg := DefaultConfig()
	mgr, _ := NewHybridManager(cfg, store)
	h := mgr.(*HybridManager)
	h.SetHeadBuffer([]RenderedBlock{{Content: "v1"}}, "v1")
	h.RewriteHeadBuffer([]RenderedBlock{{Content: "v2"}}, "v2")
	if a := h.GetArchivedHeads(); len(a) != 1 || a[0].Version != "v1" { t.Error("archive fail") }
}

func TestParseMergedResponse(t *testing.T) {
	d, err := parseMergedResponse(`{"q1_constitutional_append":["x"],"q2_new_head_summary":"s","q3_step_decisions":[{"step_id":1,"target_level":0,"reason":"ok"}]}`)
	if err != nil { t.Fatal(err) }
	if len(d.ConstitutionalAppend) != 1 || len(d.StepDecisions) != 1 { t.Error("parse fail") }
}

func TestParseMergedResponse_Fences(t *testing.T) {
	_, err := parseMergedResponse("```json\n{\"q1_constitutional_append\":[],\"q2_new_head_summary\":\"\",\"q3_step_decisions\":[]}\n```")
	if err != nil { t.Fatal(err) }
}

func TestSystemPromptHint(t *testing.T) {
	if SystemPromptHint() == "" { t.Error("empty") }
}

func TestWarmupCompress(t *testing.T) {
	store := newFakeStore()
	cfg := DefaultConfig()
	cfg.SweetSpotTokens = 100
	cfg.TailKeepSteps = 2
	mgr, _ := NewHybridManager(cfg, store)
	h := mgr.(*HybridManager)
	for i := 1; i <= 5; i++ {
		store.steps[i] = StepRecord{StepID: i, TokenCount: 15}
		store.refs[i] = RefRecord{StepID: i, Level: 0, Strength: 1.0}
	}
	h.currentStep = 5
	h.warmupCompress()
	for _, id := range []int{1, 2, 3} {
		r, _ := store.GetRef(id)
		if r.Level != 1 { t.Errorf("step %d: want L1, got L%d", id, r.Level) }
	}
	for _, id := range []int{4, 5} {
		r, _ := store.GetRef(id)
		if r.Level != 0 { t.Errorf("step %d: want L0, got L%d", id, r.Level) }
	}
}

var _ interface {
	Reorganize(ctx context.Context, engine CompressEngine, focus string) (*ReorganizeResult, error)
} = (*HybridManager)(nil)
