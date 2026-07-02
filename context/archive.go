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

package contextmgr

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ArchivedMessage is the on-disk format for a single archived message.
// It mirrors session.ChatMessage but lives in the context package to
// avoid an import cycle.
type ArchivedMessage struct {
	Role      string         `json:"role"`
	Content   any            `json:"content"`
	Name      string         `json:"name,omitempty"`
	Meta      map[string]any `json:"meta,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
}

// ArchiveMessages writes dropped messages to a timestamped .jsonl file
// under dir. Each line is one ArchivedMessage JSON. The file is created
// with Write + Sync to guarantee durability. Returns the archive path.
func ArchiveMessages(dir string, msgs []ArchivedMessage) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("archive: create dir: %w", err)
	}

	name := time.Now().Format("20060102-150405.000") + ".jsonl"
	path := filepath.Join(dir, name)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return "", fmt.Errorf("archive: open file: %w", err)
	}

	for _, msg := range msgs {
		data, err := json.Marshal(msg)
		if err != nil {
			f.Close()
			return "", fmt.Errorf("archive: marshal: %w", err)
		}
		if _, err := f.Write(append(data, '\n')); err != nil {
			f.Close()
			return "", fmt.Errorf("archive: write: %w", err)
		}
	}

	// Write + Sync: ensure all archived messages are durable on disk.
	if err := f.Sync(); err != nil {
		f.Close()
		return "", fmt.Errorf("archive: sync: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("archive: close: %w", err)
	}

	return path, nil
}
