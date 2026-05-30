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

package blocks

import (
	"context"
	"errors"
	"testing"
)

func TestBlockRegistry(t *testing.T) {
	reg := NewBlockRegistry()
	reason := &ReasonBlock{ModelName: "gpt-4"}
	if err := reg.Register(reason, false); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, err := reg.Get("reason")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name() != "reason" {
		t.Errorf("expected 'reason', got %s", got.Name())
	}
}

func TestBlockRegistryDuplicate(t *testing.T) {
	reg := NewBlockRegistry()
	reg.Register(&ReasonBlock{ModelName: "m"}, false)
	err := reg.Register(&ReasonBlock{ModelName: "m"}, false)
	if !errors.Is(err, ErrBlockExists) {
		t.Fatalf("expected ErrBlockExists, got %v", err)
	}
	// replace=true should work.
	if err := reg.Register(&ReasonBlock{ModelName: "m"}, true); err != nil {
		t.Fatalf("replace: %v", err)
	}
}

func TestBlockRegistryNotFound(t *testing.T) {
	reg := NewBlockRegistry()
	_, err := reg.Get("nonexistent")
	if !errors.Is(err, ErrBlockNotFound) {
		t.Fatalf("expected ErrBlockNotFound, got %v", err)
	}
}

func TestBuiltinBlocks(t *testing.T) {
	tests := []struct {
		block FlowBlock
		name  string
	}{
		{&ReasonBlock{ModelName: "m"}, "reason"},
		{&ActBlock{AllowedActions: []string{"bash"}}, "act"},
		{&IntentBlock{ModelName: "m"}, "intent"},
	}
	for _, tt := range tests {
		if tt.block.Name() != tt.name {
			t.Errorf("expected name=%s, got %s", tt.name, tt.block.Name())
		}
		result, err := tt.block.Execute(context.Background(), "test-input")
		if err != nil {
			t.Errorf("%s.Execute: %v", tt.name, err)
		}
		if result == nil {
			t.Errorf("%s.Execute returned nil", tt.name)
		}
	}
}

func TestBuildOperators(t *testing.T) {
	reason := &ReasonBlock{ModelName: "gpt-4"}
	ops, err := reason.BuildOperators(context.Background(), nil)
	if err != nil {
		t.Fatalf("BuildOperators: %v", err)
	}
	if len(ops) != 1 {
		t.Fatalf("expected 1 operator, got %d", len(ops))
	}
	if ops[0].ID != "reason_op" {
		t.Errorf("expected reason_op, got %s", ops[0].ID)
	}
}

func TestExecuteBlueprint(t *testing.T) {
	reg := NewBlockRegistry()
	reg.Register(&ReasonBlock{ModelName: "m"}, false)
	reg.Register(&ActBlock{AllowedActions: []string{"bash"}}, false)

	bp := &BlockBlueprint{
		Blocks: []BlockRef{
			{BlockName: "reason"},
			{BlockName: "act"},
		},
	}

	result, err := reg.ExecuteBlueprint(context.Background(), bp, "initial")
	if err != nil {
		t.Fatalf("ExecuteBlueprint: %v", err)
	}
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestCompileBlueprint(t *testing.T) {
	reg := NewBlockRegistry()
	reg.Register(&ReasonBlock{ModelName: "m"}, false)
	reg.Register(&IntentBlock{ModelName: "m"}, false)

	bp := &BlockBlueprint{
		Blocks: []BlockRef{
			{BlockName: "reason"},
			{BlockName: "intent"},
		},
	}

	ops, err := reg.CompileBlueprint(context.Background(), bp)
	if err != nil {
		t.Fatalf("CompileBlueprint: %v", err)
	}
	if len(ops) != 2 {
		t.Fatalf("expected 2 operators, got %d", len(ops))
	}
}

func TestListBlocks(t *testing.T) {
	reg := NewBlockRegistry()
	reg.Register(&ReasonBlock{}, false)
	reg.Register(&ActBlock{}, false)

	names := reg.List()
	if len(names) != 2 {
		t.Fatalf("expected 2, got %d", len(names))
	}
}
