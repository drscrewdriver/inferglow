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
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package flow

import (
	"testing"
)

// TestClearNonDurablePreservesDurableIndex verifies that after ClearNonDurable,
// durable bindings remain routable via Route. Regression for BUG-2:
// ClearNonDurable cleared dynamicIndex BEFORE building newIndex, so the
// durable binding's index entry was lost and Route returned no handlers.
func TestClearNonDurablePreservesDurableIndex(t *testing.T) {
	sn := NewSignalNet()

	durableCalled := false
	_, err := sn.RegisterDynamicHandler("START", func(data *TriggerFlowRuntimeData) (any, error) {
		durableCalled = true
		return "durable_result", nil
	}, WithDurable(true))
	if err != nil {
		t.Fatalf("RegisterDynamicHandler(durable) failed: %v", err)
	}

	// Add a non-durable binding that should be cleared.
	_, err = sn.RegisterDynamicHandler("START", func(data *TriggerFlowRuntimeData) (any, error) {
		return "non_durable_result", nil
	})
	if err != nil {
		t.Fatalf("RegisterDynamicHandler(non-durable) failed: %v", err)
	}

	// Sanity check: before ClearNonDurable, both handlers route.
	sig := &Signal{ID: "sig-1", TriggerEvent: "START", TriggerType: SignalEvent}
	beforeHandlers := sn.Route(sig)
	if len(beforeHandlers) != 2 {
		t.Fatalf("expected 2 handlers before ClearNonDurable, got %d", len(beforeHandlers))
	}

	sn.ClearNonDurable()

	// After ClearNonDurable, only the durable binding should remain.
	afterHandlers := sn.Route(sig)
	if len(afterHandlers) != 1 {
		t.Fatalf("expected 1 handler after ClearNonDurable, got %d (durable index lost)", len(afterHandlers))
	}

	// Verify the remaining handler is the durable one.
	rd := &TriggerFlowRuntimeData{Signal: sig}
	result, err := afterHandlers[0](rd)
	if err != nil {
		t.Fatalf("durable handler call failed: %v", err)
	}
	if result != "durable_result" {
		t.Errorf("result = %v, want durable_result", result)
	}
	if !durableCalled {
		t.Error("durable handler was not called")
	}
}

// TestClearNonDurablePreservesDurableIndexMultipleEvents verifies that
// durable bindings on DIFFERENT events remain routable after ClearNonDurable.
func TestClearNonDurablePreservesDurableIndexMultipleEvents(t *testing.T) {
	sn := NewSignalNet()

	_, _ = sn.RegisterDynamicHandler("START", func(data *TriggerFlowRuntimeData) (any, error) {
		return "start_durable", nil
	}, WithDurable(true))
	_, _ = sn.RegisterDynamicHandler("END", func(data *TriggerFlowRuntimeData) (any, error) {
		return "end_durable", nil
	}, WithDurable(true))
	// Non-durable on a third event.
	_, _ = sn.RegisterDynamicHandler("MIDDLE", func(data *TriggerFlowRuntimeData) (any, error) {
		return "middle_non_durable", nil
	})

	sn.ClearNonDurable()

	startSig := &Signal{ID: "s", TriggerEvent: "START", TriggerType: SignalEvent}
	endSig := &Signal{ID: "e", TriggerEvent: "END", TriggerType: SignalEvent}
	middleSig := &Signal{ID: "m", TriggerEvent: "MIDDLE", TriggerType: SignalEvent}

	if h := sn.Route(startSig); len(h) != 1 {
		t.Errorf("START: expected 1 durable handler, got %d", len(h))
	}
	if h := sn.Route(endSig); len(h) != 1 {
		t.Errorf("END: expected 1 durable handler, got %d", len(h))
	}
	if h := sn.Route(middleSig); len(h) != 0 {
		t.Errorf("MIDDLE: expected 0 handlers after clear, got %d", len(h))
	}
}
