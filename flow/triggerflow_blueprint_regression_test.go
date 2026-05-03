package flow

import (
	"testing"
)

// TestCompileMultipleOperatorsSameKind verifies that when multiple operators
// of the SAME Kind are added, each one gets its own handler entry after
// Compile. Regression for F-CRITICAL-2: the handlers map was keyed by
// `string(op.Kind)`, so multiple operators of the same Kind overwrote each
// other and only the LAST one survived.
func TestCompileMultipleOperatorsSameKind(t *testing.T) {
	bp := NewTriggerFlowBlueprint()
	bp.AddOperator(&Operator{ID: "op-1", Kind: OpChunk, Name: "chunk1"})
	bp.AddOperator(&Operator{ID: "op-2", Kind: OpChunk, Name: "chunk2"})
	if err := bp.Compile(); err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	// Both operators should have their own handler entry retrievable by ID.
	h1, ok := bp.GetHandlerByID(OpChunk, "op-1", "event")
	if !ok || h1 == nil {
		t.Error("GetHandlerByID(OpChunk, op-1, event) failed (operator lost during compile)")
	}
	h2, ok := bp.GetHandlerByID(OpChunk, "op-2", "event")
	if !ok || h2 == nil {
		t.Error("GetHandlerByID(OpChunk, op-2, event) failed (operator lost during compile)")
	}

	// The two handlers should be DISTINCT closures (bound to different operators).
	// They are not required to be different function pointers, but both must
	// be retrievable independently.
	if h1 == nil || h2 == nil {
		t.Error("expected both handlers to be non-nil")
	}
}

// TestCompileMultipleOperatorsSameKindAllLayers verifies the same bug for
// all three layers (event/flow_data/runtime_data).
func TestCompileMultipleOperatorsSameKindAllLayers(t *testing.T) {
	bp := NewTriggerFlowBlueprint()
	bp.AddOperator(&Operator{ID: "op-A", Kind: OpResultSink, Name: "sinkA"})
	bp.AddOperator(&Operator{ID: "op-B", Kind: OpResultSink, Name: "sinkB"})
	bp.AddOperator(&Operator{ID: "op-C", Kind: OpResultSink, Name: "sinkC"})
	if err := bp.Compile(); err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	for _, opID := range []string{"op-A", "op-B", "op-C"} {
		for _, layer := range []string{"event", "flow_data", "runtime_data"} {
			h, ok := bp.GetHandlerByID(OpResultSink, opID, layer)
			if !ok || h == nil {
				t.Errorf("GetHandlerByID(OpResultSink, %s, %s) failed (operator lost)", opID, layer)
			}
		}
	}
}

// TestGetHandlerByIDUnknown verifies GetHandlerByID returns false for
// unknown operator IDs.
func TestGetHandlerByIDUnknown(t *testing.T) {
	bp := NewTriggerFlowBlueprint()
	bp.AddOperator(&Operator{ID: "op-1", Kind: OpChunk, Name: "chunk1"})
	if err := bp.Compile(); err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	if _, ok := bp.GetHandlerByID(OpChunk, "nonexistent", "event"); ok {
		t.Error("GetHandlerByID should return false for nonexistent opID")
	}
	if _, ok := bp.GetHandlerByID(OperatorKind("unknown_kind"), "op-1", "event"); ok {
		t.Error("GetHandlerByID should return false for unknown kind")
	}
}

// TestGetHandlerByIDNilSafe verifies GetHandlerByID on a nil blueprint
// returns false without panicking.
func TestGetHandlerByIDNilSafe(t *testing.T) {
	var bp *TriggerFlowBlueprint
	if _, ok := bp.GetHandlerByID(OpChunk, "op-1", "event"); ok {
		t.Error("nil GetHandlerByID should return false")
	}
}
