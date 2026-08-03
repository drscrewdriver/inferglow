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

package workspace

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ContextSource implements the taskcontext.ContextSource interface
// for a Workspace, allowing the task context system to enumerate and read
// files from the workspace.
type ContextSource struct {
	ws *Workspace
}

// WorkspaceContextSource is kept for backward compatibility.
//
//nolint:revive
type WorkspaceContextSource = ContextSource

// Compile-time interface check (commented out to avoid import cycle;
// the interface is defined in orchestrator/taskcontext which depends
// on no workspace types).

// NewWorkspaceContextSource creates a context source backed by a Workspace.
func NewWorkspaceContextSource(ws *Workspace) *ContextSource {
	return &ContextSource{ws: ws}
}

// wsDescriptor is a lightweight context descriptor for workspace files.
type wsDescriptor struct {
	Ref         wsRef
	Description string
	SizeHint    int
	UpdatedAt   time.Time
}

// wsRef identifies a file within the workspace.
type wsRef struct {
	Ref string // relative path within workspace
}

// EnumerateDescriptors lists files in the workspace up to limit.
func (s *ContextSource) EnumerateDescriptors(_ context.Context, cursor string, limit int) ([]wsDescriptor, string, error) {
	root := s.ws.Root()
	var files []wsDescriptor

	startAfter := cursor
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		if startAfter != "" && rel <= startAfter {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		files = append(files, wsDescriptor{
			Ref:         wsRef{Ref: rel},
			Description: fmt.Sprintf("workspace file: %s", rel),
			SizeHint:    int(info.Size()),
			UpdatedAt:   info.ModTime(),
		})
		if limit > 0 && len(files) >= limit {
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil && !strings.Contains(err.Error(), "SkipAll") {
		return nil, "", err
	}

	nextCursor := ""
	if limit > 0 && len(files) >= limit {
		nextCursor = files[len(files)-1].Ref.Ref
	}
	return files, nextCursor, nil
}

// ReadFile reads a file from the workspace, up to maxChars.
func (s *ContextSource) ReadFile(relPath string, maxChars int) (string, bool, error) {
	data, err := s.ws.ReadFile(relPath)
	if err != nil {
		return "", false, err
	}
	content := string(data)
	truncated := false
	if maxChars > 0 && len(content) > maxChars {
		content = content[:maxChars]
		truncated = true
	}
	return content, truncated, nil
}

// Workspace is re-exported for convenience.
var _ = (*os.File)(nil) // ensure os is used
