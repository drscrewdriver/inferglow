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

package actions

import (
	"context"
	"math"
	"testing"

	"github.com/inferglow/action"
)

func TestCalculateBasicOps(t *testing.T) {
	cases := []struct {
		expr string
		want float64
	}{
		{"2 + 3", 5},
		{"10 - 4", 6},
		{"6 * 7", 42},
		{"20 / 4", 5},
		{"17 % 5", 2},
		{"2 + 3 * 4", 14},      // precedence: 2 + (3*4)
		{"(2 + 3) * 4", 20},    // parentheses
		{"-5 + 3", -2},         // unary minus
		{"+5", 5},              // unary plus
		{"2 ** 10", 1024},      // power
		{"2 ** 3 ** 2", 64},    // left-assoc power (Go AST): (2^3)^2=64
		{"(2 ** 3) ** 2", 64},  // explicit grouping
		{"2 ** (3 ** 2)", 512}, // right-assoc via parentheses: 2^(3^2)=512
		{"3.14 * 2", 6.28},
		{"1.5 + 2.5", 4},
		{"100 / 3", 100.0 / 3.0},
	}
	for _, tc := range cases {
		t.Run(tc.expr, func(t *testing.T) {
			got, err := Calculate(tc.expr)
			if err != nil {
				t.Fatalf("Calculate(%q) error: %v", tc.expr, err)
			}
			if math.Abs(got-tc.want) > 1e-9 {
				t.Errorf("Calculate(%q) = %v, want %v", tc.expr, got, tc.want)
			}
		})
	}
}

func TestCalculateErrors(t *testing.T) {
	cases := []struct {
		expr string
	}{
		{""},          // empty
		{"2 / 0"},     // division by zero
		{"5 % 0"},     // modulo by zero
		{"1 + "},      // incomplete
		{"foo"},       // identifier (not allowed)
		{"print(1)"},  // function call (not allowed)
		{"1 + bar"},   // identifier in expression
		{"1 ? 2 : 3"}, // unsupported syntax
		{"`hello`"},   // string literal
		{"[1, 2, 3]"}, // composite literal
		{"1.2.3"},     // malformed number
	}
	for _, tc := range cases {
		t.Run(tc.expr, func(t *testing.T) {
			_, err := Calculate(tc.expr)
			if err == nil {
				t.Errorf("Calculate(%q) expected error, got nil", tc.expr)
			}
		})
	}
}

func TestCalculatorSpec(t *testing.T) {
	if CalculatorSpec.SideEffectLevel != action.SideEffectNone {
		t.Errorf("SideEffectLevel = %q, want %q", CalculatorSpec.SideEffectLevel, action.SideEffectNone)
	}
	if CalculatorSpec.ApprovalRequired {
		t.Errorf("ApprovalRequired = true, want false")
	}
	if CalculatorSpec.SandboxRequired {
		t.Errorf("SandboxRequired = true, want false")
	}
	if CalculatorSpec.ActionID != CalculatorActionID {
		t.Errorf("ActionID = %q, want %q", CalculatorSpec.ActionID, CalculatorActionID)
	}
}

func TestCalculatorExecutorSuccess(t *testing.T) {
	a := NewCalculatorAction()
	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"expression": "2 + 3 * 4",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	got, ok := res.Result.(float64)
	if !ok {
		t.Fatalf("Result not float64: %T", res.Result)
	}
	if got != 14 {
		t.Errorf("Result = %v, want 14", got)
	}
}

func TestCalculatorExecutorError(t *testing.T) {
	a := NewCalculatorAction()
	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"expression": "2 / 0",
	})
	if err != nil {
		t.Fatalf("Execute returned infra error: %v", err)
	}
	if res.OK {
		t.Errorf("expected OK=false, got %+v", res)
	}
	if res.Status != "error" {
		t.Errorf("Status = %q, want error", res.Status)
	}
}

func TestCalculatorActionRegistration(t *testing.T) {
	r := action.NewRegistry()
	if err := r.Register(NewCalculatorAction()); err != nil {
		t.Fatalf("Register error: %v", err)
	}
	if !r.Has(CalculatorActionID) {
		t.Errorf("registry missing %q", CalculatorActionID)
	}
}
