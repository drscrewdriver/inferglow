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

package taskcontext

import (
	"context"
	"testing"
)

// staticSource is a test ContextSource with fixed descriptors and content.
type staticSource struct {
	descriptors []Descriptor
	content     map[string]string
}

func (s *staticSource) EnumerateDescriptors(_ context.Context, _ string, limit int) (*DescriptorPage, error) {
	descs := s.descriptors
	if limit > 0 && len(descs) > limit {
		descs = descs[:limit]
	}
	return &DescriptorPage{Descriptors: descs}, nil
}

func (s *staticSource) ReadExact(_ context.Context, ref SourceRef, maxChars int) (*ContextSourceRead, error) {
	key := ref.Source + ":" + ref.Ref
	c, ok := s.content[key]
	if !ok {
		c = "(not found)"
	}
	truncated := false
	if maxChars > 0 && len(c) > maxChars {
		c = c[:maxChars]
		truncated = true
	}
	return &ContextSourceRead{Ref: ref, Content: c, Truncated: truncated}, nil
}

func TestTaskContextPutRemove(t *testing.T) {
	tc := NewTaskContext()
	tc.Put(ContextEntry{Key: "a", Content: "alpha", Priority: 1})
	tc.Put(ContextEntry{Key: "b", Content: "beta", Priority: 2})

	snap := tc.Snapshot()
	if len(snap.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(snap.Entries))
	}

	if !tc.Remove("a") {
		t.Fatal("expected Remove(a) = true")
	}
	if tc.Remove("nonexistent") {
		t.Fatal("expected Remove(nonexistent) = false")
	}
	snap = tc.Snapshot()
	if len(snap.Entries) != 1 {
		t.Fatalf("expected 1 entry after remove, got %d", len(snap.Entries))
	}
}

func TestTaskContextReaderEntries(t *testing.T) {
	tc := NewTaskContext()
	tc.Put(ContextEntry{Key: "low", Content: "low-priority", Priority: 1})
	tc.Put(ContextEntry{Key: "high", Content: "high-priority", Priority: 10})

	reader := tc.Reader(WithBudget(ContextBudget{MaxChars: 10000, MaxBlocks: 10, MaxBlockChars: 5000}))
	consumption, err := reader.Read(context.Background())
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(consumption.Blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(consumption.Blocks))
	}
	// High priority should come first.
	if consumption.Blocks[0].Ref != "high" {
		t.Errorf("expected first block = high, got %s", consumption.Blocks[0].Ref)
	}
}

func TestTaskContextReaderBudget(t *testing.T) {
	tc := NewTaskContext()
	tc.Put(ContextEntry{Key: "a", Content: "aaaa"})
	tc.Put(ContextEntry{Key: "b", Content: "bbbb"})
	tc.Put(ContextEntry{Key: "c", Content: "cccc"})

	// Budget allows only 2 blocks.
	reader := tc.Reader(WithBudget(ContextBudget{MaxChars: 10000, MaxBlocks: 2, MaxBlockChars: 5000}))
	consumption, err := reader.Read(context.Background())
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(consumption.Blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(consumption.Blocks))
	}
	if !consumption.Truncated {
		t.Error("expected Truncated=true")
	}
}

func TestTaskContextReaderMaxChars(t *testing.T) {
	tc := NewTaskContext()
	tc.Put(ContextEntry{Key: "big", Content: "this is a very long content that exceeds budget"})

	reader := tc.Reader(WithBudget(ContextBudget{MaxChars: 10, MaxBlocks: 10, MaxBlockChars: 5000}))
	consumption, err := reader.Read(context.Background())
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	// The entry content is 44 chars, MaxChars=10 → should not fit.
	if len(consumption.Blocks) != 0 {
		t.Fatalf("expected 0 blocks (content exceeds MaxChars), got %d", len(consumption.Blocks))
	}
}

func TestTaskContextWithSource(t *testing.T) {
	src := &staticSource{
		descriptors: []Descriptor{
			{Ref: SourceRef{Source: "file", Ref: "main.go"}, Description: "main file", SizeHint: 20},
		},
		content: map[string]string{
			"file:main.go": "package main\nfunc main() {}",
		},
	}

	tc := NewTaskContext()
	tc.Attach(src)
	tc.Put(ContextEntry{Key: "note", Content: "hello"})

	reader := tc.Reader(WithBudget(DefaultBudget()))
	consumption, err := reader.Read(context.Background())
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	// Should have entry + source content.
	if len(consumption.Blocks) < 2 {
		t.Fatalf("expected >=2 blocks, got %d", len(consumption.Blocks))
	}
}

func TestDefaultBudget(t *testing.T) {
	b := DefaultBudget()
	if b.MaxChars <= 0 || b.MaxBlocks <= 0 || b.MaxBlockChars <= 0 {
		t.Errorf("DefaultBudget has zero values: %+v", b)
	}
}

func TestTaskContextChaining(t *testing.T) {
	tc := NewTaskContext().
		Put(ContextEntry{Key: "a", Content: "alpha"}).
		Put(ContextEntry{Key: "b", Content: "beta"})

	snap := tc.Snapshot()
	if len(snap.Entries) != 2 {
		t.Fatalf("expected 2 entries from chained Put, got %d", len(snap.Entries))
	}
}
