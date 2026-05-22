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
	"errors"
	"sync"
	"testing"
)

// Check: SignalType 常量值正确
func TestSignalTypeConstants(t *testing.T) {
	if SignalEvent != SignalType("event") {
		t.Errorf("SignalEvent = %q, want event", SignalEvent)
	}
	if SignalRuntimeData != SignalType("runtime_data") {
		t.Errorf("SignalRuntimeData = %q, want runtime_data", SignalRuntimeData)
	}
	if SignalFlowData != SignalType("flow_data") {
		t.Errorf("SignalFlowData = %q, want flow_data", SignalFlowData)
	}
}

// Check: Signal 构造与字段
func TestSignalConstruction(t *testing.T) {
	sig := &Signal{
		ID:           "sig-1",
		TriggerEvent: "START",
		TriggerType:  SignalEvent,
		Value:        "hello",
		Meta:         map[string]any{"source": "test"},
	}
	if sig.ID != "sig-1" {
		t.Errorf("ID = %q, want sig-1", sig.ID)
	}
	if sig.TriggerEvent != "START" {
		t.Errorf("TriggerEvent = %q, want START", sig.TriggerEvent)
	}
	if sig.TriggerType != SignalEvent {
		t.Errorf("TriggerType = %q, want %q", sig.TriggerType, SignalEvent)
	}
	if sig.Value != "hello" {
		t.Errorf("Value = %v, want hello", sig.Value)
	}
	if sig.Meta["source"] != "test" {
		t.Errorf("Meta[source] = %v, want test", sig.Meta["source"])
	}
}

// Check: TriggerFlowRuntimeData 构造
func TestTriggerFlowRuntimeData(t *testing.T) {
	sig := &Signal{ID: "sig-1", TriggerEvent: "START"}
	rd := &TriggerFlowRuntimeData{
		RuntimeData: map[string]any{"count": 5},
		FlowData:    map[string]any{"config": "prod"},
		Signal:      sig,
	}
	if rd.RuntimeData["count"] != 5 {
		t.Errorf("RuntimeData[count] = %v, want 5", rd.RuntimeData["count"])
	}
	if rd.FlowData["config"] != "prod" {
		t.Errorf("FlowData[config] = %v, want prod", rd.FlowData["config"])
	}
	if rd.Signal == nil || rd.Signal.ID != "sig-1" {
		t.Error("Signal not set correctly")
	}
}

// Check: SignalNet 静态 handler 注册和路由
func TestSignalNetStaticHandler(t *testing.T) {
	sn := NewSignalNet()

	called := false
	handler := func(data *TriggerFlowRuntimeData) (any, error) {
		called = true
		return data.Signal.Value, nil
	}

	sn.RegisterStaticHandler("START", "chunk_handler", handler)

	sig := &Signal{
		ID:           "sig-1",
		TriggerEvent: "START",
		TriggerType:  SignalEvent,
		Value:        "test_value",
	}

	handlers := sn.Route(sig)
	if len(handlers) != 1 {
		t.Fatalf("expected 1 handler, got %d", len(handlers))
	}

	rd := &TriggerFlowRuntimeData{Signal: sig}
	result, err := handlers[0](rd)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if !called {
		t.Error("handler was not called")
	}
	if result != "test_value" {
		t.Errorf("result = %v, want test_value", result)
	}
}

// Check: SignalNet 多个静态 handler 按名称注册
func TestSignalNetMultipleStaticHandlers(t *testing.T) {
	sn := NewSignalNet()

	sn.RegisterStaticHandler("START", "handler_a", func(data *TriggerFlowRuntimeData) (any, error) {
		return "a", nil
	})
	sn.RegisterStaticHandler("START", "handler_b", func(data *TriggerFlowRuntimeData) (any, error) {
		return "b", nil
	})

	sig := &Signal{ID: "sig-1", TriggerEvent: "START", TriggerType: SignalEvent}
	handlers := sn.Route(sig)
	if len(handlers) != 2 {
		t.Fatalf("expected 2 handlers, got %d", len(handlers))
	}
}

// Check: SignalNet 动态 handler 注册和路由
func TestSignalNetDynamicHandler(t *testing.T) {
	sn := NewSignalNet()

	handler := func(data *TriggerFlowRuntimeData) (any, error) {
		return "dynamic_result", nil
	}

	bindingID, err := sn.RegisterDynamicHandler("START", handler)
	if err != nil {
		t.Fatalf("RegisterDynamicHandler failed: %v", err)
	}
	if bindingID == "" {
		t.Error("expected non-empty bindingID")
	}

	sig := &Signal{ID: "sig-1", TriggerEvent: "START", TriggerType: SignalEvent}
	handlers := sn.Route(sig)
	if len(handlers) != 1 {
		t.Fatalf("expected 1 handler, got %d", len(handlers))
	}

	rd := &TriggerFlowRuntimeData{Signal: sig}
	result, _ := handlers[0](rd)
	if result != "dynamic_result" {
		t.Errorf("result = %v, want dynamic_result", result)
	}

	// Unregister
	if !sn.UnregisterDynamicHandler(bindingID) {
		t.Error("UnregisterDynamicHandler returned false")
	}

	handlers = sn.Route(sig)
	if len(handlers) != 0 {
		t.Errorf("expected 0 handlers after unregister, got %d", len(handlers))
	}
}

// Check: SignalNet 注销不存在的 bindingID 返回 false
func TestSignalNetUnregisterNotFound(t *testing.T) {
	sn := NewSignalNet()
	if sn.UnregisterDynamicHandler("nonexistent") {
		t.Error("expected false for non-existent bindingID")
	}
}

// Check: SignalNet durable binding
func TestSignalNetDurableBinding(t *testing.T) {
	sn := NewSignalNet()

	handler := func(data *TriggerFlowRuntimeData) (any, error) {
		return nil, nil
	}

	bindingID, _ := sn.RegisterDynamicHandler("START", handler, WithDurable(true))

	binding := sn.GetDynamicBinding(bindingID)
	if binding == nil {
		t.Fatal("binding not found")
	}
	if !binding.Durable {
		t.Error("expected Durable=true")
	}
}

// Check: SignalNet 非 durable binding（默认）
func TestSignalNetNonDurableBinding(t *testing.T) {
	sn := NewSignalNet()

	handler := func(data *TriggerFlowRuntimeData) (any, error) {
		return nil, nil
	}

	bindingID, _ := sn.RegisterDynamicHandler("START", handler)

	binding := sn.GetDynamicBinding(bindingID)
	if binding == nil {
		t.Fatal("binding not found")
	}
	if binding.Durable {
		t.Error("expected Durable=false by default")
	}
}

// Check: SignalNet ClearNonDurable 清除非 durable bindings
func TestSignalNetClearNonDurable(t *testing.T) {
	sn := NewSignalNet()

	handler := func(data *TriggerFlowRuntimeData) (any, error) {
		return nil, nil
	}

	durableID, _ := sn.RegisterDynamicHandler("START", handler, WithDurable(true))
	nonDurableID, _ := sn.RegisterDynamicHandler("START", handler)

	sn.ClearNonDurable()

	// durable 应保留
	if sn.GetDynamicBinding(durableID) == nil {
		t.Error("durable binding should be retained")
	}
	// non-durable 应被清除
	if sn.GetDynamicBinding(nonDurableID) != nil {
		t.Error("non-durable binding should be cleared")
	}
}

// Check: SignalNet 信号接受追踪
func TestSignalNetAcceptSignal(t *testing.T) {
	sn := NewSignalNet()

	sig := &Signal{
		ID:           "sig-1",
		TriggerEvent: "START",
		TriggerType:  SignalEvent,
	}

	if sn.IsAccepted("sig-1") {
		t.Error("signal should not be accepted yet")
	}

	sn.AcceptSignal(sig)

	if !sn.IsAccepted("sig-1") {
		t.Error("signal should be accepted after AcceptSignal")
	}
}

// Check: SignalNet 未知事件无 handler
func TestSignalNetNoHandlersForUnknownEvent(t *testing.T) {
	sn := NewSignalNet()

	sig := &Signal{
		ID:           "sig-1",
		TriggerEvent: "UNKNOWN_EVENT",
		TriggerType:  SignalEvent,
	}

	handlers := sn.Route(sig)
	if len(handlers) != 0 {
		t.Errorf("expected 0 handlers for unknown event, got %d", len(handlers))
	}
}

// Check: SignalNet 静态 + 动态 handler 同时路由
func TestSignalNetStaticAndDynamic(t *testing.T) {
	sn := NewSignalNet()

	sn.RegisterStaticHandler("START", "static_h", func(data *TriggerFlowRuntimeData) (any, error) {
		return "static", nil
	})
	_, _ = sn.RegisterDynamicHandler("START", func(data *TriggerFlowRuntimeData) (any, error) {
		return "dynamic", nil
	})

	sig := &Signal{ID: "sig-1", TriggerEvent: "START", TriggerType: SignalEvent}
	handlers := sn.Route(sig)
	if len(handlers) != 2 {
		t.Fatalf("expected 2 handlers (static+dynamic), got %d", len(handlers))
	}
}

// Check: SignalAttemptTracker 追踪
func TestSignalAttemptTracker(t *testing.T) {
	sn := NewSignalNet()

	sig := &Signal{
		ID:           "sig-1",
		TriggerEvent: "START",
		TriggerType:  SignalEvent,
	}

	sn.AcceptSignal(sig)

	tracker := sn.GetAttemptTracker("sig-1")
	if tracker == nil {
		t.Fatal("expected tracker after AcceptSignal")
	}
	if tracker.Attempts != 0 {
		t.Errorf("initial attempts = %d, want 0", tracker.Attempts)
	}

	sn.IncrementAttempt("sig-1")
	tracker = sn.GetAttemptTracker("sig-1")
	if tracker.Attempts != 1 {
		t.Errorf("attempts = %d, want 1", tracker.Attempts)
	}

	sn.IncrementAttempt("sig-1")
	tracker = sn.GetAttemptTracker("sig-1")
	if tracker.Attempts != 2 {
		t.Errorf("attempts = %d, want 2", tracker.Attempts)
	}
}

// Check: SignalAttemptTracker 记录错误
func TestSignalAttemptTrackerError(t *testing.T) {
	sn := NewSignalNet()

	sig := &Signal{ID: "sig-1", TriggerEvent: "START"}
	sn.AcceptSignal(sig)

	testErr := errors.New("handler failed")
	sn.MarkAttemptError("sig-1", testErr)

	tracker := sn.GetAttemptTracker("sig-1")
	if tracker.LastError == nil {
		t.Fatal("expected LastError to be set")
	}
	if tracker.LastError.Error() != "handler failed" {
		t.Errorf("LastError = %q, want 'handler failed'", tracker.LastError.Error())
	}
}

// Check: SignalNet 并发安全
func TestSignalNetConcurrent(t *testing.T) {
	sn := NewSignalNet()

	var wg sync.WaitGroup
	// 并发注册动态 handler
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = sn.RegisterDynamicHandler("START", func(data *TriggerFlowRuntimeData) (any, error) {
				return nil, nil
			})
		}()
	}
	// 并发路由
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sig := &Signal{ID: "sig-concurrent", TriggerEvent: "START", TriggerType: SignalEvent}
			_ = sn.Route(sig)
		}()
	}
	wg.Wait()
}
