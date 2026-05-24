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

import (
	"regexp"
	"strings"
)

// thinkState enumerates the streaming state-machine phases for
// LeadingThinkNormalizer.
type thinkState int

const (
	thinkStateUnknown   thinkState = 0 // initial: scanning for a leading <think>
	thinkStateReasoning thinkState = 1 // inside a <think>...</think> block
	thinkStateAnswer    thinkState = 2 // outside think blocks; plain answer text
)

const thinkOpenTag = "<think>"   // case-insensitive when matched
const thinkCloseTag = "</think>" // case-insensitive when matched

// thinkBlockRe matches a single <think>...</think> block case-insensitively.
// The non-greedy (.*?) inner capture preserves content across newlines.
var thinkBlockRe = regexp.MustCompile(`(?is)<think>(.*?)</think>`)

// LeadingThinkNormalizer is a streaming state machine that splits an
// incrementally-fed text stream into reasoning deltas (inside <think> tags)
// and answer deltas (outside <think> tags). It handles three corner cases
// that naive string splitting misses:
//
//  1. Partial <think> prefixes arriving across chunk boundaries
//     (buffered up to len("<think>") characters).
//  2. Case-insensitive <think>/<THINK> matching.
//  3. Trailing answer text glued to a closing </think> tag
//     (e.g. "</think>bar") — the answer is buffered and emitted on the
//     next FeedDelta call.
//
// FeedDelta returns ONE event per call. Possible event types:
//   - "delta"           — answer text, pass through to the user.
//   - "reasoning_delta" — reasoning text (inside <think>), route to a
//     reasoning channel.
//   - "reasoning_done"  — a </think> close tag was observed; reasoning
//     phase ended.
//   - ""                — the input was buffered (partial prefix); the
//     caller should not emit anything yet.
type LeadingThinkNormalizer struct {
	state         thinkState
	buffer        string // partial <think> prefix (unknown state only)
	pendingAnswer string // trailing text after </think>, flushed next call
}

// FeedDelta processes one streaming chunk and returns a single event.
// See LeadingThinkNormalizer for the event-type contract.
func (n *LeadingThinkNormalizer) FeedDelta(delta string) (eventType string, payload string) {
	switch n.state {
	case thinkStateUnknown:
		return n.feedUnknown(delta)
	case thinkStateReasoning:
		return n.feedReasoning(delta)
	default: // thinkStateAnswer
		return n.feedAnswer(delta)
	}
}

// feedUnknown buffers up to len(thinkOpenTag) characters to detect a
// leading <think> tag. If the accumulated input is a prefix of
// thinkOpenTag it keeps buffering; once the full tag arrives it switches
// to the reasoning state and immediately delegates the remainder to
// feedReasoning so that a complete <think>...</think> block arriving in
// a single delta is handled correctly. Any non-matching input is emitted
// as a delta and the state advances to answer.
func (n *LeadingThinkNormalizer) feedUnknown(delta string) (string, string) {
	combined := n.buffer + delta
	n.buffer = ""
	lower := strings.ToLower(combined)

	if strings.HasPrefix(lower, thinkOpenTag) {
		remainder := combined[len(thinkOpenTag):]
		n.state = thinkStateReasoning
		// The remainder may contain </think> — delegate to feedReasoning
		// so close tags in the same delta are processed.
		return n.feedReasoning(remainder)
	}
	// Still a partial prefix of <think>? Keep buffering.
	if len(combined) < len(thinkOpenTag) && strings.HasPrefix(thinkOpenTag, lower) {
		n.buffer = combined
		return "", ""
	}
	// Not a <think> prefix — emit as plain answer and advance.
	n.state = thinkStateAnswer
	if combined == "" {
		return "", ""
	}
	return "delta", combined
}

// feedReasoning looks for a case-insensitive </think> close tag. When
// found, it buffers any trailing answer text and switches to the answer
// state. If there is reasoning content before the close tag, it is
// returned as a reasoning_delta (the reasoning_done marker is implicit
// in the state transition). Otherwise a reasoning_done event is returned.
// If no close tag is found the delta is forwarded as a reasoning_delta.
func (n *LeadingThinkNormalizer) feedReasoning(delta string) (string, string) {
	closeIdx := strings.Index(strings.ToLower(delta), thinkCloseTag)
	if closeIdx == -1 {
		return "reasoning_delta", delta
	}
	before := delta[:closeIdx]
	after := delta[closeIdx+len(thinkCloseTag):]
	n.state = thinkStateAnswer
	n.pendingAnswer = after
	if before != "" {
		// Reasoning content precedes the close tag — emit it as
		// reasoning_delta. The state has already transitioned to
		// answer; the trailing text is buffered in pendingAnswer.
		return "reasoning_delta", before
	}
	return "reasoning_done", ""
}

// feedAnswer flushes any buffered trailing answer text first, then
// inspects the new delta for a fresh <think> opening tag. If found it
// re-enters the reasoning state; otherwise the combined text is emitted
// as a delta.
func (n *LeadingThinkNormalizer) feedAnswer(delta string) (string, string) {
	combined := n.pendingAnswer + delta
	n.pendingAnswer = ""
	if combined == "" {
		return "", ""
	}
	lower := strings.ToLower(combined)
	if strings.HasPrefix(lower, thinkOpenTag) {
		remainder := combined[len(thinkOpenTag):]
		n.state = thinkStateReasoning
		return n.feedReasoning(remainder)
	}
	return "delta", combined
}

// FeedDone performs non-streaming extraction of <think>...</think>
// blocks from a complete content string. It returns the joined
// reasoning content (blocks separated by "\n") and the answer text
// with all think tags removed and surrounding whitespace trimmed.
// Matching is case-insensitive. If no think tags are present the
// content is returned unchanged with an empty reasoning string.
func (n *LeadingThinkNormalizer) FeedDone(content string) (reasoning string, answer string) {
	return normalizeThinkingTags(content)
}

// --- internal helpers (moved from openai.go) ---

// hasThinkingTags reports whether content contains a <think>...</think>
// block. Matching is case-insensitive. Used by BroadcastResponse to
// detect reasoning wrapped in content tags.
func hasThinkingTags(content string) bool {
	return thinkBlockRe.MatchString(content)
}

// normalizeThinkingTags extracts <think>...</think> content from the
// given string. Returns (reasoning, cleaned) where reasoning is the
// concatenated content inside think blocks (multiple blocks joined by
// "\n") and cleaned is the original content with think tags removed
// and leading whitespace trimmed.
//
// If content has no think tags, returns ("", content) unchanged.
// Matching is case-insensitive.
func normalizeThinkingTags(content string) (reasoning string, cleaned string) {
	if !hasThinkingTags(content) {
		return "", content
	}
	var reasoningParts []string
	cleaned = thinkBlockRe.ReplaceAllStringFunc(content, func(match string) string {
		subs := thinkBlockRe.FindStringSubmatch(match)
		inner := subs[1]
		if inner != "" {
			reasoningParts = append(reasoningParts, inner)
		}
		return ""
	})
	cleaned = strings.TrimSpace(cleaned)
	reasoning = strings.Join(reasoningParts, "\n")
	return reasoning, cleaned
}
