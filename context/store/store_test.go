package store

import (
	"sort"
	"sync"
	"testing"

	contextmgr "github.com/inferglow/context"
)

// mockStepStore implements StepStore for testing.
type mockStepStore struct {
	mu      sync.Mutex
	steps   map[int]contextmgr.StepRecord
	refs    map[int]contextmgr.RefRecord
	l1      map[int]contextmgr.L1Record
	l2      map[int]contextmgr.L2Record
	l3      map[int]contextmgr.L3Record
	longmem map[string]contextmgr.LongMemRecord
	audits  []contextmgr.AuditRecord
}

func newMockStepStore() *mockStepStore {
	return &mockStepStore{
		steps:   make(map[int]contextmgr.StepRecord),
		refs:    make(map[int]contextmgr.RefRecord),
		l1:      make(map[int]contextmgr.L1Record),
		l2:      make(map[int]contextmgr.L2Record),
		l3:      make(map[int]contextmgr.L3Record),
		longmem: make(map[string]contextmgr.LongMemRecord),
	}
}

func (m *mockStepStore) AppendStep(step contextmgr.StepRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.steps[step.StepID] = step
	return nil
}

func (m *mockStepStore) GetStep(stepID int) (*contextmgr.StepRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.steps[stepID]
	if !ok {
		return nil, nil
	}
	return &s, nil
}

func (m *mockStepStore) RangeSteps(from, to int) ([]contextmgr.StepRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []contextmgr.StepRecord
	for i := from; i <= to; i++ {
		if s, ok := m.steps[i]; ok {
			out = append(out, s)
		}
	}
	return out, nil
}

func (m *mockStepStore) UpsertRef(ref contextmgr.RefRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.refs[ref.StepID] = ref
	return nil
}

func (m *mockStepStore) GetRef(stepID int) (*contextmgr.RefRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.refs[stepID]
	if !ok {
		return nil, nil
	}
	return &r, nil
}

func (m *mockStepStore) AllActiveStepIDs() ([]int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var ids []int
	for id := range m.refs {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids, nil
}

func (m *mockStepStore) RemoveRef(stepID int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.refs, stepID)
	return nil
}

func (m *mockStepStore) AppendL1(rec contextmgr.L1Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.l1[rec.StepID] = rec
	return nil
}

func (m *mockStepStore) GetL1(stepID int) (*contextmgr.L1Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.l1[stepID]
	if !ok {
		return nil, nil
	}
	return &r, nil
}

func (m *mockStepStore) AppendL2(rec contextmgr.L2Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.l2[rec.StepID] = rec
	return nil
}

func (m *mockStepStore) GetL2(stepID int) (*contextmgr.L2Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.l2[stepID]
	if !ok {
		return nil, nil
	}
	return &r, nil
}

func (m *mockStepStore) HotFacts(minRefCount int, minStrength float64) ([]contextmgr.L2Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []contextmgr.L2Record
	for _, rec := range m.l2 {
		ref, ok := m.refs[rec.StepID]
		if ok && ref.RefCount >= minRefCount && ref.Strength >= minStrength {
			out = append(out, rec)
		}
	}
	return out, nil
}

func (m *mockStepStore) AppendL3(rec contextmgr.L3Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.l3[rec.StepID] = rec
	return nil
}

func (m *mockStepStore) GetL3(stepID int) (*contextmgr.L3Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.l3[stepID]
	if !ok {
		return nil, nil
	}
	return &r, nil
}

func (m *mockStepStore) UpsertLongMem(mem contextmgr.LongMemRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.longmem[mem.MemID] = mem
	return nil
}

func (m *mockStepStore) GetLongMem(memID string) (*contextmgr.LongMemRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.longmem[memID]
	if !ok {
		return nil, nil
	}
	return &r, nil
}

func (m *mockStepStore) SearchLongMem(query string, category string, limit int) ([]contextmgr.LongMemRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []contextmgr.LongMemRecord
	for _, mem := range m.longmem {
		if category != "" && mem.Category != category {
			continue
		}
		out = append(out, mem)
	}
	return out, nil
}

func (m *mockStepStore) RemoveLongMem(memID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.longmem, memID)
	return nil
}

func (m *mockStepStore) AppendAudit(rec contextmgr.AuditRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.audits = append(m.audits, rec)
	return nil
}

func (m *mockStepStore) Close() error {
	return nil
}

// --- Tests ---

func TestMockStore_AppendStep(t *testing.T) {
	m := newMockStepStore()
	step := contextmgr.StepRecord{
		StepID: 1, Type: "reasoning", Role: "assistant",
		Content: "test content", TokenCount: 10,
	}
	if err := m.AppendStep(step); err != nil {
		t.Fatalf("AppendStep returned error: %v", err)
	}
	got, err := m.GetStep(1)
	if err != nil {
		t.Fatalf("GetStep returned error: %v", err)
	}
	if got == nil {
		t.Fatal("GetStep returned nil")
	}
	if got.StepID != 1 || got.Content != "test content" {
		t.Errorf("unexpected step: %+v", got)
	}
}

func TestMockStore_GetStep(t *testing.T) {
	m := newMockStepStore()
	step := contextmgr.StepRecord{
		StepID: 42, Type: "tool", Role: "tool",
		Content: "tool output", TokenCount: 5,
		ToolName: "search", KeyParams: "q=test",
	}
	if err := m.AppendStep(step); err != nil {
		t.Fatal(err)
	}
	got, err := m.GetStep(42)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("GetStep returned nil")
	}
	if got.StepID != 42 || got.Type != "tool" || got.Role != "tool" {
		t.Errorf("field mismatch: %+v", got)
	}
	if got.Content != "tool output" || got.TokenCount != 5 {
		t.Errorf("content mismatch: %+v", got)
	}
	if got.ToolName != "search" || got.KeyParams != "q=test" {
		t.Errorf("tool info mismatch: %+v", got)
	}
}

func TestMockStore_GetStep_NotFound(t *testing.T) {
	m := newMockStepStore()
	got, err := m.GetStep(999)
	if err != nil {
		t.Fatalf("GetStep for non-existent ID returned error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for non-existent step, got %+v", got)
	}
}

func TestMockStore_RangeSteps(t *testing.T) {
	m := newMockStepStore()
	steps := []contextmgr.StepRecord{
		{StepID: 1, Content: "step one"},
		{StepID: 2, Content: "step two"},
		{StepID: 3, Content: "step three"},
		{StepID: 5, Content: "step five"},
	}
	for _, s := range steps {
		if err := m.AppendStep(s); err != nil {
			t.Fatal(err)
		}
	}

	// Range [1, 3] should return steps 1, 2, 3 in order.
	got, err := m.RangeSteps(1, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(got))
	}
	for i, s := range got {
		if s.StepID != i+1 {
			t.Errorf("position %d: expected step %d, got %d", i, i+1, s.StepID)
		}
	}

	// Range [1, 5] should return 4 steps (step 4 doesn't exist).
	got, err = m.RangeSteps(1, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("expected 4 steps, got %d", len(got))
	}
}

func TestMockStore_AppendRefs(t *testing.T) {
	m := newMockStepStore()

	// Upsert refs.
	ref1 := contextmgr.RefRecord{StepID: 1, Level: 0, RefCount: 3, Strength: 1.5}
	ref2 := contextmgr.RefRecord{StepID: 2, Level: 1, RefCount: 1, Strength: 1.0}
	if err := m.UpsertRef(ref1); err != nil {
		t.Fatal(err)
	}
	if err := m.UpsertRef(ref2); err != nil {
		t.Fatal(err)
	}

	// GetRef should return the correct record.
	got, err := m.GetRef(1)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("GetRef returned nil")
	}
	if got.StepID != 1 || got.Level != 0 || got.RefCount != 3 {
		t.Errorf("ref1 mismatch: %+v", got)
	}

	// AllActiveStepIDs should return [1, 2].
	ids, err := m.AllActiveStepIDs()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || ids[0] != 1 || ids[1] != 2 {
		t.Errorf("expected [1, 2], got %v", ids)
	}

	// RemoveRef should remove the ref.
	if err := m.RemoveRef(1); err != nil {
		t.Fatal(err)
	}
	ids, err = m.AllActiveStepIDs()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != 2 {
		t.Errorf("expected [2], got %v", ids)
	}
}
