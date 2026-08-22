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

package action

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DefaultMaxInlineBytes is the largest tool result inlined into an
// ActionResult before a spill policy replaces it with a digest.
const DefaultMaxInlineBytes = 1 << 20

// DigestPreviewBytes is the head/tail preview size used by positional
// digests for oversized content.
const DigestPreviewBytes = 4 << 10

// SpillRef is the result of persisting oversized tool output to a spill store.
//
// Locator is an opaque model-facing handle: the local backend renders it as
// a filesystem path, while a remote or database backend could render a URI
// or key. Consumers must not parse the Locator; they render RetrievalHint
// instead (e.g. "use file_read offset=...").
type SpillRef struct {
	// Locator is the opaque handle for one spilled artifact.
	Locator string `json:"locator"`
	// Bytes is the exact byte length of the persisted content.
	Bytes int64 `json:"bytes"`
	// RetrievalHint is backend-supplied guidance for fetching the content.
	RetrievalHint string `json:"retrieval_hint"`
}

// SpillStore persists oversized text and returns a model-facing reference.
//
// Implementations must persist the FULL content verbatim and reject on a
// real storage failure (permissions, ENOSPC, backend unavailable). Callers
// decide how to degrade: the registry spill policy treats a rejection as
// best-effort and keeps the original inline result.
type SpillStore interface {
	// SaveText persists content under the owner namespace (typically a
	// session id) with a descriptive source label and a suggested base
	// name that the backend sanitizes into a single safe path segment.
	SaveText(ctx context.Context, owner, source, suggestedName, content string) (*SpillRef, error)
}

// LocalSpillStore writes spill artifacts under a private root directory.
//
// Artifacts are stored at <root>/spill/<sha256(owner)>/<random>-<safeName>
// with a 0700 private root and an exclusive owner-only create (O_EXCL, 0600)
// so a planted symlink cannot redirect the write.
type LocalSpillStore struct {
	root string
}

// NewLocalSpillStore creates a spill store rooted at root (created on first
// save if missing). The root is normalized to an absolute, cleaned path.
func NewLocalSpillStore(root string) *LocalSpillStore {
	return &LocalSpillStore{root: filepath.Clean(root)}
}

// SaveText persists content under the session-scoped owner directory.
func (s *LocalSpillStore) SaveText(ctx context.Context, owner, source, suggestedName, content string) (*SpillRef, error) {
	ownerDir := filepath.Join(s.root, "spill", ownerHash(owner))
	if err := os.MkdirAll(ownerDir, 0o700); err != nil {
		return nil, fmt.Errorf("spill: create owner dir: %w", err)
	}
	safeName := sanitizeName(suggestedName)
	path := filepath.Join(ownerDir, randomPrefix()+"-"+safeName)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, fmt.Errorf("spill: create artifact: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("spill: write artifact: %w", err)
	}
	return &SpillRef{
		Locator:       path,
		Bytes:         int64(len(content)),
		RetrievalHint: fmt.Sprintf("content spilled by %s: use file_read with path %q and offset=<n> to fetch segments", source, path),
	}, nil
}

// randomPrefix returns a short random hex token for collision-free artifact names.
func randomPrefix() string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// ownerHash returns a stable hex digest of the owner namespace.
func ownerHash(owner string) string {
	sum := sha256.Sum256([]byte(owner))
	return hex.EncodeToString(sum[:16])
}

// sanitizeName reduces a caller-suggested name to a single safe path segment.
// The result never contains path separators, dot-dot segments, or control
// characters; empty input falls back to "artifact".
func sanitizeName(suggested string) string {
	s := strings.TrimSpace(suggested)
	s = strings.ReplaceAll(s, "\\", "_")
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return '_'
		}
		return r
	}, s)
	s = strings.Trim(s, "._ ")
	if s == "" || s == "." || s == ".." {
		return "artifact"
	}
	return s
}

// PositionalDigest is a head/tail preview with byte-position statistics for
// oversized file content. The model sees stable offsets and sizes, and can
// fetch any segment with a ranged read (e.g. file_read offset=<n> limit=<k>).
type PositionalDigest struct {
	Head       string `json:"head"`                  // head preview text
	Tail       string `json:"tail"`                  // tail preview text
	HeadBytes  int    `json:"head_bytes"`            // bytes actually kept in Head
	TailBytes  int    `json:"tail_bytes"`            // bytes actually kept in Tail
	Offset     int64  `json:"offset"`                // byte offset where Tail begins
	TotalBytes int64  `json:"total_bytes"`           // full content size
	Lines      int    `json:"lines"`                 // line count of full content
	Truncated  bool   `json:"truncated"`             // true when head+tail < total
}

// NewPositionalDigest builds a digest from full content with bounded head and
// tail preview sizes. headBytes and tailBytes are capped at the content size;
// zero or negative preview sizes disable the corresponding side.
func NewPositionalDigest(content []byte, headBytes, tailBytes int) *PositionalDigest {
	total := int64(len(content))
	if headBytes < 0 {
		headBytes = 0
	}
	if tailBytes < 0 {
		tailBytes = 0
	}
	lines := 0
	if total > 0 {
		lines = 1 + strings.Count(string(content), "\n")
	}
	d := &PositionalDigest{
		TotalBytes: total,
		Lines:      lines,
	}
	if headBytes > 0 && total > 0 {
		if headBytes > len(content) {
			headBytes = len(content)
		}
		d.Head = string(content[:headBytes])
		d.HeadBytes = headBytes
	}
	if tailBytes > 0 && total > 0 {
		if tailBytes > len(content) {
			tailBytes = len(content)
		}
		d.Tail = string(content[len(content)-tailBytes:])
		d.TailBytes = tailBytes
	}
	d.Offset = total - int64(d.TailBytes)
	d.Truncated = int64(d.HeadBytes+d.TailBytes) < total
	return d
}

// SpilledOutput is the digest-shaped replacement for an oversized inline
// result after spill persistence. It carries the opaque locator and the
// model-facing retrieval guidance.
type SpilledOutput struct {
	Locator       string            `json:"locator"`
	Bytes         int64             `json:"bytes"`
	RetrievalHint string            `json:"retrieval_hint"`
	Digest        *PositionalDigest `json:"digest,omitempty"`
}

// OutputSpiller is an optional post-execute hook attached to an
// ActionRegistry. It inspects a successful result and, when its text is
// oversized, persists it to a SpillStore and replaces result.Result with a
// SpilledOutput digest. A returned error is best-effort: the registry keeps
// the result unchanged and never turns a successful call into an error.
type OutputSpiller interface {
	Spill(ctx context.Context, result *ActionResult) error
}

// DefaultSpiller implements OutputSpiller for plain-text string results.
// Results that are not strings, or fit within MaxInlineBytes, pass through
// untouched.
type DefaultSpiller struct {
	Store          SpillStore
	Owner          string // storage namespace (session id)
	Source         string // descriptive producer label
	MaxInlineBytes int    // zero means DefaultMaxInlineBytes
}

// NewDefaultSpiller builds a spiller with the default inline budget.
func NewDefaultSpiller(store SpillStore, owner, source string) *DefaultSpiller {
	return &DefaultSpiller{Store: store, Owner: owner, Source: source}
}

// Spill persists an oversized string result and replaces it with a digest.
func (s *DefaultSpiller) Spill(ctx context.Context, result *ActionResult) error {
	if result == nil || result.Result == nil {
		return nil
	}
	text, ok := result.Result.(string)
	if !ok {
		return nil
	}
	maxInline := s.MaxInlineBytes
	if maxInline <= 0 {
		maxInline = DefaultMaxInlineBytes
	}
	if len(text) <= maxInline {
		return nil
	}
	if s.Store == nil {
		return ErrSpillStoreRequired
	}
	ref, err := s.Store.SaveText(ctx, s.Owner, s.Source, "result", text)
	if err != nil {
		return err
	}
	result.Result = SpilledOutput{
		Locator:       ref.Locator,
		Bytes:         ref.Bytes,
		RetrievalHint: ref.RetrievalHint,
		Digest:        NewPositionalDigest([]byte(text), DigestPreviewBytes, DigestPreviewBytes),
	}
	if result.Metadata == nil {
		result.Metadata = map[string]any{}
	}
	result.Metadata["spilled"] = true
	return nil
}

// ErrSpillStoreRequired is returned when a spill policy is attached but no
// SpillStore is configured.
var ErrSpillStoreRequired = errors.New("spill store required")
