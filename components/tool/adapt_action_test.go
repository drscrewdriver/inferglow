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

package tool

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/inferglow/action"
)

// mockActionExecutor 是一个最小的 action.ActionExecutor 实现，
// 记录接收到的输入并返回预设的结果或错误。
type mockActionExecutor struct {
	lastInput map[string]any
	result    *action.ActionResult
	err       error
}

func (m *mockActionExecutor) Execute(ctx context.Context, input map[string]any) (*action.ActionResult, error) {
	m.lastInput = input
	if m.err != nil {
		return nil, m.err
	}
	if m.result == nil {
		return &action.ActionResult{OK: true, Status: "success", Result: nil}, nil
	}
	return m.result, nil
}

func newTestAction(exec *mockActionExecutor) *action.Action {
	return &action.Action{
		Name:        "calc",
		Description: "a calculator",
		Schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"x": map[string]any{"type": "number"}},
			"required":   []any{"x"},
		},
		Tags:     []string{"math"},
		Executor: exec,
	}
}

func TestActionToTool_Info(t *testing.T) {
	exec := &mockActionExecutor{}
	a := newTestAction(exec)
	tool := ActionToTool(a)

	info, err := tool.Info(context.Background())
	if err != nil {
		t.Fatalf("Info returned unexpected error: %v", err)
	}
	if info.Name != "calc" {
		t.Errorf("Name mismatch: got %q want %q", info.Name, "calc")
	}
	if info.Description != "a calculator" {
		t.Errorf("Description mismatch: got %q want %q", info.Description, "a calculator")
	}
	if info.Params == nil {
		t.Fatal("Params is nil")
	}
	if info.Params.Type != "object" {
		t.Errorf("Params.Type mismatch: got %q want %q", info.Params.Type, "object")
	}
	if info.Params.Properties == nil {
		t.Error("Params.Properties is nil")
	}
	if len(info.Params.Required) != 1 || info.Params.Required[0] != "x" {
		t.Errorf("Params.Required mismatch: got %v want [x]", info.Params.Required)
	}
	if len(info.Tags) != 1 || info.Tags[0] != "math" {
		t.Errorf("Tags mismatch: got %v want [math]", info.Tags)
	}
}

func TestActionToTool_Invoke_JSON(t *testing.T) {
	exec := &mockActionExecutor{
		result: &action.ActionResult{OK: true, Status: "success", Result: 42},
	}
	a := newTestAction(exec)
	tool := ActionToTool(a)

	out, err := tool.Invoke(context.Background(), `{"x":1,"y":"foo"}`)
	if err != nil {
		t.Fatalf("Invoke returned unexpected error: %v", err)
	}
	// Verify the executor received the parsed args.
	if got := exec.lastInput["x"]; got != float64(1) {
		t.Errorf("executor received x=%v want 1", got)
	}
	if got := exec.lastInput["y"]; got != "foo" {
		t.Errorf("executor received y=%v want %q", got, "foo")
	}
	// Verify the result is JSON-serialized.
	var result action.ActionResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("failed to unmarshal output: %v", err)
	}
	if !result.OK {
		t.Error("expected OK=true in result")
	}
	if result.Status != "success" {
		t.Errorf("expected Status %q, got %q", "success", result.Status)
	}
}

func TestActionToTool_Invoke_RawString(t *testing.T) {
	exec := &mockActionExecutor{
		result: &action.ActionResult{OK: true, Status: "success", Result: "ok"},
	}
	a := newTestAction(exec)
	tool := ActionToTool(a)

	raw := "not a json object"
	out, err := tool.Invoke(context.Background(), raw)
	if err != nil {
		t.Fatalf("Invoke returned unexpected error: %v", err)
	}
	// Verify the executor received the raw string under "input".
	if got := exec.lastInput["input"]; got != raw {
		t.Errorf("executor received input=%v want %q", got, raw)
	}
	// Verify the result is JSON-serialized.
	var result action.ActionResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("failed to unmarshal output: %v", err)
	}
	if result.Result != "ok" {
		t.Errorf("expected Result %q, got %v", "ok", result.Result)
	}
}

func TestActionToTool_Invoke_Error(t *testing.T) {
	sentinel := errors.New("boom")
	exec := &mockActionExecutor{err: sentinel}
	a := newTestAction(exec)
	tool := ActionToTool(a)

	_, err := tool.Invoke(context.Background(), `{}`)
	if err == nil {
		t.Fatal("Invoke expected error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("expected wrapped sentinel error, got %v", err)
	}
	if !strings.Contains(err.Error(), "calc") {
		t.Errorf("expected error to mention tool name %q, got %q", "calc", err.Error())
	}
	if !strings.Contains(err.Error(), "execution failed") {
		t.Errorf("expected error to mention 'execution failed', got %q", err.Error())
	}
}

func TestActionToTool_Info_NilSchema(t *testing.T) {
	exec := &mockActionExecutor{}
	a := &action.Action{
		Name:        "noop",
		Description: "no schema",
		Executor:    exec,
	}
	tool := ActionToTool(a)

	info, err := tool.Info(context.Background())
	if err != nil {
		t.Fatalf("Info returned unexpected error: %v", err)
	}
	if info.Params != nil {
		t.Errorf("expected nil Params for nil schema, got %+v", info.Params)
	}
}
