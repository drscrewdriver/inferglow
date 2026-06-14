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
	"testing"
)

// Compile-time interface compliance checks.
var _ Memory = (*SummaryMemory)(nil)
var _ Memory = (*TokenBufferMemory)(nil)

// TestMemory_SummaryMemory_SatisfiesInterface verifies SummaryMemory
// implements Memory with basic Load/Save/Clear.
func TestMemory_SummaryMemory_SatisfiesInterface(t *testing.T) {
	var m Memory = NewSummaryMemory()
	m.Save([]ChatMessage{{Role: "user", Content: "hi"}})
	if got := m.Load(); len(got) != 1 {
		t.Fatalf("expected 1 message, got %d", len(got))
	}
	m.Clear()
	if got := m.Load(); len(got) != 0 {
		t.Fatalf("expected 0 messages after Clear, got %d", len(got))
	}
}

// TestMemory_TokenBufferMemory_SatisfiesInterface verifies TokenBufferMemory
// implements Memory with basic Load/Save/Clear.
func TestMemory_TokenBufferMemory_SatisfiesInterface(t *testing.T) {
	var m Memory = NewTokenBufferMemory()
	m.Save([]ChatMessage{{Role: "user", Content: "hi"}})
	if got := m.Load(); len(got) != 1 {
		t.Fatalf("expected 1 message, got %d", len(got))
	}
	m.Clear()
	if got := m.Load(); len(got) != 0 {
		t.Fatalf("expected 0 messages after Clear, got %d", len(got))
	}
}
