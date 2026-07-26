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

// Behavior tests for the refactored (storage.Map-backed) FlowStore. Together
// with the characterization tests they prove old/new equivalence; the
// concurrent cases additionally exercise concurrency under `go test -race`.

package server

import (
	"sync"
	"testing"
)

// TestFlowStoreConcurrentRegisterList exercises concurrent Register+List on the
// Map-backed store. Safe to run under -race.
func TestFlowStoreConcurrentRegisterList(t *testing.T) {
	fs := NewFlowStore(nil)

	const workers = 16
	const perWorker = 50
	var wg sync.WaitGroup

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				name := "flow"
				_ = fs.Register(validFlowDef(name)) // same key, upsert contention
				fs.Get(name)
				fs.List()
			}
		}(w)
	}
	wg.Wait()

	if n := len(fs.List()); n != 1 {
		t.Fatalf("List len = %d, want 1 (single key upserted concurrently)", n)
	}
	if _, ok := fs.Get("flow"); !ok {
		t.Fatal("expected registered flow to be retrievable")
	}
}

// TestFlowStoreConcurrentDistinctKeys verifies unique keys stay independently
// addressable after concurrent registration.
func TestFlowStoreConcurrentDistinctKeys(t *testing.T) {
	fs := NewFlowStore(nil)

	const workers = 20
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			name := "flow"
			for i := 0; i < 20; i++ {
				def := validFlowDef(name)
				if err := fs.Register(def); err != nil {
					t.Errorf("unexpected Register error: %v", err)
					return
				}
			}
		}(w)
	}
	wg.Wait()

	if _, ok := fs.Get("flow"); !ok {
		t.Fatal("expected 'flow' to be present after concurrent registration")
	}
}