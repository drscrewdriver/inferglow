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

package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	// workspaceHistoryMax is the max number of persisted workspace entries.
	workspaceHistoryMax = 10
	// workspaceHistoryFile is the persistence path under the data dir.
	workspaceHistoryFile = "workspace_history.json"
)

// WorkspaceInfo holds the current workspace state (SC-5).
type WorkspaceInfo struct {
	Path        string   // current working directory (absolute)
	Previous    string   // previous workspace (for `-` switching)
	History     []string // recent workspaces (most recent first, max 10)
	ProjectName string   // project name (directory name)
}

// WorkspaceSwitchMode selects the workspace interaction mode (SC-5).
type WorkspaceSwitchMode int

const (
	WorkspaceNormal  WorkspaceSwitchMode = iota // normal: switch by path
	WorkspaceHistory                            // history list mode
	WorkspaceBrowse                             // directory browse mode (reserved)
)

// WorkspaceSwitch is the workspace directory switching state (SC-5).
type WorkspaceSwitch struct {
	active  bool
	current WorkspaceInfo
	mode    WorkspaceSwitchMode
	// historyPath, when non-empty, overrides the default persistence path
	// (workspaceHistoryPath). Used by tests to avoid writing the real
	// ~/.inferglow/workspace_history.json. Empty = default behavior.
	historyPath string
}

// newWorkspaceSwitch initializes the switcher with the process cwd and the
// persisted history. History load failures are non-fatal.
func newWorkspaceSwitch() *WorkspaceSwitch {
	w := &WorkspaceSwitch{current: WorkspaceInfo{Path: mustGetwd()}}
	w.current.ProjectName = filepath.Base(w.current.Path)
	w.loadHistory()
	return w
}

func mustGetwd() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	return dir
}

// GetCurrentDir returns the current working directory (absolute).
func (w *WorkspaceSwitch) GetCurrentDir() string {
	return w.current.Path
}

// SetCurrentDir resolves path to an absolute directory and switches to it.
// On success the previous directory is recorded and the path is added to the
// history. The process working directory is changed via os.Chdir.
func (w *WorkspaceSwitch) SetCurrentDir(path string) error {
	if path == "" {
		path = "."
	}
	if path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("home directory unavailable: %w", err)
		}
		path = home
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", abs)
	}
	old := w.current.Path
	w.current.Previous = old
	w.current.Path = abs
	w.current.ProjectName = filepath.Base(abs)
	w.AddHistory(abs)
	return os.Chdir(abs)
}

// GetHistory returns the recent workspace history (most recent first).
func (w *WorkspaceSwitch) GetHistory() []string {
	return w.current.History
}

// AddHistory prepends path to the history, deduplicates, caps the list and
// persists it (persist failures are silent).
func (w *WorkspaceSwitch) AddHistory(path string) {
	if path == "" {
		return
	}
	hist := make([]string, 0, workspaceHistoryMax+1)
	hist = append(hist, path)
	for _, p := range w.current.History {
		if p != path {
			hist = append(hist, p)
		}
	}
	if len(hist) > workspaceHistoryMax {
		hist = hist[:workspaceHistoryMax]
	}
	w.current.History = hist
	w.saveHistory()
}

// RenderStatus renders the workspace indicator for the status bar.
func (w *WorkspaceSwitch) RenderStatus() string {
	if w.current.Path == "" {
		return ""
	}
	return "📁 " + compactMiddle(w.current.Path, 24)
}

// SwitchToPrevious switches back to the previous workspace ("-" semantics).
func (w *WorkspaceSwitch) SwitchToPrevious() error {
	if w.current.Previous == "" {
		return fmt.Errorf("no previous workspace")
	}
	return w.SetCurrentDir(w.current.Previous)
}

// workspaceHistoryPath returns the persistence file path.
func workspaceHistoryPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return workspaceHistoryFile
	}
	return filepath.Join(home, ".inferglow", workspaceHistoryFile)
}

// loadHistory reads the persisted history (silently ignores failures).
func (w *WorkspaceSwitch) loadHistory() {
	w.loadHistoryFrom(w.effectiveHistoryPath())
}

func (w *WorkspaceSwitch) effectiveHistoryPath() string {
	if w.historyPath != "" {
		return w.historyPath
	}
	return workspaceHistoryPath()
}

func (w *WorkspaceSwitch) loadHistoryFrom(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var hist []string
	if err := json.Unmarshal(data, &hist); err != nil {
		return
	}
	w.current.History = hist
}

// saveHistory writes the history (silently ignores failures).
func (w *WorkspaceSwitch) saveHistory() {
	w.saveHistoryTo(w.effectiveHistoryPath())
}

func (w *WorkspaceSwitch) saveHistoryTo(path string) {
	data, err := json.Marshal(w.current.History)
	if err != nil {
		return
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return
		}
	}
	_ = os.WriteFile(path, data, 0o644)
}
