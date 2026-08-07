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

package server

import (
	"testing"
	"time"
)

// TestMessageStore_AppendAndCount verifies append-only semantics and ID assignment.
func TestMessageStore_AppendAndCount(t *testing.T) {
	ms := NewMessageStore()
	for i := 0; i < 5; i++ {
		rec, err := ms.Append("s1", MessageRecord{Role: MessageRoleUser, Content: "hi"})
		if err != nil {
			t.Fatal(err)
		}
		if rec.ID == "" || rec.SessionID != "s1" {
			t.Fatalf("rec = %+v, want id+session filled", rec)
		}
	}
	if got := ms.Count("s1"); got != 5 {
		t.Fatalf("Count = %d, want 5", got)
	}
	if got := ms.Count("other"); got != 0 {
		t.Fatalf("Count(other) = %d, want 0", got)
	}
	if _, err := ms.Append("", MessageRecord{}); err == nil {
		t.Fatal("expected error appending with empty session id")
	}
}

// TestMessageStore_ListBefore_NewestFirst verifies the default page returns
// the newest messages in descending order.
func TestMessageStore_ListBefore_NewestFirst(t *testing.T) {
	ms := NewMessageStore()
	base := time.Now().Add(-time.Hour)
	for i := 0; i < 5; i++ {
		_, _ = ms.Append("s1", MessageRecord{
			Role:      MessageRoleUser,
			Content:   "m",
			CreatedAt: base.Add(time.Duration(i) * time.Minute),
		})
	}
	msgs, hasMore := ms.ListBefore("s1", time.Time{}, 3)
	if len(msgs) != 3 {
		t.Fatalf("len = %d, want 3", len(msgs))
	}
	if !hasMore {
		t.Fatal("want has_more=true with 5 total and limit 3")
	}
	// Newest first: m4, m3, m2.
	for i, want := range []int{4, 3, 2} {
		if msgs[i].CreatedAt.Minute() != base.Add(time.Duration(want)*time.Minute).Minute() {
			t.Fatalf("msgs[%d] = %v, want minute %d", i, msgs[i].CreatedAt, want)
		}
	}
}

// TestMessageStore_ListBefore_Cursor verifies pagination with a before cursor:
// the second page returns older messages and has_more flips correctly.
func TestMessageStore_ListBefore_Cursor(t *testing.T) {
	ms := NewMessageStore()
	base := time.Now().Add(-time.Hour)
	for i := 0; i < 5; i++ {
		_, _ = ms.Append("s1", MessageRecord{
			Role:      MessageRoleUser,
			Content:   "m",
			CreatedAt: base.Add(time.Duration(i) * time.Minute),
		})
	}
	// Page 1: newest 3 (m4..m2), cursor points at m2.
	msgs, hasMore := ms.ListBefore("s1", time.Time{}, 3)
	if !hasMore {
		t.Fatal("page1: want has_more")
	}
	cursor := msgs[len(msgs)-1].CreatedAt

	// Page 2: older than m2 → m1, m0; no more after.
	msgs2, hasMore2 := ms.ListBefore("s1", cursor, 3)
	if len(msgs2) != 2 {
		t.Fatalf("page2 len = %d, want 2", len(msgs2))
	}
	if hasMore2 {
		t.Fatal("page2: want has_more=false")
	}
}

// TestMessageStore_ListBefore_EmptySession verifies an empty session yields
// an empty result and has_more=false.
func TestMessageStore_ListBefore_EmptySession(t *testing.T) {
	ms := NewMessageStore()
	msgs, hasMore := ms.ListBefore("ghost", time.Time{}, 10)
	if len(msgs) != 0 || hasMore {
		t.Fatalf("msgs=%v hasMore=%v, want empty/false", msgs, hasMore)
	}
}

// TestMessageStore_ConcurrentAppend verifies Append is safe under concurrency.
func TestMessageStore_ConcurrentAppend(t *testing.T) {
	ms := NewMessageStore()
	const workers = 8
	const perWorker = 25
	done := make(chan struct{}, workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for i := 0; i < perWorker; i++ {
				_, _ = ms.Append("s1", MessageRecord{Role: MessageRoleUser, Content: "x"})
			}
		}()
	}
	for w := 0; w < workers; w++ {
		<-done
	}
	if got := ms.Count("s1"); got != workers*perWorker {
		t.Fatalf("Count = %d, want %d", got, workers*perWorker)
	}
}
