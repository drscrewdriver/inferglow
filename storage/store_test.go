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

package storage

import (
	"sync"
	"testing"
)

func TestMapGetMissing(t *testing.T) {
	m := NewMap[string, int]()
	if _, ok := m.Get("nope"); ok {
		t.Fatal("expected missing key to report not-ok")
	}
}

func TestMapSetGetOverwrite(t *testing.T) {
	m := NewMap[string, int]()
	m.Set("a", 1)
	if v, ok := m.Get("a"); !ok || v != 1 {
		t.Fatalf("got (%d, %v), want (1, true)", v, ok)
	}
	m.Set("a", 2) // overwrite
	if v, _ := m.Get("a"); v != 2 {
		t.Fatalf("after overwrite got %d, want 2", v)
	}
}

func TestMapDelete(t *testing.T) {
	m := NewMap[string, int]()
	m.Set("a", 1)
	m.Delete("a")
	if _, ok := m.Get("a"); ok {
		t.Fatal("expected deleted key to be gone")
	}
	if n := m.Len(); n != 0 {
		t.Fatalf("Len after delete = %d, want 0", n)
	}
	// Deleting missing key is a no-op.
	m.Delete("missing")
	if n := m.Len(); n != 0 {
		t.Fatalf("Len after delete-missing = %d, want 0", n)
	}
}

func TestMapLen(t *testing.T) {
	m := NewMap[string, int]()
	for i := 0; i < 5; i++ {
		m.Set(string(rune('a'+i)), i)
	}
	if n := m.Len(); n != 5 {
		t.Fatalf("Len = %d, want 5", n)
	}
}

func TestMapKeysValues(t *testing.T) {
	m := NewMap[string, string]()
	m.Set("a", "x")
	m.Set("b", "y")
	m.Set("c", "z")
	if n := len(m.Keys()); n != 3 {
		t.Fatalf("len(Keys) = %d, want 3", n)
	}
	if n := len(m.Values()); n != 3 {
		t.Fatalf("len(Values) = %d, want 3", n)
	}
	seen := map[string]bool{}
	for _, k := range m.Keys() {
		seen[k] = true
	}
	for _, want := range []string{"a", "b", "c"} {
		if !seen[want] {
			t.Fatalf("Keys missing %q", want)
		}
	}
	seenV := map[string]bool{}
	for _, v := range m.Values() {
		seenV[v] = true
	}
	for _, want := range []string{"x", "y", "z"} {
		if !seenV[want] {
			t.Fatalf("Values missing %q", want)
		}
	}
}

func TestMapConcurrentReadWrite(t *testing.T) {
	m := NewMap[string, int]()
	const goroutines = 32
	const perG = 200
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				m.Set(string(rune(id)), i)
				m.Get(string(rune(id)))
				_ = m.Len()
				if i%50 == 0 {
					m.Keys()
					m.Values()
				}
			}
		}(g)
	}
	wg.Wait()
	if n := m.Len(); n != goroutines {
		t.Fatalf("Len = %d, want %d", n, goroutines)
	}
}