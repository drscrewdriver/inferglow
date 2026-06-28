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
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
// MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO
// EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES
// OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
// ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
// DEALINGS IN THE SOFTWARE.

package actions

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/inferglow/action"
)

// ListDirActionID is the registered Action name for directory listing.
const ListDirActionID = "list_dir"

// ListDirConfig restricts which directories the list_dir Action may access.
type ListDirConfig struct {
	// AllowedDirs is the list of absolute directory paths the action
	// may list. Any path outside these directories is rejected.
	// An empty slice denies all reads.
	AllowedDirs []string
}

// DirEntry is a single entry in a directory listing.
type DirEntry struct {
	Name string `json:"name"`
	Type string `json:"type"` // "file", "dir", "symlink", "other"
	Size int64  `json:"size"`
}

// ListDirResult is the structured payload returned by list_dir.
type ListDirResult struct {
	Path    string     `json:"path"`
	Entries []DirEntry `json:"entries"`
	Count   int        `json:"count"`
}

// listDirExecutor is the ActionExecutor for directory listing.
type listDirExecutor struct {
	cfg ListDirConfig
}

// NewListDirAction builds an Action that lists directory contents restricted
// to cfg.AllowedDirs.
func NewListDirAction(cfg ListDirConfig) *action.Action {
	// Normalize allowed dirs to absolute, cleaned paths.
	for i, d := range cfg.AllowedDirs {
		if abs, err := filepath.Abs(d); err == nil {
			cfg.AllowedDirs[i] = filepath.Clean(abs)
		}
	}
	return &action.Action{
		Name:        ListDirActionID,
		Description: "List the contents of a directory (files and subdirectories).",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string"},
			},
			"required": []string{"path"},
		},
		Executor: &listDirExecutor{cfg: cfg},
		Tags:     []string{"filesystem", "read", "builtin"},
	}
}

// Execute lists the directory contents if it lives under an allowed dir.
func (e *listDirExecutor) Execute(ctx context.Context, input map[string]any) (*action.ActionResult, error) {
	path, _ := input["path"].(string)
	if path == "" {
		return &action.ActionResult{OK: false, Status: "error", Error: "list_dir: path is required"}, nil
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return &action.ActionResult{OK: false, Status: "error", Error: fmt.Sprintf("list_dir: resolve path: %s", err)}, nil
	}

	// Path-traversal guard: ensure absPath is under at least one allowed dir.
	if !isUnderAllowedDir(absPath, e.cfg.AllowedDirs) {
		return &action.ActionResult{
			OK:     false,
			Status: "blocked",
			Error:  fmt.Sprintf("list_dir: path %q is outside allowed directories", absPath),
		}, nil
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return &action.ActionResult{OK: false, Status: "error", Error: fmt.Sprintf("list_dir: stat: %s", err)}, nil
	}
	if !info.IsDir() {
		return &action.ActionResult{OK: false, Status: "error", Error: fmt.Sprintf("list_dir: %q is not a directory", absPath)}, nil
	}

	entries, err := os.ReadDir(absPath)
	if err != nil {
		return &action.ActionResult{OK: false, Status: "error", Error: fmt.Sprintf("list_dir: read: %s", err)}, nil
	}

	result := ListDirResult{
		Path:    absPath,
		Entries: make([]DirEntry, 0, len(entries)),
	}

	for _, entry := range entries {
		e := DirEntry{Name: entry.Name()}
		info, err := entry.Info()
		if err == nil {
			e.Size = info.Size()
		}
		switch {
		case entry.IsDir():
			e.Type = "dir"
		case entry.Type()&os.ModeSymlink != 0:
			e.Type = "symlink"
		case entry.Type().IsRegular():
			e.Type = "file"
		default:
			e.Type = "other"
		}
		result.Entries = append(result.Entries, e)
	}
	result.Count = len(result.Entries)

	return &action.ActionResult{
		OK:     true,
		Status: "success",
		Result: result,
	}, nil
}

// isUnderAllowedDir checks whether absPath is under any of the allowed
// directories. Both absPath and allowed dirs must already be cleaned.
func isUnderAllowedDir(absPath string, allowedDirs []string) bool {
	if len(allowedDirs) == 0 {
		return false
	}
	for _, dir := range allowedDirs {
		if absPath == dir || strings.HasPrefix(absPath, dir+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
