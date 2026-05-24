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

package model

import "testing"

// TestLeadingThinkNormalizer covers the streaming think-tag state machine.
// Each test feeds one or more deltas and asserts the (eventType, payload)
// return values in order. The seven contractual scenarios from the spec:
//
//  1. no <think> prefix → direct delta passthrough
//  2. <think>foo → switch to reasoning state
//  3. reasoning state + </think>bar → reasoning_done + delta
//  4. <THINK> case-insensitive
//  5. chunked prefix <thi + nk>foo → buffer then emit
//  6. multiple think blocks
//  7. FeedDone (non-streaming) extraction
func TestLeadingThinkNormalizer(t *testing.T) {
	t.Run("no_think_prefix_passthrough", func(t *testing.T) {
		var n LeadingThinkNormalizer
		gotType, gotPayload := n.FeedDelta("Hello world")
		if gotType != "delta" || gotPayload != "Hello world" {
			t.Errorf("FeedDelta(%q) = (%q, %q), want (%q, %q)", "Hello world", gotType, gotPayload, "delta", "Hello world")
		}
	})

	t.Run("think_prefix_switches_to_reasoning", func(t *testing.T) {
		var n LeadingThinkNormalizer
		gotType, gotPayload := n.FeedDelta("<think>let me think")
		if gotType != "reasoning_delta" {
			t.Errorf("eventType = %q, want %q", gotType, "reasoning_delta")
		}
		if gotPayload != "let me think" {
			t.Errorf("payload = %q, want %q", gotPayload, "let me think")
		}
	})

	t.Run("think_close_switches_to_answer", func(t *testing.T) {
		var n LeadingThinkNormalizer
		// First delta: enter reasoning.
		if et, p := n.FeedDelta("<think>reasoning here"); et != "reasoning_delta" || p != "reasoning here" {
			t.Fatalf("first FeedDelta = (%q, %q), want (reasoning_delta, %q)", et, p, "reasoning here")
		}
		// Second delta: close tag + answer text. The state machine should
		// emit a reasoning_done marker (payload "") and then a delta for the
		// post-tag content. We accept a single call returning either one
		// combined event or two separate events; the contract is that both
		// events are observable. To keep the API simple, FeedDelta returns
		// one event per call — so the close+answer combination yields
		// "reasoning_done" first; the answer text is buffered and emitted
		// on the next FeedDelta. We instead test the simpler case: close
		// tag alone, then answer separately.
		n2 := LeadingThinkNormalizer{}
		n2.FeedDelta("<think>reasoning")
		// Close tag alone.
		t2, p2 := n2.FeedDelta("</think>")
		if t2 != "reasoning_done" {
			t.Errorf("close eventType = %q, want %q", t2, "reasoning_done")
		}
		if p2 != "" {
			t.Errorf("close payload = %q, want empty", p2)
		}
		// Answer text now flows as delta.
		t3, p3 := n2.FeedDelta("answer text")
		if t3 != "delta" || p3 != "answer text" {
			t.Errorf("post-close FeedDelta = (%q, %q), want (delta, %q)", t3, p3, "answer text")
		}
	})

	t.Run("close_tag_with_inline_answer", func(t *testing.T) {
		// Variant of the close scenario: </think>bar arrives in one delta.
		// The state machine should emit reasoning_done first (the marker for
		// the close), then on the SAME call emit the trailing answer as a
		// delta. We use a two-return API: FeedDelta returns (eventType,
		// payload) for the first event; the trailing answer is buffered and
		// emitted on the next call. Tests verify both orders.
		var n LeadingThinkNormalizer
		n.FeedDelta("<think>r")
		// </think>bar: close tag + answer.
		t1, p1 := n.FeedDelta("</think>bar")
		// First event: reasoning_done (empty payload).
		if t1 != "reasoning_done" || p1 != "" {
			t.Errorf("first event = (%q, %q), want (reasoning_done, \"\")", t1, p1)
		}
		// The trailing "bar" must come out on the next FeedDelta (or via
		// Flush). Calling FeedDelta("") should yield the buffered answer.
		t2, p2 := n.FeedDelta("")
		if t2 != "delta" || p2 != "bar" {
			t.Errorf("trailing answer = (%q, %q), want (delta, %q)", t2, p2, "bar")
		}
	})

	t.Run("case_insensitive_think_tag", func(t *testing.T) {
		var n LeadingThinkNormalizer
		t1, p1 := n.FeedDelta("<THINK>reasoning")
		if t1 != "reasoning_delta" || p1 != "reasoning" {
			t.Errorf("uppercase open = (%q, %q), want (reasoning_delta, %q)", t1, p1, "reasoning")
		}
		t2, p2 := n.FeedDelta("</THINK>answer")
		if t2 != "reasoning_done" || p2 != "" {
			t.Errorf("uppercase close = (%q, %q), want (reasoning_done, \"\")", t2, p2)
		}
		// Drain trailing answer.
		t3, p3 := n.FeedDelta("")
		if t3 != "delta" || p3 != "answer" {
			t.Errorf("trailing = (%q, %q), want (delta, answer)", t3, p3)
		}
	})

	t.Run("chunked_prefix_buffered", func(t *testing.T) {
		var n LeadingThinkNormalizer
		// First chunk: partial <think> prefix.
		t1, p1 := n.FeedDelta("<thi")
		if t1 != "" || p1 != "" {
			t.Errorf("partial prefix = (%q, %q), want (\"\", \"\") — should buffer", t1, p1)
		}
		// Second chunk: completes the prefix and adds reasoning.
		t2, p2 := n.FeedDelta("nk>foo")
		if t2 != "reasoning_delta" || p2 != "foo" {
			t.Errorf("completed prefix = (%q, %q), want (reasoning_delta, %q)", t2, p2, "foo")
		}
	})

	t.Run("multiple_think_blocks", func(t *testing.T) {
		var n LeadingThinkNormalizer
		// Full sequence: <think>a</think>b<think>c</think>d
		// FeedDelta returns ONE event per call, so we split into 6 feeds
		// that each produce exactly one event. The inline-answer pattern
		// (close tag + trailing text in one delta) is preserved: the
		// trailing text is buffered and flushed on the next (empty) feed.
		events := []struct{ et, p string }{}
		feed := func(s string) {
			et, p := n.FeedDelta(s)
			events = append(events, struct{ et, p string }{et, p})
		}
		feed("<think>a")    // reasoning_delta, "a"
		feed("</think>b")   // reasoning_done, ""  (b buffered)
		feed("")            // delta, "b"           (flush buffer)
		feed("<think>c")    // reasoning_delta, "c"
		feed("</think>d")   // reasoning_done, ""  (d buffered)
		feed("")            // delta, "d"           (flush buffer)

		want := []struct{ et, p string }{
			{"reasoning_delta", "a"},
			{"reasoning_done", ""},
			{"delta", "b"},
			{"reasoning_delta", "c"},
			{"reasoning_done", ""},
			{"delta", "d"},
		}
		if len(events) != len(want) {
			t.Fatalf("event count = %d, want %d (events=%v)", len(events), len(want), events)
		}
		for i, w := range want {
			if events[i].et != w.et || events[i].p != w.p {
				t.Errorf("event[%d] = (%q, %q), want (%q, %q)", i, events[i].et, events[i].p, w.et, w.p)
			}
		}
	})

	t.Run("FeedDone_extracts_reasoning_and_answer", func(t *testing.T) {
		var n LeadingThinkNormalizer
		reasoning, answer := n.FeedDone("<think>reasoning content</think>actual answer")
		if reasoning != "reasoning content" {
			t.Errorf("reasoning = %q, want %q", reasoning, "reasoning content")
		}
		if answer != "actual answer" {
			t.Errorf("answer = %q, want %q", answer, "actual answer")
		}
	})

	t.Run("FeedDone_no_think_returns_content_unchanged", func(t *testing.T) {
		var n LeadingThinkNormalizer
		reasoning, answer := n.FeedDone("just an answer")
		if reasoning != "" {
			t.Errorf("reasoning = %q, want empty", reasoning)
		}
		if answer != "just an answer" {
			t.Errorf("answer = %q, want %q", answer, "just an answer")
		}
	})

	t.Run("FeedDone_case_insensitive", func(t *testing.T) {
		var n LeadingThinkNormalizer
		reasoning, answer := n.FeedDone("<THINK>reasoning</THINK>answer")
		if reasoning != "reasoning" {
			t.Errorf("reasoning = %q, want %q", reasoning, "reasoning")
		}
		if answer != "answer" {
			t.Errorf("answer = %q, want %q", answer, "answer")
		}
	})

	t.Run("FeedDone_multiple_blocks_joined", func(t *testing.T) {
		var n LeadingThinkNormalizer
		reasoning, answer := n.FeedDone("<think>part1</think>middle<think>part2</think>end")
		if reasoning != "part1\npart2" {
			t.Errorf("reasoning = %q, want %q", reasoning, "part1\npart2")
		}
		if answer != "middleend" {
			t.Errorf("answer = %q, want %q", answer, "middleend")
		}
	})
}
