package flow

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

// ============================================================================
// FlowTriggerFlowDefinition 测试
// ============================================================================

func TestNewFlowTriggerFlowDefinition(t *testing.T) {
	d := NewFlowTriggerFlowDefinition("my_flow")
	if d.Name != "my_flow" {
		t.Errorf("Name = %q, want my_flow", d.Name)
	}
	if d.Version != "trigger_flow/v1" {
		t.Errorf("Version = %q, want trigger_flow/v1", d.Version)
	}
	if len(d.Operators) != 0 {
		t.Errorf("Operators len = %d, want 0", len(d.Operators))
	}
	if d.Signals == nil {
		t.Error("Signals should be non-nil")
	}
}

func TestFlowTriggerFlowDefinitionAddOperator(t *testing.T) {
	d := NewFlowTriggerFlowDefinition("f")
	op := &Operator{ID: "op-1", Kind: OpChunk, Name: "chunk1"}
	d.AddOperator(op)
	if len(d.Operators) != 1 {
		t.Fatalf("Operators len = %d, want 1", len(d.Operators))
	}
	if d.Operators[0].ID != "op-1" {
		t.Errorf("Operators[0].ID = %q, want op-1", d.Operators[0].ID)
	}
}

// ============================================================================
// ResolveOperatorHandler 测试
// ============================================================================

func TestResolveOperatorHandlerAllKinds(t *testing.T) {
	kinds := []OperatorKind{
		OpChunk, OpSignalGate, OpBatchFanout, OpBatchCollect,
		OpForEachSplit, OpForEachCollect, OpMatchRoute, OpMatchCase,
		OpMatchCollect, OpCollectBranch, OpIntervention, OpSubFlow, OpResultSink,
	}
	if len(kinds) != 13 {
		t.Fatalf("expected 13 kinds, got %d", len(kinds))
	}
	for _, k := range kinds {
		h, err := ResolveOperatorHandler(k)
		if err != nil {
			t.Errorf("ResolveOperatorHandler(%s) error: %v", k, err)
			continue
		}
		if h == nil {
			t.Errorf("ResolveOperatorHandler(%s) = nil", k)
			continue
		}
		if h.Kind() != k {
			t.Errorf("handler.Kind() = %q, want %q", h.Kind(), k)
		}
	}
}

func TestResolveOperatorHandlerUnknownKind(t *testing.T) {
	_, err := ResolveOperatorHandler(OperatorKind("unknown"))
	if err == nil {
		t.Fatal("expected error for unknown kind")
	}
	if !strings.Contains(err.Error(), "not implemented") {
		t.Errorf("error should mention 'not implemented', got: %v", err)
	}
}

// ============================================================================
// TriggerFlowBlueprint 测试
// ============================================================================

func TestNewTriggerFlowBlueprint(t *testing.T) {
	bp := NewTriggerFlowBlueprint()
	if bp == nil {
		t.Fatal("expected non-nil blueprint")
	}
	if bp.IsCompiled() {
		t.Error("new blueprint should not be compiled")
	}
	if len(bp.ListChunks()) != 0 {
		t.Errorf("new blueprint should have no chunks, got %d", len(bp.ListChunks()))
	}
}

func TestTriggerFlowBlueprintCreateChunk(t *testing.T) {
	bp := NewTriggerFlowBlueprint()
	handler := func(rd *TriggerFlowRuntimeData) (any, error) {
		return "executed", nil
	}
	chunk := bp.CreateChunk(handler, "my_chunk")
	if chunk == nil {
		t.Fatal("expected non-nil chunk")
	}
	if chunk.Name != "my_chunk" {
		t.Errorf("chunk.Name = %q, want my_chunk", chunk.Name)
	}
	if chunk.ID == "" {
		t.Error("chunk.ID should not be empty")
	}

	// GetChunk 应能找到
	got, ok := bp.GetChunk("my_chunk")
	if !ok {
		t.Fatal("GetChunk failed")
	}
	if got != chunk {
		t.Error("GetChunk should return same chunk")
	}

	// ListChunks 应包含 1 个
	if len(bp.ListChunks()) != 1 {
		t.Errorf("ListChunks len = %d, want 1", len(bp.ListChunks()))
	}

	// ChunkRegistry 应包含 handler
	registry := bp.ChunkRegistry()
	if _, ok := registry["my_chunk"]; !ok {
		t.Error("ChunkRegistry should contain 'my_chunk'")
	}
}

func TestTriggerFlowBlueprintCreateChunkOverwrite(t *testing.T) {
	bp := NewTriggerFlowBlueprint()
	h1 := func(rd *TriggerFlowRuntimeData) (any, error) { return "v1", nil }
	h2 := func(rd *TriggerFlowRuntimeData) (any, error) { return "v2", nil }
	bp.CreateChunk(h1, "chunk")
	bp.CreateChunk(h2, "chunk")
	if len(bp.ListChunks()) != 1 {
		t.Errorf("expected 1 chunk (overwritten), got %d", len(bp.ListChunks()))
	}
}

func TestTriggerFlowBlueprintAddOperator(t *testing.T) {
	bp := NewTriggerFlowBlueprint()
	op := &Operator{ID: "op-1", Kind: OpChunk, Name: "chunk1"}
	bp.AddOperator(op)
	def := bp.GetDefinition()
	if def == nil {
		t.Fatal("GetDefinition returned nil")
	}
	if len(def.Operators) != 1 {
		t.Errorf("Operators len = %d, want 1", len(def.Operators))
	}
}

func TestTriggerFlowBlueprintCompileEmpty(t *testing.T) {
	bp := NewTriggerFlowBlueprint()
	// 空 definition 也能编译（无 operators）
	if err := bp.Compile(); err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	if !bp.IsCompiled() {
		t.Error("should be compiled after Compile()")
	}
}

func TestTriggerFlowBlueprintCompileWithOperators(t *testing.T) {
	bp := NewTriggerFlowBlueprint()
	bp.AddOperator(&Operator{ID: "op-1", Kind: OpChunk, Name: "chunk1"})
	bp.AddOperator(&Operator{ID: "op-2", Kind: OpResultSink, Name: "sink1"})
	if err := bp.Compile(); err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	// 应该有两个 handler 三层映射
	h1, ok := bp.GetHandler(OpChunk, "event")
	if !ok || h1 == nil {
		t.Error("GetHandler(OpChunk, event) failed")
	}
	h2, ok := bp.GetHandler(OpResultSink, "runtime_data")
	if !ok || h2 == nil {
		t.Error("GetHandler(OpResultSink, runtime_data) failed")
	}
}

func TestTriggerFlowBlueprintCompileInvalidKind(t *testing.T) {
	bp := NewTriggerFlowBlueprint()
	bp.AddOperator(&Operator{ID: "op-1", Kind: OperatorKind("invalid"), Name: "x"})
	err := bp.Compile()
	if err == nil {
		t.Fatal("expected error for invalid kind")
	}
	if !strings.Contains(err.Error(), "invalid") {
		t.Errorf("error should mention 'invalid', got: %v", err)
	}
}

func TestTriggerFlowBlueprintCompileInvokesHandler(t *testing.T) {
	bp := NewTriggerFlowBlueprint()
	bp.AddOperator(&Operator{ID: "op-1", Kind: OpChunk, Name: "chunk1"})
	if err := bp.Compile(); err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	// 通过 GetHandler 调用编译后的 Handler
	h, ok := bp.GetHandler(OpChunk, "event")
	if !ok {
		t.Fatal("GetHandler failed")
	}
	rd := &TriggerFlowRuntimeData{
		RuntimeData: map[string]any{},
		FlowData:    map[string]any{},
		Signal:      &Signal{Value: "test_input"},
	}
	out, err := h(rd)
	if err != nil {
		t.Fatalf("Handler call failed: %v", err)
	}
	// ChunkHandler 透传输入
	if out != "test_input" {
		t.Errorf("output = %v, want test_input", out)
	}
}

// ============================================================================
// TriggerFlowChunk 测试
// ============================================================================

func TestTriggerFlowChunkAsyncCallHandlerType(t *testing.T) {
	bp := NewTriggerFlowBlueprint()
	called := false
	handler := Handler(func(rd *TriggerFlowRuntimeData) (any, error) {
		called = true
		if rd.Signal == nil || rd.Signal.Value != "input" {
			t.Errorf("Signal.Value = %v, want input", rd.Signal)
		}
		return "result", nil
	})
	chunk := bp.CreateChunk(handler, "c1")
	rd := &TriggerFlowRuntimeData{
		RuntimeData: map[string]any{},
		FlowData:    map[string]any{},
		Signal:      &Signal{Value: "input"},
	}
	out, err := chunk.AsyncCall(rd)
	if err != nil {
		t.Fatalf("AsyncCall failed: %v", err)
	}
	if !called {
		t.Error("handler was not called")
	}
	if out != "result" {
		t.Errorf("output = %v, want result", out)
	}
}

func TestTriggerFlowChunkAsyncCallOperatorHandler(t *testing.T) {
	bp := NewTriggerFlowBlueprint()
	// 注册一个 OperatorHandler
	oh := &ChunkHandler{}
	chunk := bp.CreateChunk(oh, "c2")
	rd := &TriggerFlowRuntimeData{
		RuntimeData: map[string]any{},
		FlowData:    map[string]any{},
		Signal:      &Signal{Value: "payload"},
	}
	out, err := chunk.AsyncCall(rd)
	if err != nil {
		t.Fatalf("AsyncCall failed: %v", err)
	}
	if out != "payload" {
		t.Errorf("output = %v, want payload", out)
	}
}

func TestTriggerFlowChunkAsyncCallFuncAnyError(t *testing.T) {
	bp := NewTriggerFlowBlueprint()
	fn := func(in any) (any, error) {
		return in.(string) + "_processed", nil
	}
	chunk := bp.CreateChunk(fn, "c3")
	rd := &TriggerFlowRuntimeData{
		Signal: &Signal{Value: "data"},
	}
	out, err := chunk.AsyncCall(rd)
	if err != nil {
		t.Fatalf("AsyncCall failed: %v", err)
	}
	if out != "data_processed" {
		t.Errorf("output = %v, want data_processed", out)
	}
}

func TestTriggerFlowChunkAsyncCallFuncAnyNoError(t *testing.T) {
	bp := NewTriggerFlowBlueprint()
	fn := func(in any) any {
		return in.(string) + "_noerr"
	}
	chunk := bp.CreateChunk(fn, "c4")
	rd := &TriggerFlowRuntimeData{
		Signal: &Signal{Value: "data"},
	}
	out, err := chunk.AsyncCall(rd)
	if err != nil {
		t.Fatalf("AsyncCall failed: %v", err)
	}
	if out != "data_noerr" {
		t.Errorf("output = %v, want data_noerr", out)
	}
}

func TestTriggerFlowChunkAsyncCallUnsupportedHandler(t *testing.T) {
	bp := NewTriggerFlowBlueprint()
	// string 不是支持的 handler 类型
	chunk := bp.CreateChunk("not_a_handler", "c5")
	rd := &TriggerFlowRuntimeData{
		Signal: &Signal{Value: "x"},
	}
	_, err := chunk.AsyncCall(rd)
	if err == nil {
		t.Fatal("expected error for unsupported handler type")
	}
}

func TestTriggerFlowChunkAsyncCallNilData(t *testing.T) {
	bp := NewTriggerFlowBlueprint()
	handler := Handler(func(rd *TriggerFlowRuntimeData) (any, error) {
		if rd == nil {
			t.Error("rd should not be nil after AsyncCall nil-safe init")
		}
		if rd.RuntimeData == nil {
			t.Error("RuntimeData should be initialized")
		}
		return "ok", nil
	})
	chunk := bp.CreateChunk(handler, "c6")
	out, err := chunk.AsyncCall(nil)
	if err != nil {
		t.Fatalf("AsyncCall failed: %v", err)
	}
	if out != "ok" {
		t.Errorf("output = %v, want ok", out)
	}
}

// ============================================================================
// TriggerFlow[InputT, StreamT, ResultT] 测试
// ============================================================================

func TestNewTriggerFlow(t *testing.T) {
	f := NewTriggerFlow[string, string, string]()
	if f == nil {
		t.Fatal("expected non-nil TriggerFlow")
	}
	if f.blueprint == nil {
		t.Fatal("blueprint should be initialized")
	}
}

func TestTriggerFlowSkipExceptions(t *testing.T) {
	f := NewTriggerFlow[string, string, string]()
	if f.skipExceptions {
		t.Error("default skipExceptions should be false")
	}
	f.SkipExceptions(true)
	if !f.skipExceptions {
		t.Error("skipExceptions should be true after setter")
	}
}

func TestTriggerFlowCreateChunkDelegates(t *testing.T) {
	f := NewTriggerFlow[string, string, string]()
	handler := func(rd *TriggerFlowRuntimeData) (any, error) { return "ok", nil }
	chunk := f.CreateChunk(handler, "via_flow")
	if chunk == nil {
		t.Fatal("CreateChunk returned nil")
	}
	if chunk.Name != "via_flow" {
		t.Errorf("chunk.Name = %q, want via_flow", chunk.Name)
	}
}

func TestTriggerFlowAddOperatorDelegates(t *testing.T) {
	f := NewTriggerFlow[string, string, string]()
	op := &Operator{ID: "op-1", Kind: OpChunk, Name: "chunk1"}
	f.AddOperator(op)
	def := f.Blueprint().GetDefinition()
	if len(def.Operators) != 1 {
		t.Errorf("Operators len = %d, want 1", len(def.Operators))
	}
}

func TestTriggerFlowCompileDelegates(t *testing.T) {
	f := NewTriggerFlow[string, string, string]()
	f.AddOperator(&Operator{ID: "op-1", Kind: OpChunk, Name: "c1"})
	if err := f.Compile(); err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	if !f.Blueprint().IsCompiled() {
		t.Error("blueprint should be compiled")
	}
}

func TestTriggerFlowRun(t *testing.T) {
	// 简单的 chunk + result_sink 流程
	f := NewTriggerFlow[string, string, string]()
	f.AddOperator(&Operator{ID: "op-1", Kind: OpChunk, Name: "c1"})
	f.AddOperator(&Operator{ID: "op-2", Kind: OpResultSink, Name: "sink1"})
	out, err := f.Run("input_data")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	// Chunk 透传 + ResultSink 透传 = 原样返回
	if out != "input_data" {
		t.Errorf("output = %v, want input_data", out)
	}
}

func TestTriggerFlowRunAutoCompile(t *testing.T) {
	f := NewTriggerFlow[string, string, string]()
	f.AddOperator(&Operator{ID: "op-1", Kind: OpChunk, Name: "c1"})
	// 不显式 Compile，Run 应自动编译
	out, err := f.Run("data")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if out != "data" {
		t.Errorf("output = %v, want data", out)
	}
}

func TestTriggerFlowRunSkipExceptions(t *testing.T) {
	f := NewTriggerFlow[string, string, string]()
	f.SkipExceptions(true)
	// 添加一个会失败的算子（invalid kind）+ 一个正常算子
	f.AddOperator(&Operator{ID: "op-1", Kind: OperatorKind("invalid"), Name: "bad"})
	f.AddOperator(&Operator{ID: "op-2", Kind: OpChunk, Name: "good"})
	out, err := f.Run("data")
	if err != nil {
		t.Fatalf("Run should not return error with SkipExceptions, got: %v", err)
	}
	// "invalid" 被跳过，"good" 透传，应返回 data
	if out != "data" {
		t.Errorf("output = %v, want data", out)
	}
}

func TestTriggerFlowRunTypeMismatch(t *testing.T) {
	// 流程返回 int，但 ResultT 是 string
	f := NewTriggerFlow[int, int, string]()
	// 用一个 executor 把 int 转成 int 返回（不是 string）
	f.AddOperator(&Operator{
		ID:   "op-1",
		Kind: OpSubFlow,
		Name: "sub",
		Options: map[string]any{
			"child_flow_executor": func(in any) (any, error) {
				return 42, nil
			},
		},
	})
	_, err := f.Run(1)
	if err == nil {
		t.Fatal("expected error for type mismatch")
	}
	if !strings.Contains(err.Error(), "type mismatch") {
		t.Errorf("error should mention 'type mismatch', got: %v", err)
	}
}

// ============================================================================
// 并发安全测试
// ============================================================================

func TestTriggerFlowBlueprintConcurrentCreateChunk(t *testing.T) {
	bp := NewTriggerFlowBlueprint()
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			bp.CreateChunk(func(rd *TriggerFlowRuntimeData) (any, error) { return n, nil }, "chunk_"+intToStr(n))
		}(i)
	}
	wg.Wait()
	if len(bp.ListChunks()) != 10 {
		t.Errorf("expected 10 chunks, got %d", len(bp.ListChunks()))
	}
}

// ============================================================================
// nil 安全测试
// ============================================================================

func TestTriggerFlowBlueprintNilSafe(t *testing.T) {
	var bp *TriggerFlowBlueprint
	if bp.IsCompiled() {
		t.Error("nil IsCompiled should return false")
	}
	if _, ok := bp.GetChunk("x"); ok {
		t.Error("nil GetChunk should return false")
	}
	if bp.ListChunks() != nil {
		t.Error("nil ListChunks should return nil")
	}
	if bp.GetDefinition() != nil {
		t.Error("nil GetDefinition should return nil")
	}
	if _, ok := bp.GetHandler(OpChunk, "event"); ok {
		t.Error("nil GetHandler should return false")
	}
	bp.AddOperator(nil)        // 不应 panic
	bp.SetDefinition(nil)      // 不应 panic
	_ = bp.ChunkRegistry()     // 不应 panic
}

func TestTriggerFlowChunkNilSafe(t *testing.T) {
	var c *TriggerFlowChunk
	_, err := c.AsyncCall(nil)
	if err == nil {
		t.Error("nil AsyncCall should return error")
	}
}

// 重新声明 intToStr 以避免跨文件依赖
func intToStr2(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// 用别名以避免 unused 警告（测试中使用）
var _ = intToStr2

// 避免引入 errors 包未使用
var _ = errors.New
var _ = context.Background
