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
	"errors"
	"sync"
)

// ErrQueueFull is returned by InputQueue.Enqueue when the queue has reached
// its capacity. The server translates this to HTTP 429.
var ErrQueueFull = errors.New("agent: input queue is full")

// InputRequest represents a user input submitted while the agent is busy.
type InputRequest struct {
	// Message is the user input text.
	Message string
	// Mode is the preempt mode requested by the caller.
	Mode PreemptMode
	// ResponseCh is the channel on which the result is sent back. The
	// caller blocks on this channel until the agent processes the request.
	ResponseCh chan<- InputResponse
	// Ctx is the caller's context. If cancelled before the request is
	// dequeued, the consumer skips it.
	Ctx context.Context
}

// InputResponse is the result of processing an InputRequest.
type InputResponse struct {
	// Response is the agent's final response text.
	Response string
	// Error is non-nil when the agent failed to process the input.
	Error error
}

// InputQueue is a bounded priority queue for user inputs submitted while
// the agent is busy. It is safe for concurrent use.
//
// Requests are bucketed by PreemptMode into three priority buckets
// (later/queue is lowest, now/force is highest); Dequeue always returns
// the highest-priority pending request first, preserving FIFO order
// within each bucket.
type InputQueue struct {
	mu sync.Mutex
	// buckets[0]=queue (later), buckets[1]=safe_point (next), buckets[2]=force (now).
	buckets  [3][]InputRequest
	capacity int
	// notify is a cap=1 channel used to wake consumers when a new request
	// is enqueued. Non-blocking send on Enqueue; consumers select on WaitCh().
	notify chan struct{}
}

// NewInputQueue creates an InputQueue with the given capacity. A capacity
// of zero or less uses the default of 8.
func NewInputQueue(capacity int) *InputQueue {
	if capacity <= 0 {
		capacity = 8
	}
	return &InputQueue{
		capacity: capacity,
		notify:   make(chan struct{}, 1),
	}
}

// bucketFor maps a PreemptMode to its priority bucket index: queue
// (later) is lowest, force (now) is highest. Unknown modes fall back to
// the queue bucket so callers never lose inputs.
func bucketFor(mode PreemptMode) int {
	switch mode {
	case PreemptForce:
		return 2
	case PreemptSafePoint:
		return 1
	default:
		return 0
	}
}

// lenLocked returns the total number of pending requests across all
// buckets. Callers must hold q.mu.
func (q *InputQueue) lenLocked() int {
	n := 0
	for b := 0; b < 3; b++ {
		n += len(q.buckets[b])
	}
	return n
}

// Enqueue adds a request to the queue. Returns ErrQueueFull when the queue
// is at capacity. The caller's context is checked before enqueueing; if
// already cancelled, ctx.Err() is returned without enqueueing.
func (q *InputQueue) Enqueue(req InputRequest) error {
	// Check context before enqueueing.
	if req.Ctx != nil {
		if err := req.Ctx.Err(); err != nil {
			return err
		}
	}

	q.mu.Lock()
	defer q.mu.Unlock()
	if q.lenLocked() >= q.capacity {
		return ErrQueueFull
	}
	b := bucketFor(req.Mode)
	q.buckets[b] = append(q.buckets[b], req)
	// Wake consumers waiting on WaitCh(). Non-blocking: if a notification
	// is already pending, skip (consumer will drain all pending on wake).
	select {
	case q.notify <- struct{}{}:
	default:
	}
	return nil
}

// Dequeue removes and returns the highest-priority pending request (force
// first, then safe-point, then queue; FIFO within each bucket). The second
// return value is false when the queue is empty. Requests whose context
// has been cancelled are skipped silently.
func (q *InputQueue) Dequeue() (InputRequest, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for b := 2; b >= 0; b-- {
		for len(q.buckets[b]) > 0 {
			req := q.buckets[b][0]
			q.buckets[b] = q.buckets[b][1:]
			// Skip requests whose context has been cancelled.
			if req.Ctx != nil && req.Ctx.Err() != nil {
				continue
			}
			return req, true
		}
	}
	return InputRequest{}, false
}

// Peek returns the highest-priority pending request without removing it.
// Returns false when the queue is empty. Requests whose context has been
// cancelled are ignored.
func (q *InputQueue) Peek() (InputRequest, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for b := 2; b >= 0; b-- {
		for _, req := range q.buckets[b] {
			if req.Ctx != nil && req.Ctx.Err() != nil {
				continue
			}
			return req, true
		}
	}
	return InputRequest{}, false
}

// Snapshot returns a priority-ordered copy of the pending requests
// (force first, then safe-point, then queue; FIFO within each bucket),
// for UI display. Requests whose context has been cancelled are omitted.
// The returned requests share ResponseCh references with the originals;
// callers must not send on those channels.
func (q *InputQueue) Snapshot() []InputRequest {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]InputRequest, 0, q.lenLocked())
	for b := 2; b >= 0; b-- {
		for _, req := range q.buckets[b] {
			if req.Ctx != nil && req.Ctx.Err() != nil {
				continue
			}
			out = append(out, req)
		}
	}
	return out
}

// Len returns the number of pending requests.
func (q *InputQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.lenLocked()
}

// WaitCh returns a channel that receives a signal when a new request is
// enqueued. Consumers should select on this channel to avoid polling.
// The channel is never closed; it is safe to select on indefinitely.
func (q *InputQueue) WaitCh() <-chan struct{} {
	return q.notify
}
