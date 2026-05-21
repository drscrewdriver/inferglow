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

package model

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"
)

// TestStreamReaderFromChannel verifies that a reader created from a channel
// delivers all buffered elements and returns io.EOF once the channel is closed.
func TestStreamReaderFromChannel(t *testing.T) {
	ch := make(chan int, 3)
	ch <- 1
	ch <- 2
	ch <- 3
	close(ch)

	r := StreamReaderFromChannel[int](ch)
	defer r.Close()

	var got []int
	for {
		v, err := r.Recv()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				t.Fatalf("expected io.EOF, got %v", err)
			}
			break
		}
		got = append(got, v)
	}
	want := []int{1, 2, 3}
	if len(got) != len(want) || got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Fatalf("got %v, want %v", got, want)
	}

	// Close is idempotent.
	r.Close()
}

// TestStreamReaderFromArray verifies that a reader created from a slice
// delivers all elements in order and returns io.EOF when exhausted.
func TestStreamReaderFromArray(t *testing.T) {
	r := StreamReaderFromArray[int]([]int{10, 20, 30})
	defer r.Close()

	var got []int
	for {
		v, err := r.Recv()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				t.Fatalf("expected io.EOF, got %v", err)
			}
			break
		}
		got = append(got, v)
	}
	want := []int{10, 20, 30}
	if len(got) != len(want) {
		t.Fatalf("got %d items, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}

	// An empty array yields io.EOF immediately.
	empty := StreamReaderFromArray[int](nil)
	defer empty.Close()
	if _, err := empty.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("expected io.EOF for empty array, got %v", err)
	}
}

// TestStreamReaderClose verifies that after Close, Recv returns io.EOF and no
// longer delivers buffered values.
func TestStreamReaderClose(t *testing.T) {
	// Channel reader with buffered data: Close discards buffered values.
	ch := make(chan int, 2)
	ch <- 1
	ch <- 2
	r := StreamReaderFromChannel[int](ch)

	r.Close()
	if _, err := r.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("expected io.EOF after Close, got %v", err)
	}

	// Array reader: Close stops further receives.
	arr := StreamReaderFromArray[int]([]int{1, 2, 3})
	if v, _ := arr.Recv(); v != 1 {
		t.Fatalf("expected first value 1, got %d", v)
	}
	arr.Close()
	if _, err := arr.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("expected io.EOF after array Close, got %v", err)
	}

	// Close is idempotent.
	r.Close()
	arr.Close()
}

// TestStreamReaderCopy verifies that Copy creates an independent reader that
// receives every element of the original stream. Both readers must be consumed
// concurrently to avoid deadlocking the internal fan-out goroutine.
func TestStreamReaderCopy(t *testing.T) {
	src := StreamReaderFromArray[int]([]int{1, 2, 3, 4, 5})
	cp := src.Copy()

	var wg sync.WaitGroup
	results := make([][]int, 2)
	readers := []*StreamReader[int]{src, cp}

	for i := range readers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for {
				v, err := readers[i].Recv()
				if errors.Is(err, io.EOF) {
					return
				}
				if err != nil {
					t.Errorf("reader %d error: %v", i, err)
					return
				}
				results[i] = append(results[i], v)
			}
		}(i)
	}
	wg.Wait()

	want := []int{1, 2, 3, 4, 5}
	for i, got := range results {
		if len(got) != len(want) {
			t.Fatalf("reader %d got %d items, want %d", i, len(got), len(want))
		}
		for j := range want {
			if got[j] != want[j] {
				t.Fatalf("reader %d got %v, want %v", i, got, want)
			}
		}
	}

	src.Close()
	cp.Close()
}

// TestMergeStreamReaders verifies that MergeStreamReaders interleaves elements
// from multiple source readers into a single stream.
func TestMergeStreamReaders(t *testing.T) {
	r1 := StreamReaderFromArray[int]([]int{1, 2})
	r2 := StreamReaderFromArray[int]([]int{3, 4})
	r3 := StreamReaderFromArray[int]([]int{5, 6})

	merged := MergeStreamReaders[int](r1, r2, r3)
	if merged == nil {
		t.Fatal("expected non-nil merged reader")
	}

	var got []int
	for {
		v, err := merged.Recv()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				t.Fatalf("expected io.EOF, got %v", err)
			}
			break
		}
		got = append(got, v)
	}
	sort.Ints(got)
	want := []int{1, 2, 3, 4, 5, 6}
	if len(got) != len(want) {
		t.Fatalf("got %d items, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	merged.Close()

	// Edge cases: 0 readers -> nil, 1 reader -> returned directly.
	if MergeStreamReaders[int]() != nil {
		t.Fatal("expected nil for zero readers")
	}
	solo := StreamReaderFromArray[int]([]int{42})
	if MergeStreamReaders[int](solo) != solo {
		t.Fatal("expected single reader to be returned directly")
	}
	solo.Close()
}

// TestConcatStreamReader verifies that ConcatStreamReader sequentially
// concatenates multiple source readers, preserving element order.
func TestConcatStreamReader(t *testing.T) {
	r1 := StreamReaderFromArray[int]([]int{1, 2})
	r2 := StreamReaderFromArray[int]([]int{3, 4})
	r3 := StreamReaderFromArray[int]([]int{5, 6})

	concat := ConcatStreamReader[int](r1, r2, r3)
	if concat == nil {
		t.Fatal("expected non-nil concatenated reader")
	}

	var got []int
	for {
		v, err := concat.Recv()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				t.Fatalf("expected io.EOF, got %v", err)
			}
			break
		}
		got = append(got, v)
	}
	// Concat preserves order: all of r1, then all of r2, then all of r3.
	want := []int{1, 2, 3, 4, 5, 6}
	if len(got) != len(want) {
		t.Fatalf("got %d items, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	concat.Close()

	// Edge cases: 0 readers -> nil, 1 reader -> returned directly.
	if ConcatStreamReader[int]() != nil {
		t.Fatal("expected nil for zero readers")
	}
	solo := StreamReaderFromArray[int]([]int{42})
	if ConcatStreamReader[int](solo) != solo {
		t.Fatal("expected single reader to be returned directly")
	}
	solo.Close()
}

// TestStreamReaderPipe verifies the Pipe writer/reader pair: basic send/recv,
// backpressure when the buffer is full, writer close causing reader EOF, and
// reader close causing Send to return ErrStreamClosed.
func TestStreamReaderPipe(t *testing.T) {
	t.Run("send_recv", func(t *testing.T) {
		w, r := Pipe[int](2)
		if err := w.Send(1); err != nil {
			t.Fatalf("Send(1) failed: %v", err)
		}
		if err := w.Send(2); err != nil {
			t.Fatalf("Send(2) failed: %v", err)
		}
		w.Close()

		v, err := r.Recv()
		if err != nil || v != 1 {
			t.Fatalf("expected 1, got %d err=%v", v, err)
		}
		v, err = r.Recv()
		if err != nil || v != 2 {
			t.Fatalf("expected 2, got %d err=%v", v, err)
		}
		if _, err := r.Recv(); !errors.Is(err, io.EOF) {
			t.Fatalf("expected io.EOF after writer Close, got %v", err)
		}
		r.Close()
	})

	t.Run("backpressure", func(t *testing.T) {
		w, r := Pipe[int](1)
		if err := w.Send(1); err != nil {
			t.Fatalf("Send(1) failed: %v", err)
		}

		sent := make(chan struct{})
		go func() {
			w.Send(2) // should block until the reader drains.
			close(sent)
		}()

		// Send(2) should block because the buffer is full.
		select {
		case <-sent:
			t.Fatal("Send did not block when the buffer was full")
		case <-time.After(50 * time.Millisecond):
		}

		// Drain one item to unblock the blocked Send.
		if v, err := r.Recv(); err != nil || v != 1 {
			t.Fatalf("expected 1, got %d err=%v", v, err)
		}

		select {
		case <-sent:
			// expected: Send completed.
		case <-time.After(time.Second):
			t.Fatal("Send did not unblock after draining the buffer")
		}

		if v, err := r.Recv(); err != nil || v != 2 {
			t.Fatalf("expected 2, got %d err=%v", v, err)
		}
		w.Close()
		r.Close()
	})

	t.Run("writer_close_eof", func(t *testing.T) {
		w, r := Pipe[int](1)
		if err := w.Send(11); err != nil {
			t.Fatalf("Send(11) failed: %v", err)
		}
		w.Close()

		v, err := r.Recv()
		if err != nil || v != 11 {
			t.Fatalf("expected 11, got %d err=%v", v, err)
		}
		if _, err := r.Recv(); !errors.Is(err, io.EOF) {
			t.Fatalf("expected io.EOF after writer Close, got %v", err)
		}
		r.Close()
	})

	t.Run("reader_close_send_returns_error", func(t *testing.T) {
		w, r := Pipe[int](1)
		r.Close()

		// After the reader closes, Send must return ErrStreamClosed.
		if err := w.Send(99); !errors.Is(err, ErrStreamClosed) {
			t.Fatalf("expected ErrStreamClosed, got %v", err)
		}
		// A subsequent Send must still return ErrStreamClosed.
		if err := w.Send(100); !errors.Is(err, ErrStreamClosed) {
			t.Fatalf("expected ErrStreamClosed on second Send, got %v", err)
		}
		w.Close()
	})
}

// TestStreamReaderWithConvert verifies that StreamReaderWithConvert transforms
// elements, skips elements when fn returns ErrNoValue, and terminates the
// stream with an error when fn returns any other error.
func TestStreamReaderWithConvert(t *testing.T) {
	t.Run("convert_type", func(t *testing.T) {
		src := StreamReaderFromArray[int]([]int{1, 2, 3})
		conv := StreamReaderWithConvert[int, string](src, func(n int) (string, error) {
			return strconv.Itoa(n), nil
		})
		defer conv.Close()

		var got []string
		for {
			v, err := conv.Recv()
			if err != nil {
				if !errors.Is(err, io.EOF) {
					t.Fatalf("expected io.EOF, got %v", err)
				}
				break
			}
			got = append(got, v)
		}
		want := []string{"1", "2", "3"}
		if len(got) != len(want) {
			t.Fatalf("got %d items, want %d", len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("got %v, want %v", got, want)
			}
		}
	})

	t.Run("skip_err_no_value", func(t *testing.T) {
		src := StreamReaderFromArray[int]([]int{1, 2, 3, 4})
		conv := StreamReaderWithConvert[int, string](src, func(n int) (string, error) {
			if n%2 == 0 {
				return "", ErrNoValue // skip even numbers
			}
			return fmt.Sprintf("n=%d", n), nil
		})
		defer conv.Close()

		var got []string
		for {
			v, err := conv.Recv()
			if err != nil {
				if !errors.Is(err, io.EOF) {
					t.Fatalf("expected io.EOF, got %v", err)
				}
				break
			}
			got = append(got, v)
		}
		want := []string{"n=1", "n=3"}
		if len(got) != len(want) {
			t.Fatalf("got %d items, want %d", len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("got %v, want %v", got, want)
			}
		}
	})

	t.Run("error_terminates", func(t *testing.T) {
		src := StreamReaderFromArray[int]([]int{1, 2, 3})
		boom := errors.New("convert boom")
		conv := StreamReaderWithConvert[int, string](src, func(n int) (string, error) {
			if n == 2 {
				return "", boom
			}
			return strconv.Itoa(n), nil
		})
		defer conv.Close()

		v, err := conv.Recv()
		if err != nil || v != "1" {
			t.Fatalf("expected '1', got %q err=%v", v, err)
		}
		if _, err := conv.Recv(); !errors.Is(err, boom) {
			t.Fatalf("expected boom error, got %v", err)
		}
	})
}

// TestStreamReaderEOF verifies that all reader types return io.EOF after the
// stream is exhausted, and continue to return io.EOF on subsequent Recv calls.
func TestStreamReaderEOF(t *testing.T) {
	t.Run("array", func(t *testing.T) {
		r := StreamReaderFromArray[int]([]int{1, 2})
		defer r.Close()

		v, err := r.Recv()
		if err != nil || v != 1 {
			t.Fatalf("expected 1, got %d err=%v", v, err)
		}
		v, err = r.Recv()
		if err != nil || v != 2 {
			t.Fatalf("expected 2, got %d err=%v", v, err)
		}
		if _, err := r.Recv(); !errors.Is(err, io.EOF) {
			t.Fatalf("expected io.EOF, got %v", err)
		}
		if _, err := r.Recv(); !errors.Is(err, io.EOF) {
			t.Fatalf("expected io.EOF on second call, got %v", err)
		}
	})

	t.Run("channel", func(t *testing.T) {
		ch := make(chan int, 1)
		ch <- 1
		close(ch)
		r := StreamReaderFromChannel[int](ch)
		defer r.Close()

		v, err := r.Recv()
		if err != nil || v != 1 {
			t.Fatalf("expected 1, got %d err=%v", v, err)
		}
		if _, err := r.Recv(); !errors.Is(err, io.EOF) {
			t.Fatalf("expected io.EOF, got %v", err)
		}
		if _, err := r.Recv(); !errors.Is(err, io.EOF) {
			t.Fatalf("expected io.EOF on second call, got %v", err)
		}
	})

	t.Run("pipe", func(t *testing.T) {
		w, r := Pipe[int](2)
		w.Send(1)
		w.Close()
		defer r.Close()

		v, err := r.Recv()
		if err != nil || v != 1 {
			t.Fatalf("expected 1, got %d err=%v", v, err)
		}
		if _, err := r.Recv(); !errors.Is(err, io.EOF) {
			t.Fatalf("expected io.EOF, got %v", err)
		}
		if _, err := r.Recv(); !errors.Is(err, io.EOF) {
			t.Fatalf("expected io.EOF on second call, got %v", err)
		}
	})
}
