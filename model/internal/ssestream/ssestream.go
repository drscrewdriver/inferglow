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

// Package ssestream contains the common infrastructure shared by the LLM
// provider SSE parsers (OpenAI Chat Completions, Anthropic Messages, and
// OpenAI Responses API).
//
// It is placed under internal/ so only code within the model module can
// import it. The package is generic over the chunk type T to avoid a
// circular dependency on the model package (which defines *StreamChunk,
// *ResultEvent, etc.). Providers instantiate RunLines with T = *StreamChunk.
//
// Extracted as part of the D7-4 audit finding: the three provider SSE
// parsers previously duplicated ~250-300 lines of identical goroutine
// skeleton, emit closure, EOF/error handling, HTTP client setup, and role
// mapping logic.
package ssestream

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultTimeout is the fallback HTTP client timeout for streaming
// responses. Five minutes is long enough for most model responses
// (including reasoning models) while still providing a safety net against
// hung connections.
const DefaultTimeout = 5 * time.Minute

// DefaultBufferSize is the bufio.Reader buffer size used by RunLines. 1MB
// accommodates large SSE lines (e.g. tool_call arguments with big JSON
// payloads) without triggering excessive refills.
const DefaultBufferSize = 1024 * 1024

// DefaultChannelCap is the buffer capacity of the channel returned by
// RunLines. 64 matches the historical provider behavior and provides
// enough backpressure buffering for typical streaming workloads.
const DefaultChannelCap = 64

// EffectiveHTTPClient returns c when non-nil, otherwise a new *http.Client
// with DefaultTimeout. This is the shared implementation of the
// effectiveHTTPClient method that was previously duplicated across
// OpenAICompatibleProvider, AnthropicCompatibleProvider,
// OpenAIResponsesProvider, and OllamaProvider.
func EffectiveHTTPClient(c *http.Client) *http.Client {
	if c != nil {
		return c
	}
	return &http.Client{Timeout: DefaultTimeout}
}

// MapRole applies a role mapping table to a role string. If mapping is nil
// or the role is not present (or maps to an empty string), the role is
// returned unchanged. This is the shared implementation of the mapRole
// method previously duplicated across OpenAICompatibleProvider and
// OpenAIResponsesProvider.
//
// Example: MapRole("developer", map[string]string{"developer": "system"})
// returns "system".
func MapRole(role string, mapping map[string]string) string {
	if mapping == nil {
		return role
	}
	if mapped, ok := mapping[role]; ok && mapped != "" {
		return mapped
	}
	return role
}

// ParseDataLine extracts the payload from an SSE "data:" line. Returns the
// trimmed payload and true when the line is a data line; returns ("", false)
// for non-data lines (event:, id:, retry:, or comment lines starting with ":").
//
// The [DONE] marker is NOT handled here because providers differ in how
// they treat it: OpenAI ignores it (stream terminates via EOF), Anthropic
// never receives it (the message_stop event terminates the stream), and the
// Responses API treats it as a stop signal. Callers handle [DONE]
// provider-specifically after calling ParseDataLine.
func ParseDataLine(line string) (string, bool) {
	if !strings.HasPrefix(line, "data: ") {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(line, "data: ")), true
}

// LineParser parses one SSE line (including the trailing newline) and emits
// zero or more chunks via emit. Returns true to signal that the stream
// should terminate immediately after this line (e.g. on message_stop or
// [DONE]). Returns false to continue reading.
//
// The emit function already handles context cancellation internally, so
// the parser should call emit unconditionally.
type LineParser[T any] func(line string, emit func(T)) bool

// ErrorChunker constructs a terminal chunk carrying a read error. The
// resulting chunk is emitted on the stream before it closes, allowing
// downstream consumers to observe non-EOF errors.
type ErrorChunker[T any] func(err error) T

// RunLines reads SSE lines from body, invokes parse for each line, and
// emits resulting chunks on the returned channel. It encapsulates the
// goroutine skeleton, emit closure, context cancellation polling, and
// EOF/error handling that was previously duplicated across all provider
// RequestModel implementations.
//
// Behavior:
//   - A goroutine is started that reads lines from body using a 1MB
//     bufio.Reader.
//   - Before each read, ctx.Done() is polled so a cancelled consumer
//     doesn't leak the goroutine.
//   - The emit closure selects on ctx.Done() so the goroutine exits even
//     when the channel buffer is full.
//   - On EOF: the final partial line (if non-empty) is parsed, then the
//     channel is closed.
//   - On non-EOF read error: errorChunk(err) is emitted, then the channel
//     is closed.
//   - When parse returns true: the channel is closed immediately.
//   - body is closed when the goroutine exits.
//
// The returned channel is buffered with DefaultChannelCap capacity.
func RunLines[T any](
	ctx context.Context,
	body io.ReadCloser,
	parse LineParser[T],
	errorChunk ErrorChunker[T],
) <-chan T {
	stream := make(chan T, DefaultChannelCap)
	go func() {
		defer close(stream)
		defer body.Close()

		reader := bufio.NewReaderSize(body, DefaultBufferSize)

		emit := func(chunk T) {
			select {
			case stream <- chunk:
			case <-ctx.Done():
			}
		}

		for {
			// Poll context before each read so a cancelled consumer
			// doesn't leak this goroutine.
			select {
			case <-ctx.Done():
				return
			default:
			}

			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					if strings.TrimSpace(line) != "" {
						if stop := parse(line, emit); stop {
							return
						}
					}
					return
				}
				emit(errorChunk(err))
				return
			}

			if stop := parse(line, emit); stop {
				return
			}
		}
	}()
	return stream
}
