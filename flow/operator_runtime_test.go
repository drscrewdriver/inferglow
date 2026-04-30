package flow

import (
	"context"
	"errors"
	"testing"
)

// Check: NewOperatorRuntime 创建默认实例
func TestNewOperatorRuntime(t *testing.T) {
	reg := NewOperatorRegistry()
	sn := NewSignalNet()
	rt := NewOperatorRuntime(reg, sn)
	if rt == nil {
		t.Fatal("NewOperatorRuntime returned nil")
	}
	if rt.registry != reg {
		t.Error("registry not set")
	}
	if rt.signalNet != sn {
		t.Error("signalNet not set")
	}
	if rt.handlers == nil {
		t.Error("handlers map not initialized")
	}
}

// Check: RegisterHandler + Dispatch 正确路由到对应 Handler
func TestOperatorRuntimeDispatchRouting(t *testing.T) {
	reg := NewOperatorRegistry()
	sn := NewSignalNet()
	rt := NewOperatorRuntime(reg, sn)

	rt.RegisterHandler(&ChunkHandler{})

	oc := &OperatorContext{
		Ctx:      context.Background(),
		Operator: &Operator{ID: "op-1", Kind: OpChunk, Name: "chunk_op"},
		Input:    "hello",
	}
	out, err := rt.Dispatch(oc)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	if out != "hello" {
		t.Errorf("output = %v, want hello", out)
	}
}

// Check: 未注册 Kind 返回 ErrOperatorNotImplemented
func TestOperatorRuntimeDispatchNotImplemented(t *testing.T) {
	rt := NewOperatorRuntime(NewOperatorRegistry(), NewSignalNet())

	oc := &OperatorContext{
		Ctx:      context.Background(),
		Operator: &Operator{ID: "op-1", Kind: OpForEachSplit, Name: "unimplemented"},
		Input:    "x",
	}
	_, err := rt.Dispatch(oc)
	if err == nil {
		t.Fatal("expected error for unimplemented kind, got nil")
	}
	if !errors.Is(err, ErrOperatorNotImplemented) {
		t.Errorf("expected ErrOperatorNotImplemented, got %v", err)
	}
}

// Check: Dispatch 错误包含算子 Kind 名称
func TestOperatorRuntimeDispatchErrorContainsKind(t *testing.T) {
	rt := NewOperatorRuntime(NewOperatorRegistry(), NewSignalNet())
	oc := &OperatorContext{
		Ctx:      context.Background(),
		Operator: &Operator{ID: "op-1", Kind: OpSubFlow, Name: "subflow_op"},
	}
	_, err := rt.Dispatch(oc)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrOperatorNotImplemented) {
		t.Errorf("expected ErrOperatorNotImplemented wrap, got %v", err)
	}
	if !contains(err.Error(), string(OpSubFlow)) {
		t.Errorf("error message %q should contain kind %q", err.Error(), OpSubFlow)
	}
}

// Check: 8 种未实现算子都返回 ErrOperatorNotImplemented
func TestOperatorRuntimeAllUnimplementedKinds(t *testing.T) {
	rt := NewOperatorRuntime(NewOperatorRegistry(), NewSignalNet())
	unimplemented := []OperatorKind{
		OpForEachSplit,
		OpForEachCollect,
		OpMatchCase,
		OpMatchCollect,
		OpCollectBranch,
		OpIntervention,
		OpSubFlow,
		OpResultSink,
	}
	if len(unimplemented) != 8 {
		t.Fatalf("expected 8 unimplemented kinds, got %d", len(unimplemented))
	}
	for _, kind := range unimplemented {
		oc := &OperatorContext{
			Ctx:      context.Background(),
			Operator: &Operator{ID: "op-" + string(kind), Kind: kind, Name: string(kind)},
		}
		_, err := rt.Dispatch(oc)
		if err == nil {
			t.Errorf("kind %q: expected error, got nil", kind)
			continue
		}
		if !errors.Is(err, ErrOperatorNotImplemented) {
			t.Errorf("kind %q: expected ErrOperatorNotImplemented, got %v", kind, err)
		}
	}
}

// Check: nil context 返回 error
func TestOperatorRuntimeDispatchNilContext(t *testing.T) {
	rt := NewOperatorRuntime(NewOperatorRegistry(), NewSignalNet())
	_, err := rt.Dispatch(nil)
	if err == nil {
		t.Fatal("expected error for nil context")
	}
}

// Check: nil Operator 返回 error
func TestOperatorRuntimeDispatchNilOperator(t *testing.T) {
	rt := NewOperatorRuntime(NewOperatorRegistry(), NewSignalNet())
	_, err := rt.Dispatch(&OperatorContext{Ctx: context.Background()})
	if err == nil {
		t.Fatal("expected error for nil operator")
	}
}

// Check: RegisterHandler(nil) 不 panic
func TestOperatorRuntimeRegisterNilHandler(t *testing.T) {
	rt := NewOperatorRuntime(NewOperatorRegistry(), NewSignalNet())
	rt.RegisterHandler(nil) // should not panic
}

// Check: 多次注册同一 Kind 覆盖之前的 Handler
func TestOperatorRuntimeRegisterOverride(t *testing.T) {
	rt := NewOperatorRuntime(NewOperatorRegistry(), NewSignalNet())
	rt.RegisterHandler(&ChunkHandler{})

	// 自定义 Handler 覆盖
	custom := &fakeHandler{kind: OpChunk, result: "custom_result"}
	rt.RegisterHandler(custom)

	oc := &OperatorContext{
		Ctx:      context.Background(),
		Operator: &Operator{ID: "op-1", Kind: OpChunk, Name: "chunk"},
		Input:    "x",
	}
	out, err := rt.Dispatch(oc)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	if out != "custom_result" {
		t.Errorf("output = %v, want custom_result (overridden handler)", out)
	}
}

// fakeHandler 用于测试的自定义 Handler
type fakeHandler struct {
	kind   OperatorKind
	result any
	err    error
}

func (f *fakeHandler) Kind() OperatorKind { return f.kind }
func (f *fakeHandler) Execute(oc *OperatorContext) (any, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

// contains 简单字符串包含检查
func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
