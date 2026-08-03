package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	contextmgr "github.com/inferglow/context"
)

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

// mockRegistry stores registered tools.
type mockRegistry struct {
	tools []Tool
}

func (r *mockRegistry) Register(t Tool) {
	r.tools = append(r.tools, t)
}

// mockContextManager implements contextmgr.ContextManager for testing.
type mockContextManager struct {
	mode        contextmgr.Mode
	searchHits  []contextmgr.SearchHit
	searchErr   error
	expandRes   *contextmgr.ExpandResult
	expandErr   error
	surroundRes []contextmgr.RenderedBlock
	surroundErr error
	stats       contextmgr.ContextStats
	longMemRes  []contextmgr.LongMemRecord
	longMemErr  error
}

func (m *mockContextManager) Mode() contextmgr.Mode                   { return m.mode }
func (m *mockContextManager) Ingest(step contextmgr.StepRecord) error { return nil }
func (m *mockContextManager) BuildContext(ctx context.Context, windowTokens int) ([]contextmgr.RenderedBlock, error) {
	return nil, nil
}
func (m *mockContextManager) TriggerCompression(ctx context.Context, opts contextmgr.CompressOpts) (*contextmgr.CompressResult, error) {
	return nil, nil
}
func (m *mockContextManager) Search(ctx context.Context, query contextmgr.SearchQuery) ([]contextmgr.SearchHit, error) {
	return m.searchHits, m.searchErr
}
func (m *mockContextManager) SearchLongMem(ctx context.Context, query string, category string, limit int) ([]contextmgr.LongMemRecord, error) {
	return m.longMemRes, m.longMemErr
}
func (m *mockContextManager) Expand(stepID int, full bool) (*contextmgr.ExpandResult, error) {
	return m.expandRes, m.expandErr
}
func (m *mockContextManager) Surround(stepID int, before, after int) ([]contextmgr.RenderedBlock, error) {
	return m.surroundRes, m.surroundErr
}
func (m *mockContextManager) Stats() contextmgr.ContextStats { return m.stats }
func (m *mockContextManager) Close() error                   { return nil }

// mockStepStore implements contextmgr.StepStoreLike for testing.
type mockStepStore struct {
	steps     map[int]contextmgr.StepRecord
	refs      map[int]contextmgr.RefRecord
	auditLogs []contextmgr.AuditRecord
	getRefErr error
	upsertErr error
	activeIDs []int
	activeErr error
}

func newMockStepStore() *mockStepStore {
	return &mockStepStore{
		steps: make(map[int]contextmgr.StepRecord),
		refs:  make(map[int]contextmgr.RefRecord),
	}
}

func (s *mockStepStore) AppendStep(step contextmgr.StepRecord) error { return nil }
func (s *mockStepStore) GetStep(stepID int) (*contextmgr.StepRecord, error) {
	if step, ok := s.steps[stepID]; ok {
		return &step, nil
	}
	return nil, nil
}
func (s *mockStepStore) RangeSteps(from, to int) ([]contextmgr.StepRecord, error) { return nil, nil }
func (s *mockStepStore) UpsertRef(ref contextmgr.RefRecord) error {
	if s.upsertErr != nil {
		return s.upsertErr
	}
	s.refs[ref.StepID] = ref
	return nil
}
func (s *mockStepStore) GetRef(stepID int) (*contextmgr.RefRecord, error) {
	if s.getRefErr != nil {
		return nil, s.getRefErr
	}
	if ref, ok := s.refs[stepID]; ok {
		return &ref, nil
	}
	return nil, nil
}
func (s *mockStepStore) AllActiveStepIDs() ([]int, error) {
	return s.activeIDs, s.activeErr
}
func (s *mockStepStore) RemoveRef(stepID int) error                     { return nil }
func (s *mockStepStore) AppendL1(rec contextmgr.L1Record) error         { return nil }
func (s *mockStepStore) GetL1(stepID int) (*contextmgr.L1Record, error) { return nil, nil }
func (s *mockStepStore) AppendL2(rec contextmgr.L2Record) error         { return nil }
func (s *mockStepStore) GetL2(stepID int) (*contextmgr.L2Record, error) { return nil, nil }
func (s *mockStepStore) HotFacts(minRefCount int, minStrength float64) ([]contextmgr.L2Record, error) {
	return nil, nil
}
func (s *mockStepStore) AppendL3(rec contextmgr.L3Record) error                     { return nil }
func (s *mockStepStore) GetL3(stepID int) (*contextmgr.L3Record, error)             { return nil, nil }
func (s *mockStepStore) UpsertLongMem(mem contextmgr.LongMemRecord) error           { return nil }
func (s *mockStepStore) GetLongMem(memID string) (*contextmgr.LongMemRecord, error) { return nil, nil }
func (s *mockStepStore) SearchLongMem(query string, category string, limit int) ([]contextmgr.LongMemRecord, error) {
	return nil, nil
}
func (s *mockStepStore) RemoveLongMem(memID string) error { return nil }
func (s *mockStepStore) AppendAudit(rec contextmgr.AuditRecord) error {
	s.auditLogs = append(s.auditLogs, rec)
	return nil
}
func (s *mockStepStore) Close() error { return nil }

// mockReorgProvider implements ReorganizeProvider.
type mockReorgProvider struct {
	reorgRes *contextmgr.ReorganizeResult
	reorgErr error
}

func (m *mockReorgProvider) Reorganize(ctx context.Context, engine contextmgr.CompressEngine, focus string) (*contextmgr.ReorganizeResult, error) {
	return m.reorgRes, m.reorgErr
}

// ---------------------------------------------------------------------------
// Tests: Tool metadata
// ---------------------------------------------------------------------------

func TestContextSearchTool_Metadata(t *testing.T) {
	tool := &ContextSearchTool{}
	if got := tool.Name(); got != "context_search" {
		t.Errorf("Name() = %q, want %q", got, "context_search")
	}
	if got := tool.Description(); got == "" {
		t.Error("Description() should not be empty")
	}
	if got := tool.InputSchema(); len(got) == 0 {
		t.Error("InputSchema() should not be empty")
	}
}

func TestContextExpandTool_Metadata(t *testing.T) {
	tool := &ContextExpandTool{}
	if got := tool.Name(); got != "context_expand" {
		t.Errorf("Name() = %q, want %q", got, "context_expand")
	}
	if got := tool.Description(); got == "" {
		t.Error("Description() should not be empty")
	}
}

func TestContextSurroundTool_Metadata(t *testing.T) {
	tool := &ContextSurroundTool{}
	if got := tool.Name(); got != "context_surround" {
		t.Errorf("Name() = %q, want %q", got, "context_surround")
	}
}

func TestMemorySearchTool_Metadata(t *testing.T) {
	tool := &MemorySearchTool{}
	if got := tool.Name(); got != "memory_search" {
		t.Errorf("Name() = %q, want %q", got, "memory_search")
	}
}

func TestContextLockL0Tool_Metadata(t *testing.T) {
	tool := &ContextLockL0Tool{}
	if got := tool.Name(); got != "context_lock_l0" {
		t.Errorf("Name() = %q, want %q", got, "context_lock_l0")
	}
}

func TestContextUnlockL0Tool_Metadata(t *testing.T) {
	tool := &ContextUnlockL0Tool{}
	if got := tool.Name(); got != "context_unlock_l0" {
		t.Errorf("Name() = %q, want %q", got, "context_unlock_l0")
	}
}

func TestContextTraceTool_Metadata(t *testing.T) {
	tool := &ContextTraceTool{}
	if got := tool.Name(); got != "context_trace" {
		t.Errorf("Name() = %q, want %q", got, "context_trace")
	}
}

func TestContextReorganizeTool_Metadata(t *testing.T) {
	tool := &ContextReorganizeTool{}
	if got := tool.Name(); got != "context_reorganize" {
		t.Errorf("Name() = %q, want %q", got, "context_reorganize")
	}
}

// ---------------------------------------------------------------------------
// Tests: RegisterContextTools / RegisterContextToolsWithStore
// ---------------------------------------------------------------------------

func TestRegisterContextTools_PassthroughMode(t *testing.T) {
	reg := &mockRegistry{}
	mgr := &mockContextManager{mode: contextmgr.ModePassthrough}
	RegisterContextTools(reg, mgr)
	if len(reg.tools) != 0 {
		t.Errorf("expected 0 tools in passthrough mode, got %d", len(reg.tools))
	}
}

func TestRegisterContextToolsWithStore_NoStore(t *testing.T) {
	reg := &mockRegistry{}
	mgr := &mockContextManager{mode: contextmgr.ModeHybrid}
	RegisterContextToolsWithStore(reg, mgr, nil)

	// Should register: context_search, context_expand, context_surround, memory_search
	// Without store, no trace/lock tools
	if len(reg.tools) < 3 {
		t.Errorf("expected at least 3 tools, got %d", len(reg.tools))
	}

	names := make(map[string]bool)
	for _, tl := range reg.tools {
		names[tl.Name()] = true
	}
	for _, want := range []string{"context_search", "context_expand", "context_surround", "memory_search"} {
		if !names[want] {
			t.Errorf("missing expected tool %q", want)
		}
	}
}

func TestRegisterContextToolsWithStore_WithStore(t *testing.T) {
	reg := &mockRegistry{}
	mgr := &mockContextManager{mode: contextmgr.ModeHybrid}
	store := newMockStepStore()
	RegisterContextToolsWithStore(reg, mgr, store)

	names := make(map[string]bool)
	for _, tl := range reg.tools {
		names[tl.Name()] = true
	}
	for _, want := range []string{"context_search", "context_expand", "context_surround",
		"memory_search", "context_trace", "context_lock_l0", "context_unlock_l0"} {
		if !names[want] {
			t.Errorf("missing expected tool %q", want)
		}
	}
}

func TestRegisterContextToolsWithStore_ReorganizeProvider(t *testing.T) {
	reg := &mockRegistry{}
	mgr := &mockContextManager{mode: contextmgr.ModeHybrid}
	store := newMockStepStore()

	// We need the manager to also implement ReorganizeProvider.
	// Since mockContextManager doesn't, we can test without it.
	RegisterContextToolsWithStore(reg, mgr, store)

	names := make(map[string]bool)
	for _, tl := range reg.tools {
		names[tl.Name()] = true
	}
	if names["context_reorganize"] {
		t.Error("context_reorganize should not be registered without ReorganizeProvider")
	}
}

// ---------------------------------------------------------------------------
// Tests: NewContextTools
// ---------------------------------------------------------------------------

func TestNewContextTools_Passthrough(t *testing.T) {
	mgr := &mockContextManager{mode: contextmgr.ModePassthrough}
	tools := NewContextTools(mgr, nil)
	if tools != nil {
		t.Errorf("expected nil in passthrough mode, got %d tools", len(tools))
	}
}

func TestNewContextTools_HybridNoStore(t *testing.T) {
	mgr := &mockContextManager{mode: contextmgr.ModeHybrid}
	tools := NewContextTools(mgr, nil)
	if len(tools) < 4 {
		t.Errorf("expected at least 4 tools, got %d", len(tools))
	}
}

func TestNewContextTools_HybridWithStore(t *testing.T) {
	mgr := &mockContextManager{mode: contextmgr.ModeHybrid}
	store := newMockStepStore()
	tools := NewContextTools(mgr, store)

	names := make(map[string]bool)
	for _, tl := range tools {
		names[tl.Name()] = true
	}
	for _, want := range []string{"context_search", "context_expand", "context_surround",
		"memory_search", "context_trace"} {
		if !names[want] {
			t.Errorf("missing expected tool %q", want)
		}
	}
}

// ---------------------------------------------------------------------------
// Tests: ContextSearchTool
// ---------------------------------------------------------------------------

func TestContextSearchTool_Execute(t *testing.T) {
	mgr := &mockContextManager{
		mode: contextmgr.ModeHybrid,
		searchHits: []contextmgr.SearchHit{
			{StepID: 1, Level: 0, Score: 0.95, Snippet: "test snippet", Type: "user"},
			{StepID: 2, Level: 1, Score: 0.80, Snippet: "another hit", Type: "tool"},
		},
	}
	tool := &ContextSearchTool{mgr: mgr}

	input := json.RawMessage(`{"query":"test query"}`)
	raw, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	var out ContextSearchOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if len(out.Hits) != 2 {
		t.Errorf("expected 2 hits, got %d", len(out.Hits))
	}
	if out.Hits[0].StepID != 1 {
		t.Errorf("expected StepID=1, got %d", out.Hits[0].StepID)
	}
}

func TestContextSearchTool_EmptyHits(t *testing.T) {
	mgr := &mockContextManager{
		mode:       contextmgr.ModeHybrid,
		searchHits: []contextmgr.SearchHit{},
	}
	tool := &ContextSearchTool{mgr: mgr}

	input := json.RawMessage(`{"query":"nothing"}`)
	raw, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	var hint map[string]string
	if err := json.Unmarshal(raw, &hint); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if hint["hint"] == "" {
		t.Error("expected hint for empty results")
	}
}

func TestContextSearchTool_InvalidInput(t *testing.T) {
	mgr := &mockContextManager{mode: contextmgr.ModeHybrid}
	tool := &ContextSearchTool{mgr: mgr}

	_, err := tool.Execute(context.Background(), json.RawMessage(`not json`))
	if err == nil {
		t.Error("expected error for invalid JSON input")
	}
}

func TestContextSearchTool_DefaultLimit(t *testing.T) {
	mgr := &mockContextManager{
		mode:       contextmgr.ModeHybrid,
		searchHits: []contextmgr.SearchHit{},
	}
	tool := &ContextSearchTool{mgr: mgr}

	// Limit=0 should default to 5
	input := json.RawMessage(`{"query":"test","limit":0}`)
	_, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Tests: ContextExpandTool
// ---------------------------------------------------------------------------

func TestContextExpandTool_Execute(t *testing.T) {
	mgr := &mockContextManager{
		mode: contextmgr.ModeHybrid,
		expandRes: &contextmgr.ExpandResult{
			StepID:  1,
			Level:   0,
			Content: "expanded content",
			Tokens:  100,
		},
	}
	tool := &ContextExpandTool{mgr: mgr}

	input := json.RawMessage(`{"step_id":1}`)
	raw, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if result["hint"] == nil {
		t.Error("expected hint in output")
	}
}

func TestContextExpandTool_InvalidInput(t *testing.T) {
	mgr := &mockContextManager{mode: contextmgr.ModeHybrid}
	tool := &ContextExpandTool{mgr: mgr}

	_, err := tool.Execute(context.Background(), json.RawMessage(`not json`))
	if err == nil {
		t.Error("expected error for invalid JSON input")
	}
}

func TestContextExpandTool_Error(t *testing.T) {
	mgr := &mockContextManager{
		mode:      contextmgr.ModeHybrid,
		expandErr: errors.New("step not found"),
	}
	tool := &ContextExpandTool{mgr: mgr}

	_, err := tool.Execute(context.Background(), json.RawMessage(`{"step_id":999}`))
	if err == nil {
		t.Error("expected error for non-existent step")
	}
}

// ---------------------------------------------------------------------------
// Tests: ContextSurroundTool
// ---------------------------------------------------------------------------

func TestContextSurroundTool_Execute(t *testing.T) {
	mgr := &mockContextManager{
		mode: contextmgr.ModeHybrid,
		surroundRes: []contextmgr.RenderedBlock{
			{StepID: 1, Level: 0, Content: "step 1 content"},
			{StepID: 2, Level: 0, Content: "step 2 content"},
		},
	}
	tool := &ContextSurroundTool{mgr: mgr}

	input := json.RawMessage(`{"step_id":1}`)
	raw, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	var out ContextSurroundOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if len(out.Steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(out.Steps))
	}
}

func TestContextSurroundTool_WithStoreCausal(t *testing.T) {
	store := newMockStepStore()
	store.steps[1] = contextmgr.StepRecord{StepID: 1, Type: "tool", Content: "step 1", FilesRead: []string{"main.go"}}
	store.steps[2] = contextmgr.StepRecord{StepID: 2, Type: "reasoning", Content: "step 2", FilesModified: []string{"main.go"}}
	store.activeIDs = []int{1, 2}

	mgr := &mockContextManager{
		mode: contextmgr.ModeHybrid,
	}
	tool := &ContextSurroundTool{mgr: mgr, store: store}

	input := json.RawMessage(`{"step_id":1,"causal":true}`)
	raw, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	var out ContextSurroundOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if len(out.Steps) == 0 {
		t.Error("expected at least one step in causal output")
	}
}

func TestContextSurroundTool_WithStoreTaskGroup(t *testing.T) {
	store := newMockStepStore()
	store.steps[1] = contextmgr.StepRecord{StepID: 1, Type: "tool", Content: "step 1", TaskGroup: "group-a"}
	store.steps[2] = contextmgr.StepRecord{StepID: 2, Type: "reasoning", Content: "step 2", TaskGroup: "group-a"}
	store.activeIDs = []int{1, 2}

	mgr := &mockContextManager{
		mode: contextmgr.ModeHybrid,
	}
	tool := &ContextSurroundTool{mgr: mgr, store: store}

	input := json.RawMessage(`{"step_id":1,"task_group":"group-a"}`)
	raw, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	var out ContextSurroundOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if len(out.Steps) != 2 {
		t.Errorf("expected 2 steps in task group, got %d", len(out.Steps))
	}
}

// ---------------------------------------------------------------------------
// Tests: MemorySearchTool
// ---------------------------------------------------------------------------

func TestMemorySearchTool_Execute(t *testing.T) {
	mgr := &mockContextManager{
		mode: contextmgr.ModeHybrid,
		longMemRes: []contextmgr.LongMemRecord{
			{MemID: "mem1", Facts: []string{"fact1"}, Category: "decision", Confidence: 0.9},
			{MemID: "mem2", Facts: []string{"fact2"}, Category: "constraint", Confidence: 0.3},
		},
	}
	tool := &MemorySearchTool{mgr: mgr}

	input := json.RawMessage(`{"query":"test query"}`)
	raw, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	var out MemorySearchOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	// mem2 has confidence 0.3 < 0.5, should be filtered out
	if len(out.Hits) != 1 {
		t.Errorf("expected 1 hit (filtered by confidence), got %d", len(out.Hits))
	}
	if out.Hits[0].MemID != "mem1" {
		t.Errorf("expected MemID=mem1, got %s", out.Hits[0].MemID)
	}
}

func TestMemorySearchTool_EmptyHits(t *testing.T) {
	mgr := &mockContextManager{
		mode:       contextmgr.ModeHybrid,
		longMemRes: []contextmgr.LongMemRecord{},
	}
	tool := &MemorySearchTool{mgr: mgr}

	input := json.RawMessage(`{"query":"nothing"}`)
	raw, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	var hint map[string]string
	if err := json.Unmarshal(raw, &hint); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if hint["hint"] == "" {
		t.Error("expected hint for empty results")
	}
}

func TestMemorySearchTool_InvalidInput(t *testing.T) {
	mgr := &mockContextManager{mode: contextmgr.ModeHybrid}
	tool := &MemorySearchTool{mgr: mgr}

	_, err := tool.Execute(context.Background(), json.RawMessage(`not json`))
	if err == nil {
		t.Error("expected error for invalid JSON input")
	}
}

// ---------------------------------------------------------------------------
// Tests: ContextLockL0Tool
// ---------------------------------------------------------------------------

func TestContextLockL0Tool_Lock(t *testing.T) {
	store := newMockStepStore()
	store.refs[1] = contextmgr.RefRecord{StepID: 1, LockL0: false}

	tool := &ContextLockL0Tool{store: store}
	input := json.RawMessage(`{"step_id":1,"reason":"test lock"}`)
	raw, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if result["locked"] != true {
		t.Error("expected locked=true")
	}
	if !store.refs[1].LockL0 {
		t.Error("expected store ref LockL0 to be true")
	}
	if len(store.auditLogs) != 1 {
		t.Errorf("expected 1 audit log, got %d", len(store.auditLogs))
	}
}

func TestContextLockL0Tool_AlreadyLocked(t *testing.T) {
	store := newMockStepStore()
	store.refs[1] = contextmgr.RefRecord{StepID: 1, LockL0: true}

	tool := &ContextLockL0Tool{store: store}
	input := json.RawMessage(`{"step_id":1}`)
	raw, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if result["locked"] != true {
		t.Error("expected locked=true")
	}
	// Should not add another audit log for already locked step
	if len(store.auditLogs) != 0 {
		t.Errorf("expected 0 audit logs for already locked step, got %d", len(store.auditLogs))
	}
}

func TestContextLockL0Tool_NotFound(t *testing.T) {
	store := newMockStepStore()
	store.getRefErr = errors.New("step not found")

	tool := &ContextLockL0Tool{store: store}
	_, err := tool.Execute(context.Background(), json.RawMessage(`{"step_id":999}`))
	if err == nil {
		t.Error("expected error for non-existent step")
	}
}

// ---------------------------------------------------------------------------
// Tests: ContextUnlockL0Tool
// ---------------------------------------------------------------------------

func TestContextUnlockL0Tool_Unlock(t *testing.T) {
	store := newMockStepStore()
	store.refs[1] = contextmgr.RefRecord{StepID: 1, LockL0: true}

	tool := &ContextUnlockL0Tool{store: store}
	input := json.RawMessage(`{"step_id":1,"reason":"test unlock"}`)
	raw, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if result["locked"] != false {
		t.Error("expected locked=false")
	}
	if store.refs[1].LockL0 {
		t.Error("expected store ref LockL0 to be false")
	}
	if len(store.auditLogs) != 1 {
		t.Errorf("expected 1 audit log, got %d", len(store.auditLogs))
	}
}

func TestContextUnlockL0Tool_AlreadyUnlocked(t *testing.T) {
	store := newMockStepStore()
	store.refs[1] = contextmgr.RefRecord{StepID: 1, LockL0: false}

	tool := &ContextUnlockL0Tool{store: store}
	input := json.RawMessage(`{"step_id":1}`)
	raw, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if result["locked"] != false {
		t.Error("expected locked=false")
	}
	if len(store.auditLogs) != 0 {
		t.Errorf("expected 0 audit logs for already unlocked step, got %d", len(store.auditLogs))
	}
}

// ---------------------------------------------------------------------------
// Tests: ContextTraceTool
// ---------------------------------------------------------------------------

func TestContextTraceTool_ByFile(t *testing.T) {
	store := newMockStepStore()
	store.steps[1] = contextmgr.StepRecord{StepID: 1, Type: "tool", Content: "step 1", FilesRead: []string{"main.go"}}
	store.steps[2] = contextmgr.StepRecord{StepID: 2, Type: "reasoning", Content: "step 2", FilesModified: []string{"main.go"}}
	store.activeIDs = []int{1, 2}

	tool := &ContextTraceTool{store: store}
	input := json.RawMessage(`{"file":"main.go"}`)
	raw, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	var out ContextTraceOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if len(out.Steps) == 0 {
		t.Error("expected at least one step for file trace")
	}
}

func TestContextTraceTool_ByStepID(t *testing.T) {
	store := newMockStepStore()
	store.steps[1] = contextmgr.StepRecord{StepID: 1, Type: "tool", Content: "step 1", FilesRead: []string{"main.go"}}
	store.steps[2] = contextmgr.StepRecord{StepID: 2, Type: "reasoning", Content: "step 2", FilesModified: []string{"main.go"}}
	store.activeIDs = []int{1, 2}

	tool := &ContextTraceTool{store: store}
	input := json.RawMessage(`{"step_id":1}`)
	raw, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	var out ContextTraceOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if len(out.Steps) == 0 {
		t.Error("expected at least one step for step trace")
	}
}

func TestContextTraceTool_ByTaskGroup(t *testing.T) {
	store := newMockStepStore()
	store.steps[1] = contextmgr.StepRecord{StepID: 1, Type: "tool", Content: "step 1", TaskGroup: "group-a"}
	store.steps[2] = contextmgr.StepRecord{StepID: 2, Type: "reasoning", Content: "step 2", TaskGroup: "group-a"}
	store.activeIDs = []int{1, 2}

	tool := &ContextTraceTool{store: store}
	input := json.RawMessage(`{"task_group":"group-a"}`)
	raw, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	var out ContextTraceOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if len(out.Steps) != 2 {
		t.Errorf("expected 2 steps for task group trace, got %d", len(out.Steps))
	}
}

func TestContextTraceTool_NoParams(t *testing.T) {
	store := newMockStepStore()

	tool := &ContextTraceTool{store: store}
	input := json.RawMessage(`{}`)
	raw, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	var hint map[string]string
	if err := json.Unmarshal(raw, &hint); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if hint["hint"] == "" {
		t.Error("expected hint for no-params trace")
	}
}

// ---------------------------------------------------------------------------
// Tests: ContextReorganizeTool
// ---------------------------------------------------------------------------

func TestContextReorganizeTool_Execute(t *testing.T) {
	reorgMock := &mockReorgProvider{
		reorgRes: &contextmgr.ReorganizeResult{
			ConstitutionalAdded: 2,
			HeadRewritten:       true,
			StepsAdjusted:       3,
		},
	}
	tool := &ContextReorganizeTool{mgr: reorgMock}

	input := json.RawMessage(`{"focus":"redis config migration"}`)
	raw, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if result["hint"] == nil {
		t.Error("expected hint in output")
	}
}

func TestContextReorganizeTool_InvalidInput(t *testing.T) {
	reorgMock := &mockReorgProvider{}
	tool := &ContextReorganizeTool{mgr: reorgMock}

	_, err := tool.Execute(context.Background(), json.RawMessage(`not json`))
	if err == nil {
		t.Error("expected error for invalid JSON input")
	}
}

// ---------------------------------------------------------------------------
// Tests: extractStepType (unexported, tested via surrounding logic)
// ---------------------------------------------------------------------------

func TestExtractStepType(t *testing.T) {
	tests := []struct {
		content string
		want    string
	}{
		{content: "⟨§1·tool·L0⟩ some content", want: "tool"},
		{content: "⟨§2·reasoning·L1⟩ reasoning content", want: "reasoning"},
		{content: "⟨§3·user·L2⟩ user content", want: "user"},
		{content: "no marker here", want: "unknown"},
		{content: "", want: "unknown"},
	}
	for _, tt := range tests {
		got := extractStepType(tt.content)
		if got != tt.want {
			t.Errorf("extractStepType(%q) = %q, want %q", tt.content, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Tests: truncateContent (unexported, tested via surrounding logic)
// ---------------------------------------------------------------------------

func TestTruncateContent(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{input: "short", maxLen: 10, want: "short"},
		{input: "this is a long string", maxLen: 7, want: "this is..."},
		{input: "", maxLen: 5, want: ""},
		{input: "exact", maxLen: 5, want: "exact"},
	}
	for _, tt := range tests {
		got := truncateContent(tt.input, tt.maxLen)
		if got != tt.want {
			t.Errorf("truncateContent(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Tests: estimatePressure (unexported, tested via Expand tool)
// ---------------------------------------------------------------------------

func TestEstimatePressure_FromWindowPressure(t *testing.T) {
	mgr := &mockContextManager{
		stats: contextmgr.ContextStats{
			WindowPressure:   0.75,
			TotalTokens:      1000,
			CompressedTokens: 200,
		},
	}
	got := estimatePressure(mgr)
	if got != 0.75 {
		t.Errorf("estimatePressure = %f, want 0.75", got)
	}
}

func TestEstimatePressure_Fallback(t *testing.T) {
	mgr := &mockContextManager{
		stats: contextmgr.ContextStats{
			WindowPressure:   0,
			TotalTokens:      1000,
			CompressedTokens: 200,
		},
	}
	got := estimatePressure(mgr)
	want := 0.2
	if got != want {
		t.Errorf("estimatePressure = %f, want %f", got, want)
	}
}

func TestEstimatePressure_ZeroTokens(t *testing.T) {
	mgr := &mockContextManager{
		stats: contextmgr.ContextStats{
			WindowPressure:   0,
			TotalTokens:      0,
			CompressedTokens: 0,
		},
	}
	got := estimatePressure(mgr)
	if got != 0 {
		t.Errorf("estimatePressure = %f, want 0", got)
	}
}

// ---------------------------------------------------------------------------
// Tests: StepStoreProvider integration
// ---------------------------------------------------------------------------

// mockStepStoreProvider implements StepStoreProvider.
type mockStepStoreProvider struct {
	store *mockStepStore
}

func (m *mockStepStoreProvider) StepStore() contextmgr.StepStoreLike {
	return m.store
}

func TestRegisterContextToolsWithStore_StepStoreProvider(t *testing.T) {
	reg := &mockRegistry{}
	store := newMockStepStore()
	mgr := &mockContextManager{mode: contextmgr.ModeHybrid}

	RegisterContextToolsWithStore(reg, mgr, store)

	names := make(map[string]bool)
	for _, tl := range reg.tools {
		names[tl.Name()] = true
	}
	// trace tools require store != nil
	if !names["context_trace"] {
		t.Error("expected context_trace when store is provided")
	}
}

// ---------------------------------------------------------------------------
// Test JSON round-trip for input/output types
// ---------------------------------------------------------------------------

func TestContextSearchInput_JSONRoundTrip(t *testing.T) {
	in := ContextSearchInput{Query: "test", LevelMax: 2, TaskGroup: 1, Limit: 10}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var out ContextSearchInput
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Query != "test" || out.LevelMax != 2 || out.TaskGroup != 1 || out.Limit != 10 {
		t.Errorf("round-trip mismatch: %+v", out)
	}
}

func TestContextExpandInput_JSONRoundTrip(t *testing.T) {
	in := ContextExpandInput{StepID: 5, Full: true}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var out ContextExpandInput
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.StepID != 5 || !out.Full {
		t.Errorf("round-trip mismatch: %+v", out)
	}
}

func TestMemorySearchInput_JSONRoundTrip(t *testing.T) {
	in := MemorySearchInput{Query: "test", Category: "decision", Limit: 3}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var out MemorySearchInput
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Query != "test" || out.Category != "decision" || out.Limit != 3 {
		t.Errorf("round-trip mismatch: %+v", out)
	}
}
