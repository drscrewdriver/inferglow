// Copyright 2026 InferGlow Authors
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software are
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

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Terminal v1 (POST /v1/exec): one-command-one-result execution, NOT an
// interactive shell — the sandbox Execute contract has no PTY. Defense in
// depth (all gates required):
//  1. route registered only when cfg.APIKey != "" (router.go) — no key, no route;
//  2. cfg.ExecEnabled switch (server -exec), default off;
//  3. APIKeyAuth middleware already wraps the whole /v1 mux;
//  4. argv[0] must be a bare command on the allowlist (no shell, no paths);
//  5. workdir resolves through workspace.SafePath (confined to the workspace
//     root, no traversal);
//  6. timeout 10s default / 60s hard cap;
//  7. every attempt (allowed or not) lands in exec-audit.jsonl.
const (
	execDefaultTimeout = 10 * time.Second
	execMaxTimeout     = 60 * time.Second
	execOutputLimit    = 256 * 1024
)

// execAllowlist is the v1 command set. Bare names only; extend deliberately.
var execAllowlist = map[string]bool{
	"git": true,
	"ls":  true,
	"dir": true,
	"pwd": true,
	"go":  true,
}

type execRequest struct {
	Argv      []string `json:"argv"`
	// Workspace selects which registered workspace provides the root for
	// Workdir (empty = first registered, matching the fs endpoints).
	Workspace string `json:"workspace,omitempty"`
	Workdir   string `json:"workdir,omitempty"`
	TimeoutMs int      `json:"timeout_ms,omitempty"`
}

type execResult struct {
	ExitCode   int    `json:"exit_code"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	DurationMs int64  `json:"duration_ms"`
	Truncated  bool   `json:"truncated"`
}

func (s *Server) handleExec(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	var req execRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if len(req.Argv) == 0 || strings.TrimSpace(req.Argv[0]) == "" {
		s.auditExec(req, -1, "empty argv")
		writeError(w, http.StatusBadRequest, "argv is required")
		return
	}
	cmdName := req.Argv[0]
	if strings.ContainsAny(cmdName, `/\`) || strings.Contains(cmdName, "..") {
		s.auditExec(req, -1, "argv[0] must be a bare command name")
		writeError(w, http.StatusForbidden, "argv[0] must be a bare command name (no paths)")
		return
	}
	if !execAllowlist[strings.ToLower(cmdName)] {
		s.auditExec(req, -1, "command not in allowlist")
		writeError(w, http.StatusForbidden, fmt.Sprintf("command %q is not in the allowlist %v", cmdName, allowlistNames()))
		return
	}

	timeout := execDefaultTimeout
	if req.TimeoutMs > 0 {
		timeout = time.Duration(req.TimeoutMs) * time.Millisecond
		if timeout > execMaxTimeout {
			timeout = execMaxTimeout
		}
	}

	// Resolve workdir inside the selected workspace root (SafePath rejects
	// traversal).
	ws, err := s.newFileWorkspaceNamed(req.Workspace)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	workdir, err := ws.SafePath(req.Workdir)
	if err != nil {
		s.auditExec(req, -1, "workdir rejected: "+err.Error())
		writeError(w, http.StatusForbidden, "workdir rejected: "+err.Error())
		return
	}
	if info, statErr := os.Stat(workdir); statErr != nil || !info.IsDir() {
		s.auditExec(req, -1, "workdir is not a directory")
		writeError(w, http.StatusBadRequest, "workdir is not a directory")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, req.Argv[0], req.Argv[1:]...)
	cmd.Dir = workdir

	var stdout, stderr strings.Builder
	cmd.Stdout = &limitedWriter{b: &stdout, limit: execOutputLimit}
	cmd.Stderr = &limitedWriter{b: &stderr, limit: execOutputLimit}

	runErr := cmd.Run()
	res := execResult{
		Stdout:     stdout.String(),
		Stderr:     stderr.String(),
		DurationMs: time.Since(start).Milliseconds(),
		Truncated:  stdout.Len() >= execOutputLimit || stderr.Len() >= execOutputLimit,
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		res.ExitCode = exitErr.ExitCode()
	} else if runErr != nil {
		res.ExitCode = -1
		res.Stderr = strings.TrimSpace(res.Stderr + "\n" + runErr.Error())
	}
	s.auditExec(req, res.ExitCode, fmt.Sprintf("duration=%s", time.Since(start).Round(time.Millisecond)))
	writeJSON(w, http.StatusOK, res)
}

// auditExec appends one JSONL line to {UsageDataDir}/exec-audit.jsonl.
func (s *Server) auditExec(req execRequest, exitCode int, note string) {
	if s.cfg.UsageDataDir == "" {
		return
	}
	if err := os.MkdirAll(s.cfg.UsageDataDir, 0o755); err != nil {
		return
	}
	entry := map[string]any{
		"ts":        time.Now().UTC().Format(time.RFC3339),
		"argv":      req.Argv,
		"workspace": req.Workspace,
		"workdir":   req.Workdir,
		"exit_code": exitCode,
		"note":      note,
	}
	line, err := json.Marshal(entry)
	if err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(s.cfg.UsageDataDir, "exec-audit.jsonl"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(line, '\n'))
}

func allowlistNames() []string {
	names := make([]string, 0, len(execAllowlist))
	for n := range execAllowlist {
		names = append(names, n)
	}
	return names
}

// limitedWriter caps how much a pipe may buffer in memory.
type limitedWriter struct {
	b     *strings.Builder
	limit int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if w.b.Len() >= w.limit {
		return len(p), nil // swallow the rest, Truncated flag reports it
	}
	room := w.limit - w.b.Len()
	if len(p) > room {
		w.b.Write(p[:room])
		return len(p), nil
	}
	w.b.Write(p)
	return len(p), nil
}
