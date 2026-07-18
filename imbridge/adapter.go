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

// Package imbridge provides IM platform adapters for InferGlow (OT-9).
// It defines a unified adapter interface and a bridge core that routes
// incoming messages to the agent and sends replies back.
package imbridge

import "context"

// IncomingMessage represents a message received from an IM platform.
type IncomingMessage struct {
	ChatID  string // Platform-specific chat/channel identifier
	UserID  string // Sender identifier
	Text    string // Message text content
	MsgID   string // Platform message ID (used for deduplication)
	ReplyTo string // Optional: message ID being replied to
}

// PlatformAdapter is the interface that IM platform implementations must satisfy.
type PlatformAdapter interface {
	// Start begins listening for incoming messages. Blocks until ctx is cancelled.
	Start(ctx context.Context) error
	// Incoming returns a channel of received messages.
	Incoming() <-chan IncomingMessage
	// Send delivers a text reply to the specified chat.
	Send(ctx context.Context, chatID, text string) error
	// Stop gracefully shuts down the adapter.
	Stop() error
	// Platform returns the platform identifier (e.g., "telegram", "feishu").
	Platform() string
}

// AdapterConfig holds common configuration for platform adapters.
type AdapterConfig struct {
	// Token is the platform bot token or API key.
	Token string
	// WebhookURL is the public URL for webhook-based platforms (optional).
	WebhookURL string
	// PollInterval is the long-polling interval for polling-based platforms.
	PollInterval int // seconds, default 30
}
