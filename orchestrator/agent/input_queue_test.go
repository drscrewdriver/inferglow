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

package agent

import (
	"context"
	"sync"
	"testing"
)

// TestInputQueue_EnqueueDequeue verifies basic FIFO ordering.
func TestInputQueue_EnqueueDequeue(t *testing.T) {
	q := NewInputQueue(4)

	ch1 := make(chan InputResponse, 1)
	ch2 := make(chan InputResponse, 1)

	if err := q.Enqueue(InputRequest{Message: "first", ResponseCh: ch1, Ctx: context.Background()}); err != nil {
		t.Fatalf("Enqueue first: %v", err)
	}
	if err := q.Enqueue(InputRequest{Message: "second", ResponseCh: ch2, Ctx: context.Background()}); err != nil {
		t.Fatalf("Enqueue second: %v", err)
	}

	if q.Len() != 2 {
		t.Errorf("Len = %d, want 2", q.Len())
	}

	req, ok := q.Dequeue()
	if !ok || req.Message != "first" {
		t.Errorf("Dequeue = %q, %v; want 'first', true", req.Message, ok)
	}
	req, ok = q.Dequeue()
	if !ok || req.Message != "second" {
		t.Errorf("Dequeue = %q, %v; want 'second', true", req.Message, ok)
	}
	_, ok = q.Dequeue()
	if ok {
		t.Errorf("Dequeue from empty queue returned true")
	}
}

// TestInputQueue_Capacity verifies that Enqueue returns ErrQueueFull when
// the queue is at capacity.
func TestInputQueue_Capacity(t *testing.T) {
	q := NewInputQueue(2)

	for i := 0; i < 2; i++ {
		if err := q.Enqueue(InputRequest{Message: "msg", Ctx: context.Background()}); err != nil {
			t.Fatalf("Enqueue %d: %v", i, err)
		}
	}

	err := q.Enqueue(InputRequest{Message: "overflow", Ctx: context.Background()})
	if err != ErrQueueFull {
		t.Errorf("Enqueue overflow = %v, want ErrQueueFull", err)
	}
}

// TestInputQueue_ContextCancelled verifies that Enqueue returns an error
// when the context is already cancelled, and Dequeue skips cancelled requests.
func TestInputQueue_ContextCancelled(t *testing.T) {
	q := NewInputQueue(4)

	ctx, cancel := context.WithCancel(context.Background())

	// Enqueue with a cancelled context should fail.
	cancel()
	err := q.Enqueue(InputRequest{Message: "cancelled", Ctx: ctx})
	if err == nil {
		t.Errorf("Enqueue with cancelled context should return error")
	}
	if q.Len() != 0 {
		t.Errorf("Len = %d after cancelled Enqueue, want 0", q.Len())
	}

	// Enqueue a valid request, then a cancelled one, then dequeue.
	// The cancelled one should be skipped.
	ctx2 := context.Background()
	ctx3, cancel3 := context.WithCancel(context.Background())
	cancel3()

	_ = q.Enqueue(InputRequest{Message: "valid", Ctx: ctx2})
	_ = q.Enqueue(InputRequest{Message: "cancelled-in-queue", Ctx: ctx3})

	req, ok := q.Dequeue()
	if !ok || req.Message != "valid" {
		t.Errorf("Dequeue = %q, %v; want 'valid', true", req.Message, ok)
	}
	// The cancelled request should be skipped.
	_, ok = q.Dequeue()
	if ok {
		t.Errorf("Dequeue should skip cancelled request")
	}
}

// TestInputQueue_Concurrent verifies thread safety under concurrent access.
func TestInputQueue_Concurrent(t *testing.T) {
	q := NewInputQueue(100)
	var wg sync.WaitGroup

	// Enqueue from multiple goroutines.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_ = q.Enqueue(InputRequest{Message: "msg", Ctx: context.Background()})
		}(i)
	}
	wg.Wait()

	if q.Len() != 50 {
		t.Errorf("Len = %d after concurrent Enqueue, want 50", q.Len())
	}

	// Dequeue from multiple goroutines.
	var dequeued sync.Map
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if req, ok := q.Dequeue(); ok {
				dequeued.Store(req.Message, true)
			}
		}()
	}
	wg.Wait()

	if q.Len() != 0 {
		t.Errorf("Len = %d after concurrent Dequeue, want 0", q.Len())
	}
}

// TestInputQueue_DefaultCapacity verifies that zero capacity uses the default.
func TestInputQueue_DefaultCapacity(t *testing.T) {
	q := NewInputQueue(0)
	for i := 0; i < 8; i++ {
		if err := q.Enqueue(InputRequest{Message: "msg", Ctx: context.Background()}); err != nil {
			t.Fatalf("Enqueue %d: %v", i, err)
		}
	}
	err := q.Enqueue(InputRequest{Message: "overflow", Ctx: context.Background()})
	if err != ErrQueueFull {
		t.Errorf("Enqueue overflow = %v, want ErrQueueFull (default capacity 8)", err)
	}
}

// TestInputQueue_Peek verifies Peek without removing.
func TestInputQueue_Peek(t *testing.T) {
	q := NewInputQueue(4)

	_, ok := q.Peek()
	if ok {
		t.Errorf("Peek on empty queue returned true")
	}

	_ = q.Enqueue(InputRequest{Message: "peek-me", Ctx: context.Background()})
	req, ok := q.Peek()
	if !ok || req.Message != "peek-me" {
		t.Errorf("Peek = %q, %v; want 'peek-me', true", req.Message, ok)
	}
	// Peek should not remove the item.
	if q.Len() != 1 {
		t.Errorf("Len = %d after Peek, want 1", q.Len())
	}
}
