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
	"strings"
	"testing"

	"github.com/inferglow/action"
	"github.com/inferglow/model"
	"github.com/inferglow/session"
)

// TestFormatToolResult_HookRunsBeforeTruncation guards the R10 semantics: the
// per-run toolResultHook (orthogonal improvement pass) sees the FULL result —
// its output is what gets size-capped, not its input.
func TestFormatToolResult_HookRunsBeforeTruncation(t *testing.T) {
	sess := session.NewSession("hook-test", 10000)
	ag := New(sess, NewActionExtension(), &mockModelRequester{})
	e := ag.engine

	// No hook: passthrough.
	raw := "hello"
	if got := e.formatToolResult("t", &action.ActionResult{OK: true, Result: raw}); !strings.Contains(got, "hello") {
		t.Fatalf("passthrough lost content: %q", got)
	}

	// Hook sees the full (oversized) input and its shrinkage is what the cap
	// then applies to — assert the hook received everything.
	var seenLen int
	e.toolResultHook = func(name, content string) string {
		seenLen = len(content)
		return strings.ReplaceAll(content, "noise", "")
	}
	big := strings.Repeat("noise", 10_000) // 50KB > 16KB cap
	got := e.formatToolResult("t", &action.ActionResult{OK: true, Result: big})
	// JSON marshaling adds the surrounding quotes; the point is the hook saw
	// the FULL input (>> the 16KB cap), not a truncated one.
	if seenLen < len(big) {
		t.Fatalf("hook saw %d bytes, want >= full %d (hook must run BEFORE truncation)", seenLen, len(big))
	}
	if strings.Contains(got, "noise") || strings.Contains(got, "truncated") {
		// 10KB after cleaning < 16KB cap: nothing truncated, noise gone.
		t.Fatalf("hook output not applied: %q", got[:80])
	}

	// A no-op hook on an oversized result still truncates (cap intact).
	e.toolResultHook = func(name, content string) string { return content }
	got = e.formatToolResult("t", &action.ActionResult{OK: true, Result: big})
	if !strings.Contains(got, "[truncated") {
		t.Fatal("size cap disappeared when hook installed")
	}
}

// TestWithToolResultHook_RunOption wires the option through Agent.Run.
func TestWithToolResultHook_RunOption(t *testing.T) {
	sess := session.NewSession("hook-opt-test", 10000)
	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 1)
			ch <- &model.StreamChunk{Delta: `{"next_action":"response","final_response":"done"}`, IsDone: true}
			close(ch)
			return ch, nil
		},
	}
	ag := New(sess, NewActionExtension(), mockReq)
	if ag.engine.toolResultHook != nil {
		t.Fatal("hook must default to nil")
	}
	_, err := ag.Run(context.Background(), "hi", WithToolResultHook(func(name, content string) string {
		return content
	}))
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if ag.engine.toolResultHook == nil {
		t.Fatal("WithToolResultHook did not propagate to the engine")
	}
}
