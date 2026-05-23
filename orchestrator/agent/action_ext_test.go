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

package agent

import (
	"context"
	"testing"

	"github.com/inferglow/action"
)

func TestActionExtension_Register(t *testing.T) {
	ext := NewActionExtension()

	// Create action using action.New()
	actionInst, err := action.New("test_action", "A test action",
		func(ctx context.Context, input map[string]any) (any, error) {
			return "hello", nil
		})
	if err != nil {
		t.Fatalf("Failed to create action: %v", err)
	}

	err = ext.Register(actionInst)
	if err != nil {
		t.Fatalf("Failed to register action: %v", err)
	}

	actions := ext.ListActions()

	if len(actions) != 1 {
		t.Fatalf("Expected 1 action, got %d", len(actions))
	}
	if actions[0]["name"] != "test_action" {
		t.Errorf("Action name mismatch: got %v", actions[0]["name"])
	}
}

func TestActionExtension_ExecutesSuccessfully(t *testing.T) {
	ext := NewActionExtension()

	actionInst, err := action.New("double", "Doubles the input value",
		func(ctx context.Context, input map[string]any) (any, error) {
			val, ok := input["value"].(float64)
			if !ok {
				return nil, nil
			}
			return val * 2, nil
		})
	if err != nil {
		t.Fatalf("Failed to create action: %v", err)
	}

	err = ext.Register(actionInst)
	if err != nil {
		t.Fatalf("Failed to register action: %v", err)
	}

	result, err := ext.Execute(context.Background(), "double", map[string]any{"value": float64(21)})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.OK {
		t.Errorf("Expected OK=true, status=%s", result.Status)
	}
	val, ok := result.Result.(float64)
	if !ok || val != 42 {
		t.Errorf("Expected result 42, got %v", result.Result)
	}
}

func TestActionExtension_ExecuteNotFound(t *testing.T) {
	ext := NewActionExtension()

	_, err := ext.Execute(context.Background(), "nonexistent", map[string]any{})
	if err == nil {
		t.Fatal("Expected error for nonexistent action, got nil")
	}
}
