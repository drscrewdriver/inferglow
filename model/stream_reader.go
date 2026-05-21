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
	"io"
	"sync"
)

// ErrNoValue is returned by a convert function passed to
// [StreamReaderWithConvert] to indicate that the current element should be
// skipped (not emitted) and the next element read.
//
// Use it to filter out empty or irrelevant chunks:
//
//	out = StreamReaderWithConvert(src, func(v int) (string, error) {
//	    if v == 0 {
//	        return "", ErrNoValue // skip zeros
//	    }
//	    return strconv.Itoa(v), nil
//	})
var ErrNoValue = errors.New("inferglow: no value to return")

// ErrStreamClosed is returned by [StreamWriter.Send] when the stream has been
// closed — either the writer was closed via [StreamWriter.Close] or the reader
// closed first via [StreamReader.Close].
var ErrStreamClosed = errors.New("inferglow: stream closed")

// StreamReader is the consumer side of a streamed sequence of values of type T.
// It is internally backed by a channel and supports fan-out (Copy), merging,
// concatenation, transformation, and piping.
//
// A StreamReader must be used through a pointer. The typical consumption loop
// is:
//
//	defer r.Close()
//	for {
//	    v, err := r.Recv()
//	    if errors.Is(err, io.EOF) {
//	        break
//	    }
//	    if err != nil {
//	        return err
//	    }
//	    process(v)
//	}
//
// A reader must not be copied by value.
type StreamReader[T any] struct {
	// ch is the receive channel backing the stream. It is closed when the
	// stream ends naturally; Recv returns io.EOF (or err if set) afterwards.
	ch <-chan T
	// err holds a terminal error. When ch is closed and err is non-nil, Recv
	// returns err instead of io.EOF.
	err error
	// done is closed by Close to cancel any pending Recv. Once closed, Recv
	// returns io.EOF immediately.
	done chan struct{}
	// once guards Close so it is safe to call multiple times.
	once sync.Once
	// closeHook is invoked on Close to release associated resources (e.g.
	// closing source readers in a merge/convert pipeline). It is optional;
	// nil for basic readers.
	closeHook func()
}

// Recv reads the next element from the stream. It returns io.EOF when the
// stream has ended naturally (the underlying channel was closed) or when the
// reader has been closed via Close.
//
// If the stream terminated with a non-EOF error (e.g. from a convert function
// or a concatenated source), that error is returned instead of io.EOF.
func (r *StreamReader[T]) Recv() (T, error) {
	var zero T
	// Prioritise the done signal so that Close is observed even when the
	// channel still has buffered data.
	select {
	case <-r.done:
		return zero, io.EOF
	default:
	}
	select {
	case <-r.done:
		return zero, io.EOF
	case v, ok := <-r.ch:
		if !ok {
			if r.err != nil {
				return zero, r.err
			}
			return zero, io.EOF
		}
		return v, nil
	}
}

// Close closes the reader and releases any associated resources. It is safe to
// call Close multiple times. After Close, Recv returns io.EOF.
//
// For readers created by [MergeStreamReaders], [ConcatStreamReader], or
// [StreamReaderWithConvert], Close propagates to all underlying source readers.
func (r *StreamReader[T]) Close() error {
	r.once.Do(func() {
		if r.done != nil {
			close(r.done)
		}
		if r.closeHook != nil {
			r.closeHook()
		}
	})
	return nil
}

// Copy creates a new reader that independently receives every element of the
// original stream. After Copy, both the original reader and the returned copy
// can read all elements independently.
//
// Copy must not be called concurrently with Recv on the same reader. Both
// readers should be consumed concurrently (e.g. in separate goroutines)
// because the internal fan-out goroutine delivers each element to both
// readers before reading the next; consuming one reader without draining the
// other will eventually block the fan-out goroutine.
func (r *StreamReader[T]) Copy() *StreamReader[T] {
	// Capture the original channel and replace it with a fan-out channel.
	// The fan-out goroutine drains oldCh and broadcasts every element to both
	// the original reader (via a) and the copy (via b).
	oldCh := r.ch
	a := make(chan T)
	b := make(chan T)
	r.ch = a

	cp := &StreamReader[T]{
		ch:   b,
		done: make(chan struct{}),
	}

	go func() {
		defer close(a)
		defer close(b)
		for {
			select {
			case <-r.done:
				return
			case <-cp.done:
				return
			case v, ok := <-oldCh:
				if !ok {
					return
				}
				// Deliver to the original reader.
				select {
				case a <- v:
				case <-r.done:
					return
				case <-cp.done:
					return
				}
				// Deliver to the copy.
				select {
				case b <- v:
				case <-r.done:
					return
				case <-cp.done:
					return
				}
			}
		}
	}()

	return cp
}

// StreamReaderFromChannel creates a reader that drains the provided channel.
// When the channel is closed, Recv returns io.EOF. The reader does not close
// the supplied channel; ownership remains with the caller.
func StreamReaderFromChannel[T any](ch <-chan T) *StreamReader[T] {
	return &StreamReader[T]{
		ch:   ch,
		done: make(chan struct{}),
	}
}

// StreamReaderFromArray creates a reader backed by the provided slice. After
// all items are consumed, Recv returns io.EOF.
func StreamReaderFromArray[T any](items []T) *StreamReader[T] {
	ch := make(chan T)
	r := &StreamReader[T]{
		ch:   ch,
		done: make(chan struct{}),
	}
	go func() {
		defer close(ch)
		for _, item := range items {
			select {
			case ch <- item:
			case <-r.done:
				return
			}
		}
	}()
	return r
}

// MergeStreamReaders merges multiple readers into a single reader. Elements
// from all sources are interleaved in arrival order (non-deterministic). The
// merged reader returns io.EOF after every source has been exhausted.
//
// With zero readers it returns nil; with one reader it returns that reader
// directly. Closing the merged reader closes all underlying sources.
func MergeStreamReaders[T any](readers ...*StreamReader[T]) *StreamReader[T] {
	if len(readers) == 0 {
		return nil
	}
	if len(readers) == 1 {
		return readers[0]
	}

	ch := make(chan T)
	r := &StreamReader[T]{
		ch:   ch,
		done: make(chan struct{}),
	}

	var wg sync.WaitGroup
	for _, src := range readers {
		wg.Add(1)
		go func(s *StreamReader[T]) {
			defer wg.Done()
			for {
				v, err := s.Recv()
				if errors.Is(err, io.EOF) {
					return
				}
				if err != nil {
					// Stop draining this source on a non-EOF error; other
					// sources continue.
					return
				}
				select {
				case ch <- v:
				case <-r.done:
					return
				}
			}
		}(src)
	}
	go func() {
		wg.Wait()
		close(ch)
	}()

	r.closeHook = func() {
		for _, src := range readers {
			src.Close()
		}
	}
	return r
}

// ConcatStreamReader concatenates multiple readers into a single reader.
// Sources are consumed sequentially: the second reader is not opened until the
// first returns io.EOF, and so on. The concatenated reader returns io.EOF
// after the last source is exhausted.
//
// With zero readers it returns nil; with one reader it returns that reader
// directly. Closing the concatenated reader closes all underlying sources.
func ConcatStreamReader[T any](readers ...*StreamReader[T]) *StreamReader[T] {
	if len(readers) == 0 {
		return nil
	}
	if len(readers) == 1 {
		return readers[0]
	}

	ch := make(chan T)
	r := &StreamReader[T]{
		ch:   ch,
		done: make(chan struct{}),
	}

	go func() {
		defer close(ch)
		for _, src := range readers {
			for {
				v, err := src.Recv()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					// Propagate the terminal error and stop.
					r.err = err
					return
				}
				select {
				case ch <- v:
				case <-r.done:
					return
				}
			}
		}
	}()

	r.closeHook = func() {
		for _, src := range readers {
			src.Close()
		}
	}
	return r
}

// Pipe creates a writer/reader pair backed by a buffered channel of the given
// capacity.
//
// The writer's Send blocks when the buffer is full (backpressure). Closing the
// writer causes the reader to return io.EOF once buffered items are drained.
// Closing the reader causes subsequent Send calls to return [ErrStreamClosed].
func Pipe[T any](cap int) (*StreamWriter[T], *StreamReader[T]) {
	ch := make(chan T, cap)
	done := make(chan struct{})
	writer := &StreamWriter[T]{
		ch:   ch,
		done: done,
	}
	reader := &StreamReader[T]{
		ch:   ch,
		done: done,
	}
	return writer, reader
}

// StreamWriter is the producer side of a [Pipe].
type StreamWriter[T any] struct {
	ch     chan T
	done   chan struct{}
	once   sync.Once
	mu     sync.Mutex
	closed bool
}

// Send writes an item to the stream. It blocks when the buffer is full
// (backpressure). It returns [ErrStreamClosed] if the stream has been closed
// (either the writer was closed or the reader closed first), and nil on
// success.
func (w *StreamWriter[T]) Send(item T) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return ErrStreamClosed
	}
	// Prioritise the reader-close signal so that Close on the reader is
	// observed even when the buffer still has capacity.
	select {
	case <-w.done:
		return ErrStreamClosed
	default:
	}
	select {
	case w.ch <- item:
		return nil
	case <-w.done:
		return ErrStreamClosed
	}
}

// Close closes the writer. After Close, the reader receives io.EOF once
// buffered items are drained. It is safe to call Close multiple times.
func (w *StreamWriter[T]) Close() error {
	w.once.Do(func() {
		w.mu.Lock()
		w.closed = true
		close(w.ch)
		w.mu.Unlock()
	})
	return nil
}

// StreamReaderWithConvert returns a reader that transforms every element of r
// using fn. If fn returns [ErrNoValue], the element is skipped and the next
// element is read. A non-nil error (other than ErrNoValue) from fn terminates
// the converted stream with that error.
//
// The original reader r is closed when the converted reader is closed.
func StreamReaderWithConvert[T, U any](r *StreamReader[T], fn func(T) (U, error)) *StreamReader[U] {
	ch := make(chan U)
	reader := &StreamReader[U]{
		ch:   ch,
		done: make(chan struct{}),
	}

	go func() {
		defer close(ch)
		for {
			v, err := r.Recv()
			if errors.Is(err, io.EOF) {
				return
			}
			if err != nil {
				reader.err = err
				return
			}
			uv, convErr := fn(v)
			if errors.Is(convErr, ErrNoValue) {
				continue
			}
			if convErr != nil {
				reader.err = convErr
				return
			}
			select {
			case ch <- uv:
			case <-reader.done:
				return
			}
		}
	}()

	reader.closeHook = func() { r.Close() }
	return reader
}
