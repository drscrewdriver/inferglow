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
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// TelegramAdapter implements PlatformAdapter for Telegram Bot API using long-polling.
// Uses only the standard library (no external dependencies).
type TelegramAdapter struct {
	token    string
	incoming chan IncomingMessage
	client   *http.Client
	offset   int
	done     chan struct{}
}

// NewTelegramAdapter creates a Telegram bot adapter with the given bot token.
func NewTelegramAdapter(cfg AdapterConfig) *TelegramAdapter {
	pollInterval := cfg.PollInterval
	if pollInterval <= 0 {
		pollInterval = 30
	}
	return &TelegramAdapter{
		token:    cfg.Token,
		incoming: make(chan IncomingMessage, 64),
		client:   &http.Client{Timeout: time.Duration(pollInterval+5) * time.Second},
		done:     make(chan struct{}),
	}
}

func (t *TelegramAdapter) Platform() string { return "telegram" }

func (t *TelegramAdapter) Incoming() <-chan IncomingMessage { return t.incoming }

// Start begins long-polling for updates. Blocks until ctx is cancelled.
func (t *TelegramAdapter) Start(ctx context.Context) error {
	defer close(t.incoming)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		updates, err := t.getUpdates(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// Backoff on error.
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(3 * time.Second):
				continue
			}
		}

		for _, u := range updates {
			if u.Message != nil {
				msg := IncomingMessage{
					ChatID: strconv.FormatInt(u.Message.Chat.ID, 10),
					UserID: strconv.FormatInt(u.Message.From.ID, 10),
					Text:   u.Message.Text,
					MsgID:  strconv.Itoa(u.UpdateID),
				}
				select {
				case t.incoming <- msg:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			t.offset = u.UpdateID + 1
		}
	}
}

// Send delivers a text message to the specified chat.
func (t *TelegramAdapter) Send(ctx context.Context, chatID, text string) error {
	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.token)
	data := url.Values{
		"chat_id": {chatID},
		"text":    {text},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, nil)
	if err != nil {
		return err
	}
	req.URL.RawQuery = data.Encode()

	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram send: status %d", resp.StatusCode)
	}
	return nil
}

// Stop signals the adapter to shut down.
func (t *TelegramAdapter) Stop() error {
	select {
	case <-t.done:
	default:
		close(t.done)
	}
	return nil
}

// --- Telegram API types ---

type tgUpdate struct {
	UpdateID int        `json:"update_id"`
	Message  *tgMessage `json:"message"`
}

type tgMessage struct {
	MessageID int    `json:"message_id"`
	Text      string `json:"text"`
	Chat      tgChat `json:"chat"`
	From      tgUser `json:"from"`
}

type tgChat struct {
	ID int64 `json:"id"`
}

type tgUser struct {
	ID int64 `json:"id"`
}

type tgResponse struct {
	OK     bool       `json:"ok"`
	Result []tgUpdate `json:"result"`
}

func (t *TelegramAdapter) getUpdates(ctx context.Context) ([]tgUpdate, error) {
	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=30",
		t.token, t.offset)

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result tgResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if !result.OK {
		return nil, fmt.Errorf("telegram: api returned ok=false")
	}
	return result.Result, nil
}
