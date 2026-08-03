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

package stage

import (
	"context"
	"errors"
	"testing"

	"github.com/inferglow/flow"
)

// TestAdapt_BasicConversion verifies Adapt converts a Func to a StepFunc
// and correctly handles map input/output.
func TestAdapt_BasicConversion(t *testing.T) {
	// Create a simple Func that echoes input and adds a field.
	stageFn := func(ctx context.Context, in Inputs, fctx flow.Context) (Outputs, error) {
		name, _ := in["name"].(string)
		return Outputs{
			"greeting": "hello " + name,
			"count":    len(name),
		}, nil
	}

	// Adapt to StepFunc.
	stepFn := Adapt(stageFn)

	// Call with map input.
	result, err := stepFn(context.Background(), map[string]any{"name": "world"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify result is map[string]any.
	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}

	// Verify outputs.
	if resultMap["greeting"] != "hello world" {
		t.Errorf("expected greeting %q, got %v", "hello world", resultMap["greeting"])
	}
	if resultMap["count"] != 5 {
		t.Errorf("expected count %d, got %v", 5, resultMap["count"])
	}
}

// TestAdapt_NonMapInput verifies Adapt wraps non-map input under "input" key.
func TestAdapt_NonMapInput(t *testing.T) {
	stageFn := func(ctx context.Context, in Inputs, fctx flow.Context) (Outputs, error) {
		// Should receive input under "input" key.
		val, ok := in["input"]
		if !ok {
			return nil, errors.New("input key not found")
		}
		return Outputs{"received": val}, nil
	}

	stepFn := Adapt(stageFn)

	// Call with non-map input (string).
	result, err := stepFn(context.Background(), "test-string")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap := result.(map[string]any)
	if resultMap["received"] != "test-string" {
		t.Errorf("expected %q, got %v", "test-string", resultMap["received"])
	}
}

// TestAdapt_ContextExtraction verifies Adapt extracts Context from ctx.
func TestAdapt_ContextExtraction(t *testing.T) {
	var capturedFctx flow.Context

	stageFn := func(ctx context.Context, in Inputs, fctx flow.Context) (Outputs, error) {
		capturedFctx = fctx
		return Outputs{"ok": true}, nil
	}

	stepFn := Adapt(stageFn)

	// Create a mock Context.
	mockFctx := &mockContext{}
	ctx := flow.WithFlowContext(context.Background(), mockFctx)

	_, err := stepFn(ctx, map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify Context was extracted and passed to Func.
	if capturedFctx != mockFctx {
		t.Error("Context was not correctly extracted from ctx")
	}
}

// TestAdapt_NoContext verifies Adapt works when Context is not injected.
func TestAdapt_NoContext(t *testing.T) {
	var capturedFctx flow.Context

	stageFn := func(ctx context.Context, in Inputs, fctx flow.Context) (Outputs, error) {
		capturedFctx = fctx
		return Outputs{"ok": true}, nil
	}

	stepFn := Adapt(stageFn)

	// Call without injecting Context.
	_, err := stepFn(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Context should be nil (not injected).
	if capturedFctx != nil {
		t.Error("expected nil Context when not injected")
	}
}

// TestAdapt_ErrorPropagation verifies errors from Func are propagated.
func TestAdapt_ErrorPropagation(t *testing.T) {
	expectedErr := errors.New("stage error")

	stageFn := func(ctx context.Context, in Inputs, fctx flow.Context) (Outputs, error) {
		return nil, expectedErr
	}

	stepFn := Adapt(stageFn)

	_, err := stepFn(context.Background(), map[string]any{})
	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

// mockContext is a minimal Context implementation for testing.
type mockContext struct{}

func (m *mockContext) ExecuteAction(ctx context.Context, name string, params map[string]any) (any, error) {
	return nil, nil
}
func (m *mockContext) GenerateModel(ctx context.Context, system, userMessage string) (string, error) {
	return "", nil
}
func (m *mockContext) SessionHistory() []map[string]any                     { return nil }
func (m *mockContext) AppendSession(role string, content any)               {}
func (m *mockContext) AuditAppend(source, action string, input, output any) {}
func (m *mockContext) SetValue(key string, value any)                       {}
func (m *mockContext) GetValue(key string) (any, bool)                      { return nil, false }
func (m *mockContext) StartSpan(ctx context.Context, kind flow.SpanKind, name string) (context.Context, flow.Span) {
	return ctx, flow.NoopSpan()
}
func (m *mockContext) MaskInput(input string) string    { return input }
func (m *mockContext) CheckOutput(output string) error  { return nil }
func (m *mockContext) RequestPause(reason string) error { return nil }
func (m *mockContext) RunAgent(ctx context.Context, userMessage string, systemPrompt string, opts *flow.AgentRunOptions) (string, error) {
	return "", nil
}
func (m *mockContext) RunAgentParallel(ctx context.Context, agents []flow.AgentSubTask) ([]string, error) {
	return nil, nil
}
