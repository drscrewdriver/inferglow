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

package team

import (
	"sync"
	"time"
)

// Message is a lightweight inter-agent message carried by the messageBus.
type Message struct {
	From      string
	To        string
	Content   string
	Timestamp time.Time
	Metadata  map[string]any
}

// messageBus is the Coordinator's private message store.
// It is NOT a global EventBus — it exists only for the lifetime of a
// Coordinator and is not accessible outside the team package.
//
// Messages are stored (not consumed) so that multiple members can read
// the history during handoff scenarios.
type messageBus struct {
	mu       sync.RWMutex
	messages []Message
}

// newMessageBus creates an empty messageBus.
func newMessageBus() *messageBus {
	return &messageBus{
		messages: make([]Message, 0),
	}
}

// Post adds a message to the bus.
func (b *messageBus) Post(msg Message) {
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}
	b.mu.Lock()
	b.messages = append(b.messages, msg)
	b.mu.Unlock()
}

// History returns a copy of all messages in chronological order.
func (b *messageBus) History() []Message {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]Message, len(b.messages))
	copy(out, b.messages)
	return out
}

// MessagesTo returns all messages addressed to the given role.
func (b *messageBus) MessagesTo(role string) []Message {
	b.mu.RLock()
	defer b.mu.RUnlock()
	var out []Message
	for _, m := range b.messages {
		if m.To == role {
			out = append(out, m)
		}
	}
	return out
}

// Clear removes all messages from the bus.
func (b *messageBus) Clear() {
	b.mu.Lock()
	b.messages = b.messages[:0]
	b.mu.Unlock()
}
