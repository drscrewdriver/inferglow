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
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// tuiHandleSession handles /session and /resume commands.
func (m *chatTUI) tuiHandleSession(args string) {
	m.commitLine("")
	m.commitLine(accent("Session ID: ") + footerInfo(m.sessionID))
}

// tuiHandleResume handles /resume [id] command.
// Empty args: lists recent sessions. With an id: validates the session file
// exists, arms m.restartResumeID, and returns true so the caller can quit the
// current TUI program and let RunTUI relaunch against that session.
func (m *chatTUI) tuiHandleResume(args string) bool {
	m.commitLine("")
	if strings.TrimSpace(args) == "" {
		// List recent sessions by scanning session files.
		sessions := m.listRecentSessions(10)
		if len(sessions) == 0 {
			m.commitLine(dim("No previous sessions found."))
			return false
		}
		m.commitLine(accent("Recent sessions:"))
		for _, s := range sessions {
			m.commitLine(dim("  " + s))
		}
		m.commitLine(dim("Usage: /resume <session-id>"))
		return false
	}
	id := strings.TrimSpace(args)
	if !sessionFileExists(m.cfg, id) {
		m.commitLine(errorText(fmt.Sprintf("Session %q not found.", id)))
		return false
	}
	m.restartResumeID = id
	m.commitLine(successText(fmt.Sprintf("Resuming session %s …", id)))
	return true
}

// sessionFileExists reports whether a persisted session JSONL exists for id.
func sessionFileExists(cfg CLIConfig, id string) bool {
	dataDir := cfg.DataDir
	if dataDir == "" {
		dataDir = defaultDataDir()
	}
	if id == "" || strings.ContainsAny(id, `/\`) {
		return false
	}
	_, err := os.Stat(filepath.Join(dataDir, "sessions", id+".jsonl"))
	return err == nil
}

// listRecentSessions scans the data directory for session JSONL files.
func (m *chatTUI) listRecentSessions(limit int) []string {
	dataDir := m.cfg.DataDir
	if dataDir == "" {
		dataDir = defaultDataDir()
	}
	sessionsDir := filepath.Join(dataDir, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return nil
	}
	var ids []string
	// Walk in reverse to get most recent first.
	for i := len(entries) - 1; i >= 0 && len(ids) < limit; i-- {
		name := entries[i].Name()
		if strings.HasSuffix(name, ".jsonl") {
			ids = append(ids, strings.TrimSuffix(name, ".jsonl"))
		}
	}
	return ids
}

// defaultDataDir returns the default data directory path.
func defaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".inferglow"
	}
	return filepath.Join(home, ".inferglow")
}
