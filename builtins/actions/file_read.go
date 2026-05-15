package actions

import (
	"context"
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

// FileReadConfig restricts which directories the file_read Action may
// read from.
type FileReadConfig struct {
	// AllowedDirs is the list of absolute directory paths the action
	// may read from. Any path outside these directories is rejected.
	// An empty slice denies all reads.
	AllowedDirs []string
	// MaxBytes caps the read size. Zero means DefaultFileReadLimit.
	MaxBytes int64
}

// fileReadExecutor is the ActionExecutor for file reads.
type fileReadExecutor struct {
	cfg FileReadConfig
}

// FileReadResult is the structured payload returned by file_read.
type FileReadResult struct {
	Path      string `json:"path"`
	BytesRead int64  `json:"bytes_read"`
	Content   string `json:"content"`
}

// FileReadSpec is the ActionSpec for file_read: read-only, no approval,
// no sandbox.
var FileReadSpec = &action.ActionSpec{
	ActionID:         FileReadActionID,
	Name:             "FileRead",
	Description:      "Read a file from an allowed directory.",
	SideEffectLevel:  action.SideEffectRead,
	ApprovalRequired: false,
	SandboxRequired:  false,
	ReplaySafe:       true,
	ExposeToModel:    true,
	Tags:             []string{"filesystem", "read", "builtin"},
	Kwargs: map[string]any{
		"path":       map[string]any{"type": "string", "required": true},
		"max_bytes":  map[string]any{"type": "integer", "required": false},
	},
	Returns:        map[string]any{"type": "object"},
	DefaultPolicy:  &action.ActionPolicy{MaxOutputBytes: DefaultFileReadLimit},
}

// NewFileReadAction builds an Action that reads files restricted to
// cfg.AllowedDirs.
func NewFileReadAction(cfg FileReadConfig) *action.Action {
	if cfg.MaxBytes <= 0 {
		cfg.MaxBytes = DefaultFileReadLimit
	}
	// Normalize allowed dirs to absolute, cleaned paths.
	for i, d := range cfg.AllowedDirs {
		if abs, err := filepath.Abs(d); err == nil {
			cfg.AllowedDirs[i] = filepath.Clean(abs)
		}
	}
	return &action.Action{
		Name:        FileReadActionID,
		Description: "Read a file from an allowed directory.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":      map[string]any{"type": "string"},
				"max_bytes": map[string]any{"type": "integer"},
			},
			"required": []string{"path"},
		},
		Executor: &fileReadExecutor{cfg: cfg},
		Tags:     []string{"filesystem", "read", "builtin"},
	}
}

// Execute reads the requested file if it lives under an allowed dir.
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
		switch v := mb.(type) {
		case float64:
			if v > 0 && int64(v) < maxBytes {
				maxBytes = int64(v)
			}
		case int:
			if v > 0 && int64(v) < maxBytes {
				maxBytes = int64(v)
			}
		case int64:
			if v > 0 && v < maxBytes {
				maxBytes = v
			}
		}
	}

	f, err := os.Open(absPath)
	if err != nil {
		return &action.ActionResult{OK: false, Status: "error", Error: fmt.Sprintf("file_read: open: %s", err)}, nil
	}
	defer f.Close()

	// Read at most maxBytes+1 to detect truncation.
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
		Path:      path,
		BytesRead: int64(n),
		Content:   content,
	}
	meta := map[string]any{}
	if truncated {
		meta["truncated"] = true
	}
	return &action.ActionResult{OK: true, Status: "success", Result: result, Metadata: meta}, nil
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
