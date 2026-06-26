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

package eval

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/inferglow/model"
)

func TestScriptedProvider_BasicSequence(t *testing.T) {
	p := &ScriptedProvider{
		Responses: []ScriptedResponse{
			{Content: "first"},
			{Content: "second"},
			{Content: "third"},
		},
	}

	if p.Name() != "scripted" {
		t.Errorf("Name() = %q, want %q", p.Name(), "scripted")
	}

	ctx := context.Background()
	data, _ := p.GenerateRequestData(ctx, &model.ModelRequest{})

	// First call.
	ch, err := p.RequestModel(ctx, data)
	if err != nil {
		t.Fatal(err)
	}
	chunk := <-ch
	if chunk.Delta != "first" {
		t.Errorf("call 1: got %q, want %q", chunk.Delta, "first")
	}

	// Second call.
	ch, err = p.RequestModel(ctx, data)
	if err != nil {
		t.Fatal(err)
	}
	chunk = <-ch
	if chunk.Delta != "second" {
		t.Errorf("call 2: got %q, want %q", chunk.Delta, "second")
	}

	// Third call.
	ch, err = p.RequestModel(ctx, data)
	if err != nil {
		t.Fatal(err)
	}
	chunk = <-ch
	if chunk.Delta != "third" {
		t.Errorf("call 3: got %q, want %q", chunk.Delta, "third")
	}

	// Fourth call: should repeat last response.
	ch, err = p.RequestModel(ctx, data)
	if err != nil {
		t.Fatal(err)
	}
	chunk = <-ch
	if chunk.Delta != "third" {
		t.Errorf("call 4 (repeat): got %q, want %q", chunk.Delta, "third")
	}

	if p.CallCount() != 4 {
		t.Errorf("CallCount() = %d, want 4", p.CallCount())
	}
}

func TestScriptedProvider_WithDelay(t *testing.T) {
	p := &ScriptedProvider{
		Responses:     []ScriptedResponse{{Content: "delayed"}},
		ResponseDelay: 50 * time.Millisecond,
	}

	ctx := context.Background()
	data, _ := p.GenerateRequestData(ctx, &model.ModelRequest{})

	start := time.Now()
	ch, err := p.RequestModel(ctx, data)
	if err != nil {
		t.Fatal(err)
	}
	<-ch
	elapsed := time.Since(start)
	if elapsed < 40*time.Millisecond {
		t.Errorf("expected at least 40ms delay, got %s", elapsed)
	}
}

func TestRunner_DirectResponse(t *testing.T) {
	suite := Suite{
		Name: "direct",
		Cases: []Case{
			{
				Name:  "hello",
				Input: "Hi there",
				Expect: Expectation{
					Contains: []string{"hello"},
				},
			},
		},
	}

	runner := &Runner{}
	report, err := runner.Run(context.Background(), suite)
	if err != nil {
		t.Fatal(err)
	}

	if report.Total != 1 {
		t.Errorf("Total = %d, want 1", report.Total)
	}
	if report.Passed != 1 {
		t.Errorf("Passed = %d, want 1", report.Passed)
	}
	if report.Failed != 0 {
		t.Errorf("Failed = %d, want 0", report.Failed)
	}
}

func TestRunner_ToolCallSequence(t *testing.T) {
	suite := Suite{
		Name: "tools",
		Cases: []Case{
			{
				Name:  "calc",
				Input: "What is 2+2?",
				Tools: []ToolStub{
					{Name: "calculator", Description: "calculates", Response: 4},
				},
				Expect: Expectation{
					ToolSequence: []string{"calculator"},
					Contains:     []string{"ok"},
				},
			},
		},
	}

	runner := &Runner{}
	report, err := runner.Run(context.Background(), suite)
	if err != nil {
		t.Fatal(err)
	}

	if report.Total != 1 {
		t.Fatalf("Total = %d, want 1", report.Total)
	}
	res := report.Results[0]
	if !res.Pass {
		t.Errorf("case should pass, errors: %v", res.Errors)
	}
	if len(res.ToolCalls) == 0 {
		t.Error("expected tool calls, got none")
	}
}

func TestRunner_MultipleCases(t *testing.T) {
	suite := Suite{
		Name: "multi",
		Cases: []Case{
			{
				Name:  "pass",
				Input: "hello",
				Expect: Expectation{
					Contains: []string{"ok"},
				},
			},
			{
				Name:  "fail",
				Input: "hello",
				Expect: Expectation{
					// The auto-synthesized response is "ok" (from the provider).
					// NotContains "ok" will fail, producing a failing case.
					NotContains: []string{"ok"},
				},
			},
		},
	}

	runner := &Runner{}
	report, err := runner.Run(context.Background(), suite)
	if err != nil {
		t.Fatal(err)
	}

	if report.Total != 2 {
		t.Fatalf("Total = %d, want 2", report.Total)
	}
	if report.Passed != 1 {
		t.Errorf("Passed = %d, want 1", report.Passed)
	}
	if report.Failed != 1 {
		t.Errorf("Failed = %d, want 1", report.Failed)
	}
}

func TestRunner_EmptySuite(t *testing.T) {
	runner := &Runner{}
	report, err := runner.Run(context.Background(), Suite{Name: "empty"})
	if err != nil {
		t.Fatal(err)
	}
	if report.Total != 0 {
		t.Errorf("Total = %d, want 0", report.Total)
	}
}

func TestRunner_ParallelExecution(t *testing.T) {
	suite := Suite{
		Name:        "parallel",
		Parallelism: 3,
		Cases: []Case{
			{Name: "c1", Input: "a", Expect: Expectation{Contains: []string{"ok"}}},
			{Name: "c2", Input: "b", Expect: Expectation{Contains: []string{"ok"}}},
			{Name: "c3", Input: "c", Expect: Expectation{Contains: []string{"ok"}}},
		},
	}

	runner := &Runner{}
	report, err := runner.Run(context.Background(), suite)
	if err != nil {
		t.Fatal(err)
	}
	if report.Total != 3 {
		t.Fatalf("Total = %d, want 3", report.Total)
	}
	if report.Passed != 3 {
		t.Errorf("Passed = %d, want 3", report.Passed)
	}
}

func TestReport_FormatText(t *testing.T) {
	r := &Report{
		Suite:      "test-suite",
		Total:      2,
		Passed:     1,
		Failed:     1,
		P50Latency: 10 * time.Millisecond,
		P95Latency: 20 * time.Millisecond,
		Results: []CaseResult{
			{CaseName: "pass-case", Pass: true, Response: "hello world", Latency: 10 * time.Millisecond},
			{CaseName: "fail-case", Pass: false, Response: "bad", Latency: 20 * time.Millisecond, Errors: []string{"missing expected"}},
		},
	}

	var buf bytes.Buffer
	r.FormatText(&buf)
	out := buf.String()

	if !strings.Contains(out, "test-suite") {
		t.Error("report should contain suite name")
	}
	if !strings.Contains(out, "PASS") {
		t.Error("report should contain PASS")
	}
	if !strings.Contains(out, "FAIL") {
		t.Error("report should contain FAIL")
	}
	if !strings.Contains(out, "missing expected") {
		t.Error("report should contain error details")
	}
}

func TestReport_FormatJSON(t *testing.T) {
	r := &Report{
		Suite:  "json-test",
		Total:  1,
		Passed: 1,
		Results: []CaseResult{
			{CaseName: "c1", Pass: true, Response: "ok"},
		},
	}

	var buf bytes.Buffer
	if err := r.FormatJSON(&buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "json-test") {
		t.Error("JSON report should contain suite name")
	}
}

func TestReport_ExitCode(t *testing.T) {
	r := &Report{Passed: 3, Failed: 0}
	if r.ExitCode() != 0 {
		t.Errorf("ExitCode() = %d, want 0", r.ExitCode())
	}

	r.Failed = 1
	if r.ExitCode() != 1 {
		t.Errorf("ExitCode() = %d, want 1", r.ExitCode())
	}
}

func TestEvaluateAssertions(t *testing.T) {
	// Contains check.
	errs := evaluateAssertions(Expectation{Contains: []string{"hello"}}, "hello world", nil)
	if len(errs) != 0 {
		t.Errorf("unexpected errors: %v", errs)
	}

	errs = evaluateAssertions(Expectation{Contains: []string{"xyz"}}, "hello world", nil)
	if len(errs) != 1 {
		t.Errorf("expected 1 error, got %d: %v", len(errs), errs)
	}

	// NotContains check.
	errs = evaluateAssertions(Expectation{NotContains: []string{"bad"}}, "hello world", nil)
	if len(errs) != 0 {
		t.Errorf("unexpected errors: %v", errs)
	}

	errs = evaluateAssertions(Expectation{NotContains: []string{"hello"}}, "hello world", nil)
	if len(errs) != 1 {
		t.Errorf("expected 1 error, got %d: %v", len(errs), errs)
	}

	// ToolSequence check.
	errs = evaluateAssertions(Expectation{ToolSequence: []string{"a", "b"}}, "ok", []string{"a", "c", "b"})
	if len(errs) != 0 {
		t.Errorf("subsequence should match: %v", errs)
	}

	errs = evaluateAssertions(Expectation{ToolSequence: []string{"a", "b", "c"}}, "ok", []string{"a", "b"})
	if len(errs) != 1 {
		t.Errorf("expected 1 error for short sequence: %v", errs)
	}
}

func TestMatchToolSequence(t *testing.T) {
	tests := []struct {
		expected []string
		actual   []string
		want     bool
	}{
		{nil, nil, true},
		{[]string{"a"}, []string{"a"}, true},
		{[]string{"a", "b"}, []string{"a", "c", "b"}, true},
		{[]string{"a", "b", "c"}, []string{"a", "b"}, false},
		{[]string{"x"}, []string{"a", "b"}, false},
	}

	for _, tt := range tests {
		got := matchToolSequence(tt.expected, tt.actual)
		if got != tt.want {
			t.Errorf("matchToolSequence(%v, %v) = %v, want %v", tt.expected, tt.actual, got, tt.want)
		}
	}
}
