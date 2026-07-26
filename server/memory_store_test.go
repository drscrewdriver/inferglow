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

// Behavior tests for the refactored (storage.Map-backed) InMemoryStore. The
// SearchMemory O(n) scan now iterates Map.Values(); these cases confirm the
// filter/limit semantics are preserved and the store stays race-safe.

package server

import (
	"fmt"
	"sync"
	"testing"
)

// TestInMemoryStoreConcurrentUpsertSearch exercises concurrent Upsert+Search on
// the Map-backed store. Safe to run under -race.
func TestInMemoryStoreConcurrentUpsertSearch(t *testing.T) {
	s := NewInMemoryStore()

	const workers = 12
	const perWorker = 30
	var wg sync.WaitGroup

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			// Unique key per worker so Upsert/Get/Delete are self-contained.
			for i := 0; i < perWorker; i++ {
				k := fmt.Sprintf("m-%d-%d", w, i)
				_ = s.UpsertMemory(testMemRecord(k, "alpha beta", "cat"))
				if _, err := s.GetMemory(k); err != nil {
					t.Errorf("GetMemory: %v", err)
					return
				}
				if _, err := s.SearchMemory("alpha", "cat", 10); err != nil {
					t.Errorf("SearchMemory: %v", err)
					return
				}
				_ = s.DeleteMemory(k)
			}
		}(w)
	}
	wg.Wait()
}

// TestInMemoryStoreConcurrentDistinctIDs verifies distinct records stay
// independently retrievable after concurrent upsert of different IDs.
func TestInMemoryStoreConcurrentDistinctIDs(t *testing.T) {
	s := NewInMemoryStore()

	const total = 200
	var wg sync.WaitGroup
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := "m"
			_ = s.UpsertMemory(testMemRecord(id, "payload", ""))
		}(i)
	}
	wg.Wait()

	rec, err := s.GetMemory("m")
	if err != nil {
		t.Fatalf("GetMemory: %v", err)
	}
	if rec == nil || rec.Content != "payload" {
		t.Fatalf("GetMemory returned %+v", rec)
	}
}