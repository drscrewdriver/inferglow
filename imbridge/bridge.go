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
	"log"
	"sync"
	"time"
)

// ChatHandler processes an incoming message and returns a reply.
type ChatHandler func(ctx context.Context, msg IncomingMessage) (string, error)

// Bridge routes incoming messages from platform adapters to the agent
// and sends replies back. It handles deduplication and rate limiting.
type Bridge struct {
	adapters []PlatformAdapter
	handler  ChatHandler

	// Deduplication: track seen message IDs.
	mu       sync.Mutex
	seenMsgs map[string]time.Time

	// Rate limiting: per-chat token bucket (simplified).
	chatLast map[string]time.Time
	minGap   time.Duration
}

// NewBridge creates a bridge with the given chat handler.
func NewBridge(handler ChatHandler) *Bridge {
	return &Bridge{
		handler:  handler,
		seenMsgs: make(map[string]time.Time),
		chatLast: make(map[string]time.Time),
		minGap:   500 * time.Millisecond,
	}
}

// AddAdapter registers a platform adapter with the bridge.
func (b *Bridge) AddAdapter(a PlatformAdapter) {
	b.adapters = append(b.adapters, a)
}

// Run starts all adapters and processes incoming messages until ctx is cancelled.
func (b *Bridge) Run(ctx context.Context) error {
	var wg sync.WaitGroup

	for _, adapter := range b.adapters {
		wg.Add(1)
		go func(a PlatformAdapter) {
			defer wg.Done()
			if err := a.Start(ctx); err != nil && ctx.Err() == nil {
				log.Printf("imbridge: adapter %s error: %v", a.Platform(), err)
			}
		}(adapter)

		// Process incoming messages from this adapter.
		wg.Add(1)
		go func(a PlatformAdapter) {
			defer wg.Done()
			b.processLoop(ctx, a)
		}(adapter)
	}

	<-ctx.Done()
	for _, a := range b.adapters {
		_ = a.Stop()
	}
	wg.Wait()
	return nil
}

func (b *Bridge) processLoop(ctx context.Context, adapter PlatformAdapter) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-adapter.Incoming():
			if !ok {
				return
			}
			b.handleMessage(ctx, adapter, msg)
		}
	}
}

func (b *Bridge) handleMessage(ctx context.Context, adapter PlatformAdapter, msg IncomingMessage) {
	// Deduplication.
	if msg.MsgID != "" {
		b.mu.Lock()
		if _, seen := b.seenMsgs[msg.MsgID]; seen {
			b.mu.Unlock()
			return
		}
		b.seenMsgs[msg.MsgID] = time.Now()
		b.mu.Unlock()
	}

	// Rate limiting.
	b.mu.Lock()
	if last, ok := b.chatLast[msg.ChatID]; ok && time.Since(last) < b.minGap {
		b.mu.Unlock()
		return
	}
	b.chatLast[msg.ChatID] = time.Now()
	b.mu.Unlock()

	// Process message.
	reply, err := b.handler(ctx, msg)
	if err != nil {
		log.Printf("imbridge: handler error for chat %s: %v", msg.ChatID, err)
		reply = "Sorry, an error occurred processing your message."
	}

	if reply != "" {
		if err := adapter.Send(ctx, msg.ChatID, reply); err != nil {
			log.Printf("imbridge: send error to chat %s: %v", msg.ChatID, err)
		}
	}
}

// CleanupSeen removes expired message IDs (call periodically).
func (b *Bridge) CleanupSeen(maxAge time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	cutoff := time.Now().Add(-maxAge)
	for id, t := range b.seenMsgs {
		if t.Before(cutoff) {
			delete(b.seenMsgs, id)
		}
	}
}
