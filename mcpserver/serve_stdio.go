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

package mcpserver

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// StdioFrameTransport implements FrameTransport over stdin/stdout,
// exchanging newline-delimited JSON-RPC 2.0 frames.
type StdioFrameTransport struct {
	// Reader is the input source. Defaults to os.Stdin.
	Reader io.Reader
	// Writer is the output destination. Defaults to os.Stdout.
	Writer io.Writer

	reader *bufio.Reader
	writer *bufio.Writer
}

// NewStdioFrameTransport creates a StdioFrameTransport using os.Stdin/os.Stdout.
func NewStdioFrameTransport() *StdioFrameTransport {
	return &StdioFrameTransport{
		Reader: os.Stdin,
		Writer: os.Stdout,
	}
}

// Send writes a JSON-RPC response followed by a newline.
func (t *StdioFrameTransport) Send(_ context.Context, data []byte) error {
	if t.writer == nil {
		w := t.Writer
		if w == nil {
			w = os.Stdout
		}
		t.writer = bufio.NewWriter(w)
	}
	if _, err := t.writer.Write(data); err != nil {
		return err
	}
	if err := t.writer.WriteByte('\n'); err != nil {
		return err
	}
	return t.writer.Flush()
}

// Recv reads the next newline-delimited JSON-RPC request.
func (t *StdioFrameTransport) Recv(ctx context.Context) ([]byte, error) {
	if t.reader == nil {
		r := t.Reader
		if r == nil {
			r = os.Stdin
		}
		t.reader = bufio.NewReaderSize(r, 1024*1024)
	}

	// Check context before blocking read
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	line, err := t.reader.ReadBytes('\n')
	if err != nil {
		return nil, err
	}

	// Trim trailing newline
	line = line[:len(line)-1]
	if len(line) > 0 && line[len(line)-1] == '\r' {
		line = line[:len(line)-1]
	}

	// Validate it's valid JSON
	var check json.RawMessage
	if err := json.Unmarshal(line, &check); err != nil {
		return nil, fmt.Errorf("invalid JSON frame: %w", err)
	}

	return line, nil
}

// Close is a no-op for stdio transport.
func (t *StdioFrameTransport) Close() error {
	return nil
}
