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

// Characterization tests locking down InMemoryStore's observable behavior
// against the ORIGINAL map-based implementation. These must continue to pass
// unchanged after the storage abstraction refactor, proving old/new equivalence.

package server

import "testing"

func testMemRecord(id, content, category string, facts ...string) MemoryRecord {
	return MemoryRecord{ID: id, Content: content, Category: category, Facts: facts}
}

func TestInMemoryStoreCharUpsertGet(t *testing.T) {
	s := NewInMemoryStore()
	rec := testMemRecord("m1", "buy milk", "shopping")
	if err := s.UpsertMemory(rec); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	got, err := s.GetMemory("m1")
	if err != nil {
		t.Fatalf("GetMemory: %v", err)
	}
	if got == nil || got.ID != "m1" || got.Content != "buy milk" {
		t.Fatalf("GetMemory returned %+v", got)
	}
}

func TestInMemoryStoreCharUpsertOverwrite(t *testing.T) {
	s := NewInMemoryStore()
	if err := s.UpsertMemory(testMemRecord("m1", "old", "")); err != nil {
		t.Fatalf("Upsert #1: %v", err)
	}
	if err := s.UpsertMemory(testMemRecord("m1", "new", "")); err != nil {
		t.Fatalf("Upsert #2: %v", err)
	}
	got, err := s.GetMemory("m1")
	if err != nil {
		t.Fatalf("GetMemory: %v", err)
	}
	if got.Content != "new" {
		t.Fatalf("Content after overwrite = %q, want %q", got.Content, "new")
	}
}

func TestInMemoryStoreCharGetMissing(t *testing.T) {
	s := NewInMemoryStore()
	if _, err := s.GetMemory("nope"); err == nil {
		t.Fatal("expected error for missing memory")
	}
}

func TestInMemoryStoreCharSearch(t *testing.T) {
	s := NewInMemoryStore()
	_ = s.UpsertMemory(testMemRecord("m1", "alpha beta", "catA"))
	_ = s.UpsertMemory(testMemRecord("m2", "gamma delta", "catB", "factFoo"))
	_ = s.UpsertMemory(testMemRecord("m3", "EPsilon zeta", "catA"))

	// Empty query & empty category returns everything.
	all, err := s.SearchMemory("", "", 10)
	if err != nil {
		t.Fatalf("Search all: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("search all len = %d, want 3", len(all))
	}

	// Case-insensitive content match.
	byContent, err := s.SearchMemory("gamma", "", 10)
	if err != nil {
		t.Fatalf("Search content: %v", err)
	}
	if len(byContent) != 1 || byContent[0].ID != "m2" {
		t.Fatalf("content search got %+v", byContent)
	}

	// Case-insensitive match on uppercase content.
	byUpper, err := s.SearchMemory("zeta", "", 10)
	if err != nil {
		t.Fatalf("Search upper: %v", err)
	}
	if len(byUpper) != 1 || byUpper[0].ID != "m3" {
		t.Fatalf("upper search got %+v", byUpper)
	}

	// Facts match.
	byFact, err := s.SearchMemory("factfoo", "", 10)
	if err != nil {
		t.Fatalf("Search fact: %v", err)
	}
	if len(byFact) != 1 || byFact[0].ID != "m2" {
		t.Fatalf("fact search got %+v", byFact)
	}

	// Category filter.
	byCat, err := s.SearchMemory("", "catA", 10)
	if err != nil {
		t.Fatalf("Search category: %v", err)
	}
	if len(byCat) != 2 {
		t.Fatalf("category search len = %d, want 2", len(byCat))
	}

	// limit truncates.
	limited, err := s.SearchMemory("", "catA", 1)
	if err != nil {
		t.Fatalf("Search limited: %v", err)
	}
	if len(limited) != 1 {
		t.Fatalf("limited len = %d, want 1", len(limited))
	}
}

func TestInMemoryStoreCharDelete(t *testing.T) {
	s := NewInMemoryStore()
	_ = s.UpsertMemory(testMemRecord("m1", "x", ""))
	if err := s.DeleteMemory("m1"); err != nil {
		t.Fatalf("Delete existing: %v", err)
	}
	if _, err := s.GetMemory("m1"); err == nil {
		t.Fatal("expected deleted memory to be gone")
	}
	// Delete a missing memory must error.
	if err := s.DeleteMemory("m1"); err == nil {
		t.Fatal("expected error deleting already-deleted memory")
	}
}