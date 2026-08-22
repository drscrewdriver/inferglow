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

package actions

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/inferglow/action"
)

// FileReadActionID is the registered Action name for file reads.
const FileReadActionID = "file_read"

// DefaultFileReadLimit caps the bytes returned by a single file_read
// invocation (1 MiB).
const DefaultFileReadLimit = 1 << 20

// DefaultHardCapBytes is the ceiling above which a file is never read in
// full: only a statistics digest is returned (anti-OOM guard).
const DefaultHardCapBytes = 64 << 20

// DigestPreviewBytes is the head/tail preview size used by positional
// digests for oversized files.
const DigestPreviewBytes = 4 << 10

// FileReadConfig restricts which directories the file_read Action may
// read from and how oversized files are handled.
type FileReadConfig struct {
	// AllowedDirs is the list of absolute directory paths the action
	// may read from. Any path outside these directories is rejected.
	// An empty slice denies all reads.
	AllowedDirs []string
	// MaxBytes caps the read size. Zero means DefaultFileReadLimit.
	MaxBytes int64
	// MaxInlineBytes is the largest file inlined into the result.
	// Zero means DefaultMaxInlineBytes.
	MaxInlineBytes int64
	// HardCapBytes is the ceiling above which the file is never read
	// in full. Zero means DefaultHardCapBytes.
	HardCapBytes int64
	// SpillStore, when set, receives the full content of oversized
	// files (MaxInlineBytes < size < HardCapBytes) so the model can
	// fetch segments on demand via the returned locator hint.
	SpillStore action.SpillStore
	// SpillOwner is the storage namespace (typically the session id)
	// passed to SpillStore. Empty means the file path is used.
	SpillOwner string
}

// fileReadExecutor is the ActionExecutor for file reads.
type fileReadExecutor struct {
	cfg FileReadConfig
}

// FileReadResult is the structured payload returned by file_read.
//
// Oversized files carry a PositionalDigest plus a spill Locator/RetrievalHint
// instead of inline Content, so the model can fetch any segment with a
// ranged read (offset/limit) without blowing up the context window.
type FileReadResult struct {
	Path          string                   `json:"path"`
	BytesRead     int64                    `json:"bytes_read"`
	Content       string                   `json:"content,omitempty"`
	Offset        int64                    `json:"offset,omitempty"`
	TotalBytes    int64                    `json:"total_bytes,omitempty"`
	Truncated     bool                     `json:"truncated,omitempty"`
	Locator       string                   `json:"locator,omitempty"`
	RetrievalHint string                   `json:"retrieval_hint,omitempty"`
	Digest        *action.PositionalDigest `json:"digest,omitempty"`
}

// FileReadSpec is the ActionSpec for file_read: read-only, no approval,
// no sandbox.
var FileReadSpec = &action.ActionSpec{
	ActionID:         FileReadActionID,
	Name:             "FileRead",
	Description:      "Read a file from an allowed directory (supports offset/limit ranged reads; oversized files return a positional digest + spill locator).",
	SideEffectLevel:  action.SideEffectRead,
	ApprovalRequired: false,
	SandboxRequired:  false,
	ReplaySafe:       true,
	ExposeToModel:    true,
	Tags:             []string{"filesystem", "read", "builtin"},
	Kwargs: map[string]any{
		"path":      map[string]any{"type": "string", "required": true},
		"max_bytes": map[string]any{"type": "integer", "required": false},
		"offset":    map[string]any{"type": "integer", "required": false},
		"limit":     map[string]any{"type": "integer", "required": false},
	},
	Returns:       map[string]any{"type": "object"},
	DefaultPolicy: &action.ActionPolicy{MaxOutputBytes: DefaultFileReadLimit},
}

// NewFileReadAction builds an Action that reads files restricted to
// cfg.AllowedDirs.
func NewFileReadAction(cfg FileReadConfig) *action.Action {
	if cfg.MaxBytes <= 0 {
		cfg.MaxBytes = DefaultFileReadLimit
	}
	if cfg.MaxInlineBytes <= 0 {
		cfg.MaxInlineBytes = action.DefaultMaxInlineBytes
	}
	if cfg.HardCapBytes <= 0 {
		cfg.HardCapBytes = DefaultHardCapBytes
	}
	// Normalize allowed dirs to absolute, cleaned paths.
	for i, d := range cfg.AllowedDirs {
		if abs, err := filepath.Abs(d); err == nil {
			cfg.AllowedDirs[i] = filepath.Clean(abs)
		}
	}
	return &action.Action{
		Name:        FileReadActionID,
		Description: "Read a file from an allowed directory (supports offset/limit ranged reads; oversized files return a positional digest + spill locator).",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":      map[string]any{"type": "string"},
				"max_bytes": map[string]any{"type": "integer"},
				"offset":    map[string]any{"type": "integer"},
				"limit":     map[string]any{"type": "integer"},
			},
			"required": []string{"path"},
		},
		Executor: &fileReadExecutor{cfg: cfg},
		Tags:     []string{"filesystem", "read", "builtin"},
	}
}

// Execute reads the requested file if it lives under an allowed dir.
//
// Behavior by size (when no offset/limit is given):
//   - size <= MaxInlineBytes: content inlined, unchanged behavior
//   - MaxInlineBytes < size < HardCapBytes with SpillStore configured:
//     full content persisted, result carries a positional digest + locator
//   - size >= HardCapBytes: only a statistics digest returned (anti-OOM)
//
// With offset/limit, exactly that byte range is read inline (limit capped by
// MaxBytes), enabling git-style chunked reads of oversized files.
func (e *fileReadExecutor) Execute(ctx context.Context, input map[string]any) (*action.ActionResult, error) {
	path, _ := input["path"].(string)
	if path == "" {
		return &action.ActionResult{OK: false, Status: "error", Error: "file_read: path is required"}, nil
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return &action.ActionResult{OK: false, Status: "error", Error: fmt.Sprintf("file_read: resolve path: %s", err)}, nil
	}
	absPath = filepath.Clean(absPath)

	// Resolve symlinks before the allow-list check so a link inside an
	// allowed dir cannot escape to a target outside it. If the file
	// does not exist yet, EvalSymlinks returns an error that we surface
	// as "open" so callers see a consistent "not found" message.
	resolved, err := filepath.EvalSymlinks(absPath)
	if err == nil {
		absPath = filepath.Clean(resolved)
	}

	if !isPathAllowed(absPath, e.cfg.AllowedDirs) {
		return &action.ActionResult{
			OK: false, Status: "error",
			Error: fmt.Sprintf("file_read: path %q outside allowed directories", path),
		}, nil
	}

	maxBytes := e.cfg.MaxBytes
	if mb, ok := input["max_bytes"]; ok {
		if v, ok := toInt64(mb); ok && v > 0 && v < maxBytes {
			maxBytes = v
		}
	}

	f, err := os.Open(absPath)
	if err != nil {
		return &action.ActionResult{OK: false, Status: "error", Error: fmt.Sprintf("file_read: open: %s", err)}, nil
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return &action.ActionResult{OK: false, Status: "error", Error: fmt.Sprintf("file_read: stat: %s", err)}, nil
	}
	totalBytes := info.Size()

	// Ranged read: return exactly the requested segment inline.
	if offset, ok := toInt64(input["offset"]); ok {
		return e.readRange(f, path, offset, input, maxBytes, totalBytes)
	}

	// Small file: inline as before (read maxBytes+1 to detect truncation).
	if totalBytes <= e.cfg.MaxInlineBytes {
		buf := make([]byte, maxBytes+1)
		n, err := f.Read(buf)
		if err != nil && n == 0 {
			return &action.ActionResult{OK: false, Status: "error", Error: fmt.Sprintf("file_read: read: %s", err)}, nil
		}
		truncated := false
		if int64(n) > maxBytes {
			n = int(maxBytes)
			truncated = true
		}
		content := string(buf[:n])
		result := FileReadResult{
			Path:       path,
			BytesRead:  int64(n),
			Content:    content,
			TotalBytes: totalBytes,
			Truncated:  truncated,
		}
		meta := map[string]any{}
		if truncated {
			meta["truncated"] = true
		}
		return &action.ActionResult{OK: true, Status: "success", Result: result, Metadata: meta}, nil
	}

	// Oversized file: never inline full content.
	if totalBytes >= e.cfg.HardCapBytes {
		return e.digestOnly(path, f, totalBytes)
	}
	if e.cfg.SpillStore != nil {
		return e.spillOversize(ctx, path, f, totalBytes)
	}
	// No spill backend: fall back to digest-only (never inline).
	return e.digestOnly(path, f, totalBytes)
}

// readRange returns one byte range [offset, offset+limit) inline.
func (e *fileReadExecutor) readRange(f *os.File, path string, offset int64, input map[string]any, maxBytes, totalBytes int64) (*action.ActionResult, error) {
	if offset < 0 {
		return &action.ActionResult{OK: false, Status: "error", Error: "file_read: offset must be non-negative"}, nil
	}
	if offset >= totalBytes {
		return &action.ActionResult{OK: false, Status: "error", Error: fmt.Sprintf("file_read: offset %d beyond end of file (%d bytes)", offset, totalBytes)}, nil
	}
	limit := maxBytes
	if lv, ok := toInt64(input["limit"]); ok && lv > 0 && lv < limit {
		limit = lv
	}
	requested := limit
	if offset+limit > totalBytes {
		limit = totalBytes - offset
	}
	buf := make([]byte, limit)
	n, err := f.ReadAt(buf, offset)
	if err != nil && n == 0 {
		return &action.ActionResult{OK: false, Status: "error", Error: fmt.Sprintf("file_read: read at %d: %s", offset, err)}, nil
	}
	content := string(buf[:n])
	result := FileReadResult{
		Path:       path,
		BytesRead:  int64(n),
		Content:    content,
		Offset:     offset,
		TotalBytes: totalBytes,
		Truncated:  int64(n) < requested,
	}
	meta := map[string]any{}
	if result.Truncated {
		meta["truncated"] = true
	}
	return &action.ActionResult{OK: true, Status: "success", Result: result, Metadata: meta}, nil
}

// digestOnly returns statistics plus a small head/tail preview without
// reading the whole file (anti-OOM guard for files at/above the hard cap).
func (e *fileReadExecutor) digestOnly(path string, f *os.File, totalBytes int64) (*action.ActionResult, error) {
	head := make([]byte, DigestPreviewBytes)
	nHead, err := f.ReadAt(head, 0)
	if err != nil && nHead == 0 {
		return &action.ActionResult{OK: false, Status: "error", Error: fmt.Sprintf("file_read: read head: %s", err)}, nil
	}
	head = head[:nHead]

	tailLen := int64(DigestPreviewBytes)
	if tailLen > totalBytes {
		tailLen = totalBytes
	}
	tail := make([]byte, tailLen)
	nTail, err := f.ReadAt(tail, totalBytes-tailLen)
	if err != nil && nTail == 0 {
		return &action.ActionResult{OK: false, Status: "error", Error: fmt.Sprintf("file_read: read tail: %s", err)}, nil
	}
	tail = tail[:nTail]

	digest := &action.PositionalDigest{
		Head:       string(head),
		Tail:       string(tail),
		HeadBytes:  len(head),
		TailBytes:  len(tail),
		Offset:     totalBytes - int64(len(tail)),
		TotalBytes: totalBytes,
		Lines:      countLines(f),
		Truncated:  true,
	}
	result := FileReadResult{
		Path:       path,
		BytesRead:  int64(len(head) + len(tail)),
		TotalBytes: totalBytes,
		Truncated:  true,
		Digest:     digest,
	}
	return &action.ActionResult{OK: true, Status: "success", Result: result, Metadata: map[string]any{"refused_inline": true}}, nil
}

// spillOversize persists the full content and returns a digest + locator.
func (e *fileReadExecutor) spillOversize(ctx context.Context, path string, f *os.File, totalBytes int64) (*action.ActionResult, error) {
	full, err := os.ReadFile(f.Name())
	if err != nil {
		return &action.ActionResult{OK: false, Status: "error", Error: fmt.Sprintf("file_read: read full: %s", err)}, nil
	}
	owner := e.cfg.SpillOwner
	if owner == "" {
		owner = f.Name()
	}
	ref, err := e.cfg.SpillStore.SaveText(ctx, owner, FileReadActionID, filepath.Base(path), string(full))
	if err != nil {
		// Best-effort: keep the digest-only fallback instead of failing
		// the successful read.
		return e.digestOnly(path, f, totalBytes)
	}
	digest := action.NewPositionalDigest(full, DigestPreviewBytes, DigestPreviewBytes)
	result := FileReadResult{
		Path:          path,
		BytesRead:     ref.Bytes,
		TotalBytes:    totalBytes,
		Truncated:     true,
		Locator:       ref.Locator,
		RetrievalHint: ref.RetrievalHint,
		Digest:        digest,
	}
	return &action.ActionResult{OK: true, Status: "success", Result: result, Metadata: map[string]any{"spilled": true}}, nil
}

// countLines estimates the line count of f by scanning from the current
// offset; it is only used for large-file digests where the full content
// is not in memory.
func countLines(f *os.File) int {
	if _, err := f.Seek(0, 0); err != nil {
		return 0
	}
	defer f.Seek(0, 0)
	buf := make([]byte, 64<<10)
	lines := 0
	for {
		n, err := f.Read(buf)
		lines += bytesCount(buf[:n], '\n')
		if err != nil {
			break
		}
	}
	return lines
}

// bytesCount returns the number of occurrences of b in s.
func bytesCount(s []byte, b byte) int {
	count := 0
	for _, c := range s {
		if c == b {
			count++
		}
	}
	return count
}

// toInt64 converts common JSON number shapes to int64.
func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		if n < 0 {
			return 0, false
		}
		return int64(n), true
	case int:
		if n < 0 {
			return 0, false
		}
		return int64(n), true
	case int64:
		if n < 0 {
			return 0, false
		}
		return n, true
	case json.Number:
		if i, err := n.Int64(); err == nil && i >= 0 {
			return i, true
		}
	}
	return 0, false
}

// isPathAllowed reports whether absPath is contained within any of the
// allowed directories. Both sides are expected to be cleaned absolute
// paths. A path equal to an allowed dir is also rejected (only files
// inside are readable).
func isPathAllowed(absPath string, allowedDirs []string) bool {
	for _, dir := range allowedDirs {
		dir = filepath.Clean(dir)
		if absPath == dir {
			continue
		}
		rel, err := filepath.Rel(dir, absPath)
		if err != nil {
			continue
		}
		if rel == "." {
			continue
		}
		if !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel) {
			return true
		}
	}
	return false
}
