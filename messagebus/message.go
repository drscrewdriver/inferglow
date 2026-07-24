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

// Package messagebus provides a pub/sub message bus abstraction together with
// a dependency-free in-memory implementation. It is the isolated
// infrastructure used by the server for session streaming, live updates and
// cross-component events (wp-c2 / C-3).
package messagebus

import (
	"context"
	"time"
)

// Message is a single event carried by the bus. Offset is the global,
// strictly-monotonic sequence number assigned at publish time; it is the
// anchor used by Replay* to resume from a specific point in the log.
//
// Payload is intentionally left open so the bus stays decoupled from any
// concrete event type. Future consumers (sessions, SSE, schedulers) adapt
// their own event structs into Payload.
type Message struct {
	// ID is a caller-supplied unique identifier (optional).
	ID string
	// SessionID scopes the message to a session; empty means global.
	SessionID string
	// Topic is the broadcast channel name (empty means the global stream).
	Topic string
	// Kind is a caller-defined event kind, e.g. "event" | "session" | "system".
	Kind string
	// Offset is the global monotonic sequence number assigned by the bus.
	Offset int64
	// Payload is the arbitrary event body.
	Payload any
	// Timestamp is the time the message was published.
	Timestamp time.Time
}

// PubSub is the broadcast publish/subscribe channel. Subscribe fans every
// published message out to all current subscribers of that topic.
type PubSub interface {
	// Publish sends msg to every subscriber of topic. It must not block the
	// caller; slow subscribers are dropped rather than stalling production.
	Publish(ctx context.Context, topic string, msg Message) error
	// Subscribe registers a caller to receive a copy of every message
	// published to topic. The returned channel is closed when ctx is
	// cancelled.
	Subscribe(ctx context.Context, topic string) (<-chan Message, error)
}

// StreamDrain is the log-oriented consumption view of the bus: a caller
// drains the ordered message log (optionally filtered by session), either from
// the beginning (Drain*) or from a specific offset onwards (Replay*).
//
// Drain* are exclusive consumer queues: a consumer reads the existing log and
// then receives each subsequently published message until ctx is done.
// Replay* are static snapshots: they return the log history only, without a
// live subscription tail.
type StreamDrain interface {
	// DrainGlobal replays the whole global log then streams new global
	// messages until ctx is cancelled.
	DrainGlobal(ctx context.Context) (<-chan Message, error)
	// DrainSession replays the log filtered by session then streams new
	// messages for that session until ctx is cancelled.
	DrainSession(ctx context.Context, sessionID string) (<-chan Message, error)
	// ReplayGlobal returns the global log entries with Offset >= fromOffset.
	ReplayGlobal(ctx context.Context, fromOffset int64) (<-chan Message, error)
	// ReplaySession returns session-filtered log entries with Offset >= fromOffset.
	ReplaySession(ctx context.Context, sessionID string, fromOffset int64) (<-chan Message, error)
}

// LockManager is a distributed-style advisory lock with a TTL. Acquisition is
// validated by owner token so a released lock cannot be released twice, and an
// expired lock can be re-acquired even if its holder never released it.
type LockManager interface {
	// AcquireLock attempts to take lock key for ttl. It reports whether the
	// acquisition succeeded (false means the key is held by another holder).
	AcquireLock(ctx context.Context, key string, ttl time.Duration) (bool, error)
	// ReleaseLock releases lock key. It must have been acquired by this holder.
	ReleaseLock(ctx context.Context, key string) error
}

// MessageBus is the full bus contract used by the server. It is the union of
// the pub/sub, stream/drain and lock sub-interfaces and matches the C-3 spec.
type MessageBus interface {
	PubSub
	StreamDrain
	LockManager
}