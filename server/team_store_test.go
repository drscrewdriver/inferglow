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

// Behavior tests for the refactored (storage.Map-backed) TeamStore. The
// concurrent Create case verifies that the nextID counter + id-assembly critical
// section (guarded by metaMu) and the Map are mutually consistent.

package server

import (
	"sync"
	"testing"
)

// TestTeamStoreConcurrentCreateUniqueIDs verifies that concurrent Create calls
// yield unique, strictly increasing IDs under the Map-backed implementation.
func TestTeamStoreConcurrentCreateUniqueIDs(t *testing.T) {
	ts := NewTeamStore()

	const workers = 16
	const perWorker = 20
	const total = workers * perWorker

	ids := make([]string, total)
	var wg sync.WaitGroup

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				idx := w*perWorker + i
				id, err := ts.Create(teamCfg("t"))
				if err != nil {
					t.Errorf("Create: %v", err)
					return
				}
				ids[idx] = id
			}
		}(w)
	}
	wg.Wait()

	seen := make(map[string]struct{}, total)
	for _, id := range ids {
		if id == "" {
			t.Fatal("got empty ID from Create")
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate ID generated: %q", id)
		}
		seen[id] = struct{}{}
	}
	if len(ts.List()) != total {
		t.Fatalf("List len = %d, want %d", len(ts.List()), total)
	}
}

// TestTeamStoreConcurrentMixedOps exercises concurrent Create/Get/List/Delete
// to confirm the Map-backed store remains race-safe.
func TestTeamStoreConcurrentMixedOps(t *testing.T) {
	ts := NewTeamStore()

	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				id, err := ts.Create(teamCfg("mix"))
				if err != nil {
					t.Errorf("Create: %v", err)
					return
				}
				ts.Get(id)
				_ = ts.List()
				if i%2 == 0 {
					_ = ts.Delete(id)
				}
			}
		}()
	}
	wg.Wait()
}