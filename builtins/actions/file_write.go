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
	"fmt"
	"os"
	"path/filepath"

	"github.com/inferglow/action"
)

// FileWriteActionID is the registered Action name for file writes.
const FileWriteActionID = "file_write"

// FileWriteConfig restricts which directories the file_write Action may
// write to.
type FileWriteConfig struct {
	// AllowedDirs is the list of absolute directory paths the action
	// may write into. An empty slice denies all writes.
	AllowedDirs []string
}

// fileWriteExecutor is the ActionExecutor for file writes.
type fileWriteExecutor struct {
	cfg FileWriteConfig
}

// FileWriteResult is the structured payload returned by file_write.
type FileWriteResult struct {
	Path         string `json:"path"`
	BytesWritten int64  `json:"bytes_written"`
}

// FileWriteSpec is the ActionSpec for file_write: write side-effect,
// approval required, no sandbox.
var FileWriteSpec = &action.ActionSpec{
	ActionID:         FileWriteActionID,
	Name:             "FileWrite",
	Description:      "Write content to a file within an allowed directory.",
	SideEffectLevel:  action.SideEffectWrite,
	ApprovalRequired: true,
	SandboxRequired:  false,
	ReplaySafe:       false,
	ExposeToModel:    true,
	Tags:             []string{"filesystem", "write", "builtin"},
	Kwargs: map[string]any{
		"path":    map[string]any{"type": "string", "required": true},
		"content": map[string]any{"type": "string", "required": true},
		"append":  map[string]any{"type": "boolean", "required": false},
	},
	Returns: map[string]any{"type": "object"},
	DefaultPolicy: &action.ActionPolicy{
		ReadOnly: false,
	},
}

// NewFileWriteAction builds an Action that writes files restricted to
// cfg.AllowedDirs. Because ApprovalRequired is set, the runtime is
// expected to gate execution behind an approval flow.
func NewFileWriteAction(cfg FileWriteConfig) *action.Action {
	for i, d := range cfg.AllowedDirs {
		if abs, err := filepath.Abs(d); err == nil {
			cfg.AllowedDirs[i] = filepath.Clean(abs)
		}
	}
	return &action.Action{
		Name:        FileWriteActionID,
		Description: "Write content to a file within an allowed directory.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":    map[string]any{"type": "string"},
				"content": map[string]any{"type": "string"},
				"append":  map[string]any{"type": "boolean"},
			},
			"required": []string{"path", "content"},
		},
		Executor: &fileWriteExecutor{cfg: cfg},
		Tags:     []string{"filesystem", "write", "builtin"},
	}
}

// Execute writes the supplied content to path if path lives under an
// allowed directory. When append is true, content is appended instead
// of overwriting.
func (e *fileWriteExecutor) Execute(ctx context.Context, input map[string]any) (*action.ActionResult, error) {
	path, _ := input["path"].(string)
	if path == "" {
		return &action.ActionResult{OK: false, Status: "error", Error: "file_write: path is required"}, nil
	}
	content, _ := input["content"].(string)
	appendMode, _ := input["append"].(bool)

	absPath, err := filepath.Abs(path)
	if err != nil {
		return &action.ActionResult{OK: false, Status: "error", Error: fmt.Sprintf("file_write: resolve path: %s", err)}, nil
	}
	absPath = filepath.Clean(absPath)

	if !isPathAllowed(absPath, e.cfg.AllowedDirs) {
		return &action.ActionResult{
			OK: false, Status: "error",
			Error: fmt.Sprintf("file_write: path %q outside allowed directories", path),
		}, nil
	}

	// Ensure parent directory exists; create it if necessary (still
	// within the allowed dir because absPath was already validated).
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return &action.ActionResult{OK: false, Status: "error", Error: fmt.Sprintf("file_write: mkdir: %s", err)}, nil
	}

	flag := os.O_WRONLY | os.O_CREATE
	if appendMode {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}
	f, err := os.OpenFile(absPath, flag, 0o644)
	if err != nil {
		return &action.ActionResult{OK: false, Status: "error", Error: fmt.Sprintf("file_write: open: %s", err)}, nil
	}
	defer f.Close()

	n, err := f.WriteString(content)
	if err != nil {
		return &action.ActionResult{OK: false, Status: "error", Error: fmt.Sprintf("file_write: write: %s", err)}, nil
	}
	return &action.ActionResult{
		OK:     true,
		Status: "success",
		Result: FileWriteResult{
			Path:         path,
			BytesWritten: int64(n),
		},
	}, nil
}
