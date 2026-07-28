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
	"sync"
	"testing"
	"time"
)

func TestPublishAssignsMonotonicOffsets(t *testing.T) {
	b := NewInMemoryMessageBus()
	ctx := context.Background()
	var want int64 = 1
	for i := 1; i <= 10; i++ {
		if err := b.Publish(ctx, "t", Message{ID: "m"}); err != nil {
			t.Fatalf("Publish: %v", err)
		}
		last, err := b.ReplayGlobal(ctx, want)
		if err != nil {
			t.Fatalf("ReplayGlobal: %v", err)
		}
		m := <-last
		if m.Offset != want {
			t.Fatalf("offset = %d, want %d", m.Offset, want)
		}
		want++
	}
}

func TestConcurrentPublishOffsetsUnique(t *testing.T) {
	b := NewInMemoryMessageBus()
	ctx := context.Background()
	const n = 200
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = b.Publish(ctx, "t", Message{ID: "m"})
		}(i)
	}
	wg.Wait()

	log, err := b.ReplayGlobal(ctx, 0)
	if err != nil {
		t.Fatalf("ReplayGlobal: %v", err)
	}
	seen := make(map[int64]bool, n)
	got := 0
	for m := range log {
		if seen[m.Offset] {
			t.Fatalf("duplicate offset %d", m.Offset)
		}
		if m.Offset < 1 || m.Offset > n {
			t.Fatalf("offset out of range: %d", m.Offset)
		}
		seen[m.Offset] = true
		got++
	}
	if got != n {
		t.Fatalf("got %d messages, want %d", got, n)
	}
}

func TestSubscribeFanOutAndUnsubscribe(t *testing.T) {
	b := NewInMemoryMessageBus()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch1, err := b.Subscribe(ctx, "topicA")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	ch2, err := b.Subscribe(ctx, "topicA")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	if err := b.Publish(ctx, "topicA", Message{ID: "foo"}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	if m := <-ch1; m.ID != "foo" {
		t.Fatalf("ch1 got %q", m.ID)
	}
	if m := <-ch2; m.ID != "foo" {
		t.Fatalf("ch2 got %q", m.ID)
	}

	// A different topic must not be delivered.
	if err := b.Publish(ctx, "topicB", Message{ID: "bar"}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	assertNoMessage(t, ch1, ch2)
}

func TestDrainSessionIsolation(t *testing.T) {
	b := NewInMemoryMessageBus()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := b.Publish(ctx, "t", Message{SessionID: "s1", ID: "a"}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if err := b.Publish(ctx, "t", Message{SessionID: "s2", ID: "b"}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if err := b.Publish(ctx, "t", Message{SessionID: "s1", ID: "c"}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	drain, err := b.DrainSession(ctx, "s1")
	if err != nil {
		t.Fatalf("DrainSession: %v", err)
	}
	assertSession(t, drain, "s1", 2)

	global, err := b.DrainGlobal(ctx)
	if err != nil {
		t.Fatalf("DrainGlobal: %v", err)
	}
	assertSession(t, global, "", 3)
}

func TestReplayFromOffsetPreciseTruncation(t *testing.T) {
	b := NewInMemoryMessageBus()
	ctx := context.Background()
	for i := 1; i <= 5; i++ {
		if err := b.Publish(ctx, "t", Message{SessionID: "s1", ID: "m"}); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}

	// fromOffset=0 returns everything.
	log, err := b.ReplayGlobal(ctx, 0)
	if err != nil {
		t.Fatalf("ReplayGlobal: %v", err)
	}
	got := count(log)
	if got != 5 {
		t.Fatalf("replay(0) got %d, want 5", got)
	}

	// fromOffset=3 returns exactly offsets 3,4,5.
	log, err = b.ReplayGlobal(ctx, 3)
	if err != nil {
		t.Fatalf("ReplayGlobal: %v", err)
	}
	seen := make([]int64, 0, 3)
	for m := range log {
		seen = append(seen, m.Offset)
	}
	if len(seen) != 3 || seen[0] != 3 || seen[2] != 5 {
		t.Fatalf("replay(3) got %v, want [3 4 5]", seen)
	}

	// A fromOffset beyond the last offset yields an empty channel.
	log, err = b.ReplayGlobal(ctx, 99)
	if err != nil {
		t.Fatalf("ReplayGlobal: %v", err)
	}
	if seen := count(log); seen != 0 {
		t.Fatalf("replay(99) got %d, want 0", seen)
	}
}

func TestReplaySessionFilter(t *testing.T) {
	b := NewInMemoryMessageBus()
	ctx := context.Background()
	if err := b.Publish(ctx, "t", Message{SessionID: "s1", ID: "a"}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if err := b.Publish(ctx, "t", Message{SessionID: "s2", ID: "b"}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if err := b.Publish(ctx, "t", Message{SessionID: "s1", ID: "c"}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	log, err := b.ReplaySession(ctx, "s1", 0)
	if err != nil {
		t.Fatalf("ReplaySession: %v", err)
	}
	got := count(log)
	if got != 2 {
		t.Fatalf("got %d, want 2 (only s1 messages)", got)
	}
}

func TestConcurrentLockSingleWinner(t *testing.T) {
	b := NewInMemoryMessageBus()
	ctx := context.Background()
	ttl := 200 * time.Millisecond

	var mu sync.Mutex
	winners := 0
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := b.AcquireLock(ctx, "key", ttl)
			if err != nil {
				t.Errorf("AcquireLock: %v", err)
				return
			}
			if ok {
				mu.Lock()
				winners++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if winners != 1 {
		t.Fatalf("winners = %d, want exactly 1", winners)
	}

	// Held lock cannot be re-acquired until it expires.
	if ok, _ := b.AcquireLock(ctx, "key", ttl); ok {
		t.Fatal("re-acquired a live lock; want false")
	}

	time.Sleep(ttl + 20*time.Millisecond)
	ok, err := b.AcquireLock(ctx, "key", ttl)
	if err != nil {
		t.Fatalf("AcquireLock after expiry: %v", err)
	}
	if !ok {
		t.Fatal("could not re-acquire expired lock")
	}
}

func TestLockInvalidTTL(t *testing.T) {
	b := NewInMemoryMessageBus()
	ctx := context.Background()
	if _, err := b.AcquireLock(ctx, "key", 0); err != ErrInvalidTTL {
		t.Fatalf("AcquireLock(0) err = %v, want ErrInvalidTTL", err)
	}
	if _, err := b.AcquireLock(ctx, "key", -time.Second); err != ErrInvalidTTL {
		t.Fatalf("AcquireLock(-1s) err = %v, want ErrInvalidTTL", err)
	}
}

func TestReleaseLockLifecycle(t *testing.T) {
	b := NewInMemoryMessageBus()
	ctx := context.Background()

	if err := b.ReleaseLock(ctx, "missing"); err != ErrLockNotHeld {
		t.Fatalf("release missing lock err = %v, want ErrLockNotHeld", err)
	}

	if ok, err := b.AcquireLock(ctx, "key", time.Second); err != nil || !ok {
		t.Fatalf("AcquireLock: ok=%v err=%v", ok, err)
	}
	if err := b.ReleaseLock(ctx, "key"); err != nil {
		t.Fatalf("ReleaseLock: %v", err)
	}
	// Released lock cannot be released again.
	if err := b.ReleaseLock(ctx, "key"); err != ErrLockNotHeld {
		t.Fatalf("double release err = %v, want ErrLockNotHeld", err)
	}

	// Expired lock reports ErrLockNotHeld on release.
	if ok, _ := b.AcquireLock(ctx, "key", 50*time.Millisecond); !ok {
		t.Fatal("AcquireLock failed")
	}
	time.Sleep(80 * time.Millisecond)
	if err := b.ReleaseLock(ctx, "key"); err != ErrLockNotHeld {
		t.Fatalf("release expired lock err = %v, want ErrLockNotHeld", err)
	}
}

func TestSlowConsumerDoesNotBlockPublish(t *testing.T) {
	b := NewInMemoryMessageBus() // default buffer 64
	ctx := context.Background()
	sub, err := b.Subscribe(ctx, "t")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	_ = sub // never read -> buffer fills up

	done := make(chan struct{})
	go func() {
		defer close(done)
		// Publish far more messages than the subscriber buffer (64). If publish
		// blocked on the slow subscriber this never returns promptly.
		for i := 0; i < 1000; i++ {
			if err := b.Publish(ctx, "t", Message{ID: "m"}); err != nil {
				t.Errorf("Publish: %v", err)
				return
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Publish blocked on slow subscriber")
	}
}

func TestLogCompactionAndErrLogCompacted(t *testing.T) {
	b := NewInMemoryMessageBus(WithMaxLog(4))
	ctx := context.Background()
	for i := 1; i <= 10; i++ {
		if err := b.Publish(ctx, "t", Message{ID: "m"}); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}

	// Oldest retained offset is 7 (10-4+1).
	if _, err := b.ReplayGlobal(ctx, 1); err != ErrLogCompacted {
		t.Fatalf("replay(1) err = %v, want ErrLogCompacted", err)
	}
	log, err := b.ReplayGlobal(ctx, 7)
	if err != nil {
		t.Fatalf("ReplayGlobal: %v", err)
	}
	if got := count(log); got != 4 {
		t.Fatalf("replay(7) got %d, want 4", got)
	}
}

// --- helpers ---

func assertNoMessage(t *testing.T, chs ...<-chan Message) {
	t.Helper()
	for _, ch := range chs {
		select {
		case m, ok := <-ch:
			if ok {
				t.Fatalf("unexpected message %q received", m.ID)
			}
		default:
		}
	}
}

func assertSession(t *testing.T, ch <-chan Message, wantSession string, want int) {
	t.Helper()
	got := 0
	for m := range ch {
		if wantSession != "" && m.SessionID != wantSession {
			t.Fatalf("got session %q, want %q", m.SessionID, wantSession)
		}
		got++
		if got >= want {
			break
		}
	}
	if got != want {
		t.Fatalf("got %d messages, want %d", got, want)
	}
}

func count(ch <-chan Message) int {
	n := 0
	for range ch {
		n++
	}
	return n
}