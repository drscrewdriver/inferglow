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
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"
)

// M-HIGH-11: StreamingJSONParser must parse incrementally — only processing
// newly-added bytes — instead of re-parsing the entire buffer on every Feed.
// The previous implementation was O(n²) because each Feed re-parsed from byte
// 0. For 1000 chunks, this test would take seconds; with incremental parsing
// it should complete in milliseconds.
func TestStreamingJSONParser_LinearPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping performance test in short mode")
	}

	// Build N chunks that together form a JSON object with N-1 keys.
	// Each chunk adds one key-value pair (plus the closing brace on the last).
	const N = 1000

	chunks := make([]string, N)
	chunks[0] = `{"k0":0`
	for i := 1; i < N-1; i++ {
		chunks[i] = fmt.Sprintf(`,"k%d":%d`, i, i)
	}
	chunks[N-1] = fmt.Sprintf(`,"k%d":%d}`, N-1, N-1)

	start := time.Now()
	parser := NewStreamingJSONParser()
	go func() {
		for _, c := range chunks {
			if err := parser.Feed(c); err != nil {
				t.Errorf("Feed failed: %v", err)
				return
			}
		}
		if err := parser.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	count := 0
	for range parser.Events() {
		count++
	}
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Errorf("parsing %d chunks took %v, expected < 2s (O(n²) bug)", N, elapsed)
	}

	// Verify the result has all keys.
	result, ok := parser.Result().(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T (took %v)", parser.Result(), elapsed)
	}
	if len(result) != N {
		t.Errorf("expected %d keys, got %d (took %v)", N, len(result), elapsed)
	}
}

// M-HIGH-11: incremental parsing must produce the same result as a single
// full Feed. This guards against state corruption when drainTokens is called
// across multiple Feed calls.
func TestStreamingJSONParser_IncrementalEqualsFull(t *testing.T) {
	// A reasonably complex JSON payload.
	full := `{"name":"Alice","age":30,"tags":["a","b","c"],"address":{"city":"Beijing","zip":"100000"},"active":true,"score":42.5}`

	// Parse all at once.
	parserFull := NewStreamingJSONParser()
	go func() {
		parserFull.Feed(full)
		parserFull.Close()
	}()
	var eventsFull []JSONParseEvent
	var mu sync.Mutex
	done := make(chan struct{})
	go func() {
		defer close(done)
		for evt := range parserFull.Events() {
			mu.Lock()
			eventsFull = append(eventsFull, evt)
			mu.Unlock()
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("full parse timed out")
	}

	// Parse byte-by-byte (1 char at a time).
	parserInc := NewStreamingJSONParser()
	go func() {
		for i := 0; i < len(full); i++ {
			if err := parserInc.Feed(string(full[i])); err != nil {
				t.Errorf("Feed at offset %d failed: %v", i, err)
				return
			}
		}
		parserInc.Close()
	}()
	var eventsInc []JSONParseEvent
	done2 := make(chan struct{})
	go func() {
		defer close(done2)
		for evt := range parserInc.Events() {
			eventsInc = append(eventsInc, evt)
		}
	}()
	select {
	case <-done2:
	case <-time.After(10 * time.Second):
		t.Fatal("incremental parse timed out")
	}

	if len(eventsFull) != len(eventsInc) {
		t.Fatalf("event count mismatch: full=%d, incremental=%d", len(eventsFull), len(eventsInc))
	}
	for i := range eventsFull {
		if eventsFull[i].Type != eventsInc[i].Type {
			t.Errorf("event[%d].Type: full=%q, inc=%q", i, eventsFull[i].Type, eventsInc[i].Type)
		}
		if eventsFull[i].Key != eventsInc[i].Key {
			t.Errorf("event[%d].Key: full=%q, inc=%q", i, eventsFull[i].Key, eventsInc[i].Key)
		}
		if eventsFull[i].Path != eventsInc[i].Path {
			t.Errorf("event[%d].Path: full=%q, inc=%q", i, eventsFull[i].Path, eventsInc[i].Path)
		}
		if !reflect.DeepEqual(eventsFull[i].Value, eventsInc[i].Value) {
			t.Errorf("event[%d].Value: full=%v, inc=%v", i, eventsFull[i].Value, eventsInc[i].Value)
		}
	}

	// Final results must be deeply equal.
	if !reflect.DeepEqual(parserFull.Result(), parserInc.Result()) {
		t.Errorf("Result mismatch:\nfull=%v\ninc=%v", parserFull.Result(), parserInc.Result())
	}
}
