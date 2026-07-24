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

package messagebus

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// Defaults for the in-memory implementation.
const (
	// DefaultMaxLog caps the size of the retained ordered log. Older entries
	// are compacted away once the log reaches this many messages.
	DefaultMaxLog = 100000
	// DefaultBuffer is the default per-subscriber / per-drain channel buffer.
	DefaultBuffer = 64
)

// Sentinel errors exposed by the in-memory bus.
var (
	// ErrLogCompacted is returned by Replay* when the requested offset lies
	// before the oldest retained entry (i.e. it was compacted away).
	ErrLogCompacted = errors.New("messagebus: log compacted")
	// ErrInvalidTTL is returned by AcquireLock when ttl is not positive.
	ErrInvalidTTL = errors.New("messagebus: invalid lock ttl")
	// ErrLockNotHeld is returned by ReleaseLock when the key is not currently
	// held (either never acquired, or already released / expired).
	ErrLockNotHeld = errors.New("messagebus: lock not held")
)

// Option configures an InMemoryMessageBus at construction.
type Option func(*InMemoryMessageBus)

// WithMaxLog overrides the retained log size (0 disables the cap).
func WithMaxLog(n int) Option {
	return func(b *InMemoryMessageBus) { b.maxLog = n }
}

// WithBuffer overrides the per-subscriber / per-drain channel buffer size.
func WithBuffer(n int) Option {
	return func(b *InMemoryMessageBus) { b.buf = n }
}

// subscriber is a broadcast receiver. Ch receives messages published while
// subscribed; filter decides which messages match this subscriber's topic.
type subscriber struct {
	ch     chan Message
	filter func(Message) bool
}

// lockEntry is a held advisory lock with an expiry deadline.
type lockEntry struct {
	deadline time.Time
}

// InMemoryMessageBus is a dependency-free, concurrency-safe MessageBus built
// on the Go standard library. It keeps a single ordered log, a set of
// broadcast subscribers, and a TTL lock table.
type InMemoryMessageBus struct {
	mu   sync.Mutex
	cond *sync.Cond

	log   []Message // append-only ordered log
	base  int64     // Offset of log[0] (0 while log is empty)
	subs  map[int]*subscriber
	locks map[string]*lockEntry

	nextSeq  atomic.Int64 // next global offset to assign
	nextSub  atomic.Int64 // subscriber id generator
	maxLog   int
	buf      int
}

// NewInMemoryMessageBus builds a bus with default settings, overridable via
// options.
func NewInMemoryMessageBus(opts ...Option) *InMemoryMessageBus {
	b := &InMemoryMessageBus{
		log:    make([]Message, 0, DefaultBuffer),
		subs:   make(map[int]*subscriber),
		locks:  make(map[string]*lockEntry),
		maxLog: DefaultMaxLog,
		buf:    DefaultBuffer,
	}
	b.cond = sync.NewCond(&b.mu)
	for _, o := range opts {
		o(b)
	}
	return b
}

// Publish assigns a global monotonic offset, appends the message to the log,
// wakes any waiting drains, and fans the message out to matching subscribers
// without blocking (slow subscribers are dropped; they can recover via Replay).
func (b *InMemoryMessageBus) Publish(ctx context.Context, topic string, msg Message) error {
	msg.Topic = topic
	msg.Offset = b.nextSeq.Add(1)
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}

	b.mu.Lock()
	if b.maxLog > 0 && len(b.log) == b.maxLog {
		copy(b.log, b.log[1:])
		b.log = b.log[:len(b.log)-1]
		b.base = b.log[0].Offset
	}
	b.log = append(b.log, msg)
	subs := make([]*subscriber, 0, len(b.subs))
	for _, s := range b.subs {
		subs = append(subs, s)
	}
	b.cond.Broadcast()
	b.mu.Unlock()

	for _, s := range subs {
		if !s.filter(msg) {
			continue
		}
		select {
		case s.ch <- msg:
		case <-ctx.Done():
			return ctx.Err()
		default: // slow consumer: drop, recoverable via Replay
		}
	}
	return nil
}

// Subscribe registers a broadcast receiver for topic. The returned channel is
// closed when ctx is cancelled or the bus is no longer delivering.
func (b *InMemoryMessageBus) Subscribe(ctx context.Context, topic string) (<-chan Message, error) {
	ch := make(chan Message, b.buf)
	sub := &subscriber{
		ch:     ch,
		filter: func(m Message) bool { return m.Topic == topic },
	}
	b.mu.Lock()
	id := int(b.nextSub.Add(1))
	b.subs[id] = sub
	b.mu.Unlock()

	go func() {
		<-ctx.Done()
		b.mu.Lock()
		delete(b.subs, id)
		b.mu.Unlock()
		close(ch)
	}()

	return ch, nil
}

// DrainGlobal replays the whole retained log then live-streams every new
// message until ctx is cancelled.
func (b *InMemoryMessageBus) DrainGlobal(ctx context.Context) (<-chan Message, error) {
	out := make(chan Message, b.buf)
	go b.drainLoop(ctx, out, func(Message) bool { return true })
	return out, nil
}

// DrainSession replays the log filtered by sessionID then live-streams new
// messages for that session until ctx is cancelled.
func (b *InMemoryMessageBus) DrainSession(ctx context.Context, sessionID string) (<-chan Message, error) {
	out := make(chan Message, b.buf)
	go b.drainLoop(ctx, out, func(m Message) bool { return m.SessionID == sessionID })
	return out, nil
}

// drainLoop is the shared engine behind Drain*. It tracks a local cursor over
// the ordered log and awaits new entries via the condition variable. Delivery
// happens outside the lock and is non-blocking (a slow consumer is dropped so
// producers are never stalled).
func (b *InMemoryMessageBus) drainLoop(ctx context.Context, out chan Message, filter func(Message) bool) {
	defer close(out)
	own := int64(0)

	for {
		b.mu.Lock()
		if b.base > own {
			own = b.base // history was compacted; resume from oldest retained
		}
		var toSend []Message
		for _, m := range b.log {
			if m.Offset < own {
				continue
			}
			if filter(m) {
				toSend = append(toSend, m)
			}
			own = m.Offset + 1
		}
		b.mu.Unlock()

		for _, m := range toSend {
			select {
			case out <- m:
			case <-ctx.Done():
				return
			default: // slow drain consumer: drop
			}
		}

		b.mu.Lock()
		for {
			if last := lastOffset(b.log); last >= own {
				break // new entries arrived
			}
			select {
			case <-ctx.Done():
				b.mu.Unlock()
				return
			default:
			}
			b.cond.Wait() // atomically releases mu while waiting
		}
		b.mu.Unlock()
	}
}

// ReplayGlobal returns all retained log entries with Offset >= fromOffset as a
// static snapshot. It returns ErrLogCompacted when fromOffset lies before the
// oldest retained entry.
func (b *InMemoryMessageBus) ReplayGlobal(ctx context.Context, fromOffset int64) (<-chan Message, error) {
	return b.replay(ctx, "", fromOffset)
}

// ReplaySession returns session-filtered log entries with Offset >= fromOffset
// as a static snapshot.
func (b *InMemoryMessageBus) ReplaySession(ctx context.Context, sessionID string, fromOffset int64) (<-chan Message, error) {
	return b.replay(ctx, sessionID, fromOffset)
}

func (b *InMemoryMessageBus) replay(ctx context.Context, sessionID string, fromOffset int64) (<-chan Message, error) {
	b.mu.Lock()
	if b.base > fromOffset {
		b.mu.Unlock()
		return nil, ErrLogCompacted
	}
	var out []Message
	for _, m := range b.log {
		if m.Offset < fromOffset {
			continue
		}
		if sessionID != "" && m.SessionID != sessionID {
			continue
		}
		out = append(out, m)
	}
	b.mu.Unlock()

	ch := make(chan Message, len(out))
	for _, m := range out {
		select {
		case ch <- m:
		case <-ctx.Done():
			close(ch)
			return ch, nil
		}
	}
	close(ch)
	return ch, nil
}

// AcquireLock takes key for ttl. It succeeds only when the key is free or its
// previous holder's lease has expired.
func (b *InMemoryMessageBus) AcquireLock(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	if ttl <= 0 {
		return false, ErrInvalidTTL
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if e, ok := b.locks[key]; ok && time.Now().Before(e.deadline) {
		return false, nil // still held
	}
	b.locks[key] = &lockEntry{deadline: time.Now().Add(ttl)}
	return true, nil
}

// ReleaseLock releases key, failing when it is not currently held (e.g. after
// natural expiry). This forbids releasing a lock that is already gone.
func (b *InMemoryMessageBus) ReleaseLock(ctx context.Context, key string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	e, ok := b.locks[key]
	if !ok || time.Now().After(e.deadline) {
		delete(b.locks, key)
		return ErrLockNotHeld
	}
	delete(b.locks, key)
	return nil
}

// lastOffset returns the offset of the newest log entry, or -1 when empty.
func lastOffset(log []Message) int64 {
	if len(log) == 0 {
		return -1
	}
	return log[len(log)-1].Offset
}

// Compile-time assertion: *InMemoryMessageBus implements MessageBus.
var _ MessageBus = (*InMemoryMessageBus)(nil)