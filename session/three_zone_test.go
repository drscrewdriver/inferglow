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

package session

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// mkMsg is a small helper for building ChatMessages with content as a string.
func mkMsg(role, content string) ChatMessage {
	return ChatMessage{
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
	}
}

// TestThreeZoneSession_SetImmutablePrefixOnce verifies that
// SetImmutablePrefix succeeds the first time and fails the second time.
func TestThreeZoneSession_SetImmutablePrefixOnce(t *testing.T) {
	s := NewThreeZoneSession("s1", 4096)
	if err := s.SetImmutablePrefix("you are helpful", nil); err != nil {
		t.Fatalf("first SetImmutablePrefix: %v", err)
	}
	if err := s.SetImmutablePrefix("you are helpful", nil); err == nil {
		t.Fatal("second SetImmutablePrefix should fail, got nil error")
	}
}

// TestThreeZoneSession_AddToHistoryAppends verifies AddToHistory appends and
// that BuildPrompt returns the immutable prefix followed by the history.
func TestThreeZoneSession_AddToHistoryAppends(t *testing.T) {
	s := NewThreeZoneSession("s1", 4096)
	if err := s.SetImmutablePrefix("sys", nil); err != nil {
		t.Fatalf("SetImmutablePrefix: %v", err)
	}
	s.AddToHistory(mkMsg("user", "hello"))
	s.AddToHistory(mkMsg("assistant", "hi"))

	prompt := s.BuildPrompt()
	// Expect: [system "sys"] [user "hello"] [assistant "hi"]
	if len(prompt) != 3 {
		t.Fatalf("expected 3 messages, got %d: %+v", len(prompt), prompt)
	}
	if prompt[0].Role != "system" || ContentToString(prompt[0].Content) != "sys" {
		t.Errorf("prompt[0] = %+v, want system/sys", prompt[0])
	}
	if prompt[1].Role != "user" || ContentToString(prompt[1].Content) != "hello" {
		t.Errorf("prompt[1] = %+v, want user/hello", prompt[1])
	}
	if prompt[2].Role != "assistant" || ContentToString(prompt[2].Content) != "hi" {
		t.Errorf("prompt[2] = %+v, want assistant/hi", prompt[2])
	}
}

// TestThreeZoneSession_BuildPromptOrder verifies the full prompt is ordered
// Zone1 + Zone2 + Zone3.
func TestThreeZoneSession_BuildPromptOrder(t *testing.T) {
	s := NewThreeZoneSession("s1", 4096)
	if err := s.SetImmutablePrefix("SYS", nil); err != nil {
		t.Fatalf("SetImmutablePrefix: %v", err)
	}
	s.AddToHistory(mkMsg("user", "h1"))
	s.SetVolatileScratch([]ChatMessage{mkMsg("system", "scratch1")})

	prompt := s.BuildPrompt()
	if len(prompt) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(prompt))
	}
	// Zone 1 first
	if ContentToString(prompt[0].Content) != "SYS" {
		t.Errorf("prompt[0] = %v, want SYS", prompt[0])
	}
	// Zone 2 second
	if ContentToString(prompt[1].Content) != "h1" {
		t.Errorf("prompt[1] = %v, want h1", prompt[1])
	}
	// Zone 3 last
	if ContentToString(prompt[2].Content) != "scratch1" {
		t.Errorf("prompt[2] = %v, want scratch1", prompt[2])
	}
}

// TestThreeZoneSession_ImmutableHashStable verifies the immutable hash is
// stable across calls (as long as the prefix isn't changed).
func TestThreeZoneSession_ImmutableHashStable(t *testing.T) {
	s := NewThreeZoneSession("s1", 4096)
	if err := s.SetImmutablePrefix("sys prompt", []any{
		map[string]any{"name": "tool_a", "parameters": map[string]any{"type": "object"}},
		map[string]any{"name": "tool_b", "parameters": map[string]any{"type": "object"}},
	}); err != nil {
		t.Fatalf("SetImmutablePrefix: %v", err)
	}
	first := s.ImmutableHash()
	if first == "" {
		t.Fatal("ImmutableHash should not be empty")
	}
	for i := 0; i < 20; i++ {
		if got := s.ImmutableHash(); got != first {
			t.Fatalf("iter %d: ImmutableHash = %q, want %q", i, got, first)
		}
	}
}

// TestThreeZoneSession_ImmutableHashSameForSamePrefix verifies two sessions
// with the same prefix produce the same ImmutableHash (cache-shareable).
func TestThreeZoneSession_ImmutableHashSameForSamePrefix(t *testing.T) {
	mkSession := func() *ThreeZoneSession {
		s := NewThreeZoneSession("x", 4096)
		_ = s.SetImmutablePrefix("shared system prompt", []any{
			map[string]any{
				"name":        "calc",
				"description": "calc",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"expr": map[string]any{"type": "string"},
					},
				},
			},
		})
		return s
	}
	s1 := mkSession()
	s2 := mkSession()
	if s1.ImmutableHash() != s2.ImmutableHash() {
		t.Errorf("sessions with same prefix should have same hash\ns1: %s\ns2: %s",
			s1.ImmutableHash(), s2.ImmutableHash())
	}
}

// TestThreeZoneSession_ImmutableHashDiffersForDifferentPrefix verifies the
// hash changes when the system prompt differs.
func TestThreeZoneSession_ImmutableHashDiffersForDifferentPrefix(t *testing.T) {
	s1 := NewThreeZoneSession("a", 4096)
	_ = s1.SetImmutablePrefix("prompt A", nil)
	s2 := NewThreeZoneSession("b", 4096)
	_ = s2.SetImmutablePrefix("prompt B", nil)
	if s1.ImmutableHash() == s2.ImmutableHash() {
		t.Error("sessions with different prefixes should have different hashes")
	}
}

// TestThreeZoneSession_ClearVolatileScratch verifies ClearVolatileScratch
// empties Zone 3.
func TestThreeZoneSession_ClearVolatileScratch(t *testing.T) {
	s := NewThreeZoneSession("s1", 4096)
	_ = s.SetImmutablePrefix("sys", nil)
	s.SetVolatileScratch([]ChatMessage{mkMsg("system", "scratch1"), mkMsg("system", "scratch2")})

	// Verify scratch is present
	prompt := s.BuildPrompt()
	if len(prompt) != 3 { // 1 system + 0 history + 2 scratch
		t.Fatalf("expected 3 messages before clear, got %d", len(prompt))
	}

	s.ClearVolatileScratch()
	prompt = s.BuildPrompt()
	if len(prompt) != 1 { // only the system prompt
		t.Fatalf("expected 1 message after clear, got %d: %+v", len(prompt), prompt)
	}
}

// TestThreeZoneSession_SetVolatileScratchReplaces verifies SetVolatileScratch
// replaces (not appends to) Zone 3.
func TestThreeZoneSession_SetVolatileScratchReplaces(t *testing.T) {
	s := NewThreeZoneSession("s1", 4096)
	_ = s.SetImmutablePrefix("sys", nil)
	s.SetVolatileScratch([]ChatMessage{mkMsg("system", "first")})
	s.SetVolatileScratch([]ChatMessage{mkMsg("system", "second")})
	prompt := s.BuildPrompt()
	if len(prompt) != 2 { // 1 system + 1 scratch
		t.Fatalf("expected 2 messages, got %d", len(prompt))
	}
	if ContentToString(prompt[1].Content) != "second" {
		t.Errorf("scratch should be replaced, got %v", prompt[1])
	}
}

// TestThreeZoneSession_ResizeTriggersWhenOverBudget verifies AddToHistory
// triggers the resize chain when total bytes exceed maxHistoryBytes.
func TestThreeZoneSession_ResizeTriggersWhenOverBudget(t *testing.T) {
	// maxHistoryBytes=10 — small budget so adding one long message triggers resize.
	s := NewThreeZoneSession("s1", 10)
	_ = s.SetImmutablePrefix("sys", nil)

	// Configure a snip strategy that removes the oldest message.
	s.SetResizeStrategies(SnipFromHead(1), nil, nil)

	// Add a first small message, then a long one that triggers resize.
	s.AddToHistory(mkMsg("user", "0123456789"))       // 10 bytes — at budget
	s.AddToHistory(mkMsg("user", "0123456789ABCDEF")) // 16 bytes — over budget

	// After resize, the first message should have been snipped.
	prompt := s.BuildPrompt()
	// Expected: [system "sys"] [user "0123456789ABCDEF"]  (first user snipped)
	if len(prompt) != 2 {
		t.Fatalf("expected 2 messages after resize, got %d: %+v", len(prompt), prompt)
	}
	if prompt[1].Role != "user" || ContentToString(prompt[1].Content) != "0123456789ABCDEF" {
		t.Errorf("prompt[1] = %+v, want user/0123456789ABCDEF", prompt[1])
	}
}

// TestSnipFromHead verifies SnipFromHead removes the oldest N messages.
func TestSnipFromHead(t *testing.T) {
	msgs := []ChatMessage{
		mkMsg("user", "a"),
		mkMsg("assistant", "b"),
		mkMsg("user", "c"),
		mkMsg("assistant", "d"),
	}
	h := SnipFromHead(2)
	got, err := h(msgs, msgs)
	if err != nil {
		t.Fatalf("SnipFromHead error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 messages after snip, got %d", len(got))
	}
	if ContentToString(got[0].Content) != "c" || ContentToString(got[1].Content) != "d" {
		t.Errorf("snip result = %v %v, want c d", got[0], got[1])
	}
}

// TestSnipFromHead_TooMany verifies SnipFromHead returns an error when N is
// greater than or equal to the window length.
func TestSnipFromHead_TooMany(t *testing.T) {
	msgs := []ChatMessage{mkMsg("user", "a")}
	h := SnipFromHead(1)
	if _, err := h(msgs, msgs); err == nil {
		t.Error("expected error when snipping all messages, got nil")
	}
}

// TestPruneLowValue verifies PruneLowValue removes short messages.
func TestPruneLowValue(t *testing.T) {
	msgs := []ChatMessage{
		mkMsg("user", "short"),                 // 5 chars — below threshold
		mkMsg("assistant", "longer message"),   // 14 chars — above threshold
		mkMsg("user", "x"),                     // 1 char — below threshold
		mkMsg("assistant", "another long one"), // 17 chars — above threshold
	}
	h := PruneLowValue(6)
	got, err := h(msgs, msgs)
	if err != nil {
		t.Fatalf("PruneLowValue error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 messages after prune, got %d", len(got))
	}
	if ContentToString(got[0].Content) != "longer message" {
		t.Errorf("got[0] = %v, want longer message", got[0])
	}
	if ContentToString(got[1].Content) != "another long one" {
		t.Errorf("got[1] = %v, want another long one", got[1])
	}
}

// TestSummaryReplace verifies SummaryReplace replaces the entire window with
// a single summary message.
func TestSummaryReplace(t *testing.T) {
	msgs := []ChatMessage{
		mkMsg("user", "old1"),
		mkMsg("assistant", "old2"),
		mkMsg("user", "old3"),
	}
	summary := mkMsg("system", "[summary of old conversation]")
	h := SummaryReplace(summary)
	got, err := h(msgs, msgs)
	if err != nil {
		t.Fatalf("SummaryReplace error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 message after summary, got %d", len(got))
	}
	if ContentToString(got[0].Content) != "[summary of old conversation]" {
		t.Errorf("got[0] = %v, want summary", got[0])
	}
}

// TestThreeZoneSession_ResizeChainOrder verifies the chain runs snip → prune →
// summary in order, stopping when under budget.
func TestThreeZoneSession_ResizeChainOrder(t *testing.T) {
	// Use a counter to track which strategies are called.
	var callOrder []string
	var mu sync.Mutex

	snip := func(full, window []ChatMessage) ([]ChatMessage, error) {
		mu.Lock()
		callOrder = append(callOrder, "snip")
		mu.Unlock()
		// Snip nothing — pretend it's not enough.
		return window, nil
	}
	prune := func(full, window []ChatMessage) ([]ChatMessage, error) {
		mu.Lock()
		callOrder = append(callOrder, "prune")
		mu.Unlock()
		// Prune nothing — pretend it's not enough.
		return window, nil
	}
	summary := func(full, window []ChatMessage) ([]ChatMessage, error) {
		mu.Lock()
		callOrder = append(callOrder, "summary")
		mu.Unlock()
		// Summary replaces everything with one short message — gets under budget.
		return []ChatMessage{mkMsg("system", "summary")}, nil
	}

	s := NewThreeZoneSession("s1", 5) // tiny budget
	_ = s.SetImmutablePrefix("sys", nil)
	s.SetResizeStrategies(snip, prune, summary)

	// Add a message that exceeds budget, triggering resize chain.
	s.AddToHistory(mkMsg("user", "this is a long message that exceeds budget"))

	mu.Lock()
	defer mu.Unlock()
	want := []string{"snip", "prune", "summary"}
	if len(callOrder) != len(want) {
		t.Fatalf("callOrder = %v, want %v", callOrder, want)
	}
	for i, w := range want {
		if callOrder[i] != w {
			t.Errorf("callOrder[%d] = %q, want %q", i, callOrder[i], w)
		}
	}
}

// TestThreeZoneSession_ResizeChainStopsAtSnip verifies the chain stops as
// soon as a strategy gets the window under budget.
func TestThreeZoneSession_ResizeChainStopsAtSnip(t *testing.T) {
	snipCalled := false
	pruneCalled := false
	summaryCalled := false

	snip := func(full, window []ChatMessage) ([]ChatMessage, error) {
		snipCalled = true
		// Replace with a short message — gets under budget.
		return []ChatMessage{mkMsg("system", "ok")}, nil
	}
	prune := func(full, window []ChatMessage) ([]ChatMessage, error) {
		pruneCalled = true
		return window, nil
	}
	summary := func(full, window []ChatMessage) ([]ChatMessage, error) {
		summaryCalled = true
		return []ChatMessage{mkMsg("system", "sum")}, nil
	}

	s := NewThreeZoneSession("s1", 10)
	_ = s.SetImmutablePrefix("sys", nil)
	s.SetResizeStrategies(snip, prune, summary)
	s.AddToHistory(mkMsg("user", "this is way too long to fit"))

	if !snipCalled {
		t.Error("snip should have been called")
	}
	if pruneCalled {
		t.Error("prune should NOT have been called (snip got under budget)")
	}
	if summaryCalled {
		t.Error("summary should NOT have been called (snip got under budget)")
	}
}

// TestThreeZoneSession_ConcurrentAddToHistory verifies AddToHistory is safe
// under concurrent access (100 goroutines).
func TestThreeZoneSession_ConcurrentAddToHistory(t *testing.T) {
	s := NewThreeZoneSession("s1", 1<<20) // large budget so no resize
	_ = s.SetImmutablePrefix("sys", nil)

	const N = 100
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(idx int) {
			defer wg.Done()
			s.AddToHistory(mkMsg("user", "msg"))
		}(i)
	}
	wg.Wait()

	prompt := s.BuildPrompt()
	// Expect: 1 system + N user messages
	if len(prompt) != N+1 {
		t.Errorf("expected %d messages, got %d", N+1, len(prompt))
	}
}

// TestThreeZoneSession_ConcurrentReadAndWrite verifies BuildPrompt (read) is
// safe under concurrent writes.
func TestThreeZoneSession_ConcurrentReadAndWrite(t *testing.T) {
	s := NewThreeZoneSession("s1", 1<<20)
	_ = s.SetImmutablePrefix("sys", nil)

	const N = 50
	var wg sync.WaitGroup
	wg.Add(N * 2)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			s.AddToHistory(mkMsg("user", "x"))
		}()
		go func() {
			defer wg.Done()
			_ = s.BuildPrompt()
		}()
	}
	wg.Wait()
}

// TestThreeZoneSession_StableMarshalInImmutableHash verifies that the
// immutableToolsJSON serialization is byte-stable — same tools in different
// map insertion orders should produce the same hash.
func TestThreeZoneSession_StableMarshalInImmutableHash(t *testing.T) {
	// Build the same tools with different map insertion orders.
	tools1 := []any{
		map[string]any{
			"name":        "calc",
			"description": "calculator",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"expr":      map[string]any{"type": "string"},
					"precision": map[string]any{"type": "integer"},
				},
			},
		},
	}
	tools2 := []any{
		map[string]any{
			"parameters": map[string]any{
				"properties": map[string]any{
					"precision": map[string]any{"type": "integer"},
					"expr":      map[string]any{"type": "string"},
				},
				"type": "object",
			},
			"description": "calculator",
			"name":        "calc",
		},
	}

	s1 := NewThreeZoneSession("a", 4096)
	_ = s1.SetImmutablePrefix("sys", tools1)
	s2 := NewThreeZoneSession("b", 4096)
	_ = s2.SetImmutablePrefix("sys", tools2)

	if s1.ImmutableHash() != s2.ImmutableHash() {
		t.Errorf("sessions with semantically-equal tools should have same hash\ns1: %s\ns2: %s",
			s1.ImmutableHash(), s2.ImmutableHash())
	}
}

// TestThreeZoneSession_NoResizeStrategies verifies that exceeding budget
// without any resize strategies configured is a no-op (history keeps growing).
func TestThreeZoneSession_NoResizeStrategies(t *testing.T) {
	s := NewThreeZoneSession("s1", 1) // tiny budget
	_ = s.SetImmutablePrefix("sys", nil)
	// No resize strategies configured
	s.AddToHistory(mkMsg("user", "this exceeds the tiny budget"))
	prompt := s.BuildPrompt()
	if len(prompt) != 2 {
		t.Errorf("expected 2 messages (no resize), got %d", len(prompt))
	}
}

// TestThreeZoneSession_ID verifies the ID is stored correctly.
func TestThreeZoneSession_ID(t *testing.T) {
	s := NewThreeZoneSession("my-session-id", 4096)
	if s.id != "my-session-id" {
		t.Errorf("id = %q, want %q", s.id, "my-session-id")
	}
}

// TestThreeZoneSession_StableMarshalProducesSortedJSON is a sanity check that
// the local stableMarshal produces sorted-key JSON.
func TestThreeZoneSession_StableMarshalProducesSortedJSON(t *testing.T) {
	v := []any{
		map[string]any{"z": 1, "a": 2},
	}
	b, err := stableMarshal(v)
	if err != nil {
		t.Fatalf("stableMarshal error: %v", err)
	}
	s := string(b)
	// "a" should appear before "z"
	if !strings.Contains(s, `"a":2,"z":1`) {
		t.Errorf("expected sorted keys, got %s", s)
	}
}
