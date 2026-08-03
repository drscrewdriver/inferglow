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

package ssestream

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// EffectiveHTTPClient
// ---------------------------------------------------------------------------

func TestEffectiveHTTPClient_NilInput(t *testing.T) {
	client := EffectiveHTTPClient(nil)
	if client == nil {
		t.Fatal("Expected non-nil *http.Client, got nil")
	}
	if client.Timeout != DefaultTimeout {
		t.Fatalf("Expected timeout %v, got %v", DefaultTimeout, client.Timeout)
	}
}

func TestEffectiveHTTPClient_ExistingClient(t *testing.T) {
	original := &http.Client{Timeout: 10 * time.Second}
	got := EffectiveHTTPClient(original)
	if got != original {
		t.Fatal("Expected the same pointer to be returned")
	}
	if got.Timeout != 10*time.Second {
		t.Fatalf("Expected timeout 10s, got %v", got.Timeout)
	}
}

// ---------------------------------------------------------------------------
// MapRole
// ---------------------------------------------------------------------------

func TestMapRole_NormalMapping(t *testing.T) {
	mapping := map[string]string{"developer": "system", "assistant": "model"}
	if got := MapRole("developer", mapping); got != "system" {
		t.Fatalf("MapRole(developer) = %q, want %q", got, "system")
	}
	if got := MapRole("assistant", mapping); got != "model" {
		t.Fatalf("MapRole(assistant) = %q, want %q", got, "model")
	}
}

func TestMapRole_NilMapping(t *testing.T) {
	if got := MapRole("user", nil); got != "user" {
		t.Fatalf("MapRole(user, nil) = %q, want %q", got, "user")
	}
}

func TestMapRole_KeyNotFound(t *testing.T) {
	mapping := map[string]string{"developer": "system"}
	if got := MapRole("user", mapping); got != "user" {
		t.Fatalf("MapRole(user, mapping) = %q, want %q", got, "user")
	}
}

func TestMapRole_EmptyMappingValue(t *testing.T) {
	mapping := map[string]string{"developer": ""}
	if got := MapRole("developer", mapping); got != "developer" {
		t.Fatalf("MapRole(developer) with empty value = %q, want %q", got, "developer")
	}
}

// ---------------------------------------------------------------------------
// ParseDataLine
// ---------------------------------------------------------------------------

func TestParseDataLine_Valid(t *testing.T) {
	payload, ok := ParseDataLine("data: hello world")
	if !ok {
		t.Fatal("Expected ok=true")
	}
	if payload != "hello world" {
		t.Fatalf("Expected %q, got %q", "hello world", payload)
	}
}

func TestParseDataLine_ValidTrimmed(t *testing.T) {
	// TrimSpace strips leading and trailing whitespace after removing the prefix.
	payload, ok := ParseDataLine("data:   hello   ")
	if !ok {
		t.Fatal("Expected ok=true")
	}
	if payload != "hello" {
		t.Fatalf("Expected %q, got %q", "hello", payload)
	}
}

func TestParseDataLine_NonDataLine(t *testing.T) {
	_, ok := ParseDataLine("event: done")
	if ok {
		t.Fatal("Expected ok=false for event: line")
	}

	_, ok = ParseDataLine("id: 1")
	if ok {
		t.Fatal("Expected ok=false for id: line")
	}

	_, ok = ParseDataLine(":comment")
	if ok {
		t.Fatal("Expected ok=false for comment line")
	}
}

func TestParseDataLine_EmptyLine(t *testing.T) {
	payload, ok := ParseDataLine("")
	if ok {
		t.Fatal("Expected ok=false for empty line")
	}
	if payload != "" {
		t.Fatalf("Expected empty payload, got %q", payload)
	}
}

func TestParseDataLine_DataNoSpace(t *testing.T) {
	// "data:" without a following space does not match the "data: " prefix,
	// so it is treated as a non-data line.
	_, ok := ParseDataLine("data:something")
	if ok {
		t.Fatal("Expected ok=false for data:something (no space after colon)")
	}
}

// ---------------------------------------------------------------------------
// RunLines
// ---------------------------------------------------------------------------

// runLinesCollect is a helper that drains a RunLines channel into a slice.
func runLinesCollect[T any](ch <-chan T) []T {
	var out []T
	for v := range ch {
		out = append(out, v)
	}
	return out
}

func TestRunLines_NormalFlow(t *testing.T) {
	body := io.NopCloser(strings.NewReader("data: hello\ndata: world\n"))
	ctx := context.Background()

	parse := func(line string, emit func(string)) bool {
		payload, ok := ParseDataLine(line)
		if !ok {
			return false
		}
		emit(payload)
		return false
	}

	ch := RunLines(ctx, body, parse, nil)
	got := runLinesCollect(ch)

	if len(got) != 2 {
		t.Fatalf("Expected 2 chunks, got %d: %v", len(got), got)
	}
	if got[0] != "hello" {
		t.Fatalf("Chunk[0] = %q, want %q", got[0], "hello")
	}
	if got[1] != "world" {
		t.Fatalf("Chunk[1] = %q, want %q", got[1], "world")
	}
}

func TestRunLines_EOFPartialLine(t *testing.T) {
	// No trailing newline — the final partial line should still be parsed.
	body := io.NopCloser(strings.NewReader("data: first\ndata: second"))
	ctx := context.Background()

	parse := func(line string, emit func(string)) bool {
		payload, ok := ParseDataLine(line)
		if !ok {
			return false
		}
		emit(payload)
		return false
	}

	ch := RunLines(ctx, body, parse, nil)
	got := runLinesCollect(ch)

	if len(got) != 2 {
		t.Fatalf("Expected 2 chunks, got %d: %v", len(got), got)
	}
	if got[0] != "first" {
		t.Fatalf("Chunk[0] = %q, want %q", got[0], "first")
	}
	if got[1] != "second" {
		t.Fatalf("Chunk[1] = %q, want %q", got[1], "second")
	}
}

func TestRunLines_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	body := io.NopCloser(strings.NewReader("data: hello\ndata: world\n"))

	parse := func(line string, emit func(string)) bool {
		payload, ok := ParseDataLine(line)
		if !ok {
			return false
		}
		emit(payload)
		return false
	}

	ch := RunLines(ctx, body, parse, nil)
	got := runLinesCollect(ch)

	// With an already-cancelled context the goroutine should exit before
	// reading any lines, so we expect zero chunks.
	if len(got) != 0 {
		t.Fatalf("Expected 0 chunks after cancel, got %d: %v", len(got), got)
	}
}

func TestRunLines_ParseStop(t *testing.T) {
	body := io.NopCloser(strings.NewReader("data: first\ndata: stop\ndata: second\n"))
	ctx := context.Background()

	parse := func(line string, emit func(string)) bool {
		payload, ok := ParseDataLine(line)
		if !ok {
			return false
		}
		emit(payload)
		return payload == "stop"
	}

	ch := RunLines(ctx, body, parse, nil)
	got := runLinesCollect(ch)

	if len(got) != 2 {
		t.Fatalf("Expected 2 chunks (first, stop), got %d: %v", len(got), got)
	}
	if got[0] != "first" {
		t.Fatalf("Chunk[0] = %q, want %q", got[0], "first")
	}
	if got[1] != "stop" {
		t.Fatalf("Chunk[1] = %q, want %q", got[1], "stop")
	}
}

func TestRunLines_ReadError(t *testing.T) {
	// Use a body that returns an error after a few bytes.
	errRead := errors.New("simulated read error")
	body := &errorReader{msg: "data: hello\n", err: errRead}
	ctx := context.Background()

	parse := func(line string, emit func(string)) bool {
		payload, ok := ParseDataLine(line)
		if !ok {
			return false
		}
		emit(payload)
		return false
	}
	errorChunk := func(err error) string {
		return "error:" + err.Error()
	}

	ch := RunLines(ctx, body, parse, errorChunk)
	got := runLinesCollect(ch)

	if len(got) != 2 {
		t.Fatalf("Expected 2 chunks (hello + error), got %d: %v", len(got), got)
	}
	if got[0] != "hello" {
		t.Fatalf("Chunk[0] = %q, want %q", got[0], "hello")
	}
	if got[1] != "error:"+errRead.Error() {
		t.Fatalf("Chunk[1] = %q, want %q", got[1], "error:"+errRead.Error())
	}
}

// errorReader implements io.ReadCloser. It returns msg bytes first, then err
// on the next Read call, mimicking a body that fails mid-stream.
type errorReader struct {
	msg  string
	err  error
	done bool
}

func (r *errorReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, r.err
	}
	r.done = true
	return copy(p, r.msg), nil
}

func (r *errorReader) Close() error { return nil }