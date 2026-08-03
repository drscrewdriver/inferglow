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

package imbridge

import (
	"context"
	"testing"
	"time"
)

// mockAdapter is a test helper that implements PlatformAdapter
// for use in Bridge tests.
type mockAdapter struct {
	name     string
	started  bool
	stopped  bool
	incoming chan IncomingMessage
}

func (m *mockAdapter) Platform() string { return m.name }

func (m *mockAdapter) Start(ctx context.Context) error {
	m.started = true
	<-ctx.Done()
	return ctx.Err()
}

func (m *mockAdapter) Stop() error {
	m.stopped = true
	return nil
}

func (m *mockAdapter) Send(ctx context.Context, chatID, text string) error {
	return nil
}

func (m *mockAdapter) Incoming() <-chan IncomingMessage { return m.incoming }

func TestBridge_New(t *testing.T) {
	handler := func(ctx context.Context, msg IncomingMessage) (string, error) {
		return "reply", nil
	}
	b := NewBridge(handler)
	if b == nil {
		t.Fatal("NewBridge returned nil")
	}
}

func TestBridge_AddAdapter(t *testing.T) {
	handler := func(ctx context.Context, msg IncomingMessage) (string, error) {
		return "reply", nil
	}
	b := NewBridge(handler)
	m := &mockAdapter{
		name:     "mock",
		incoming: make(chan IncomingMessage, 10),
	}
	b.AddAdapter(m)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = b.Run(ctx)
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	if !m.started {
		t.Error("adapter was not started, so it was not registered")
	}
}

func TestBridge_Run(t *testing.T) {
	handler := func(ctx context.Context, msg IncomingMessage) (string, error) {
		return "reply", nil
	}
	b := NewBridge(handler)
	m := &mockAdapter{
		name:     "mock",
		incoming: make(chan IncomingMessage, 10),
	}
	b.AddAdapter(m)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = b.Run(ctx)
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	if !m.started {
		t.Error("adapter was not started")
	}
	if !m.stopped {
		t.Error("adapter was not stopped")
	}
}

func TestBridge_ChatHandler(t *testing.T) {
	received := make(chan IncomingMessage, 1)
	handler := func(ctx context.Context, msg IncomingMessage) (string, error) {
		received <- msg
		return "hello back", nil
	}
	b := NewBridge(handler)
	m := &mockAdapter{
		name:     "mock",
		incoming: make(chan IncomingMessage, 10),
	}
	b.AddAdapter(m)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = b.Run(ctx)
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)

	msg := IncomingMessage{
		ChatID: "chat1",
		UserID: "user1",
		Text:   "hello",
		MsgID:  "msg1",
	}
	m.incoming <- msg

	select {
	case got := <-received:
		if got.Text != "hello" {
			t.Errorf("handler got text %q, want %q", got.Text, "hello")
		}
		if got.ChatID != "chat1" {
			t.Errorf("handler got ChatID %q, want %q", got.ChatID, "chat1")
		}
		if got.UserID != "user1" {
			t.Errorf("handler got UserID %q, want %q", got.UserID, "user1")
		}
		if got.MsgID != "msg1" {
			t.Errorf("handler got MsgID %q, want %q", got.MsgID, "msg1")
		}
	case <-time.After(time.Second):
		t.Fatal("handler was not called within 1s")
	}

	cancel()
	<-done
}

func TestBridge_MultipleAdapters(t *testing.T) {
	handler := func(ctx context.Context, msg IncomingMessage) (string, error) {
		return "reply", nil
	}
	b := NewBridge(handler)

	m1 := &mockAdapter{
		name:     "mock1",
		incoming: make(chan IncomingMessage, 10),
	}
	m2 := &mockAdapter{
		name:     "mock2",
		incoming: make(chan IncomingMessage, 10),
	}
	m3 := &mockAdapter{
		name:     "mock3",
		incoming: make(chan IncomingMessage, 10),
	}
	b.AddAdapter(m1)
	b.AddAdapter(m2)
	b.AddAdapter(m3)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = b.Run(ctx)
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	if !m1.started {
		t.Error("adapter m1 was not started")
	}
	if !m2.started {
		t.Error("adapter m2 was not started")
	}
	if !m3.started {
		t.Error("adapter m3 was not started")
	}
	if !m1.stopped {
		t.Error("adapter m1 was not stopped")
	}
	if !m2.stopped {
		t.Error("adapter m2 was not stopped")
	}
	if !m3.stopped {
		t.Error("adapter m3 was not stopped")
	}
}
