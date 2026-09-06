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

package server

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
)

// runGit executes the system git binary with -C <root> and pinned flavour
// flags, returning combined behaviour: the machine-readable stdout and any
// error (including the leading stderr line).
func (s *Server) runGit(root string, args ...string) (string, error) {
	full := append([]string{"-C", root, "--no-pager", "-c", "color.ui=false"}, args...)
	cmd := exec.Command("git", full...)
	cmd.Env = append(cmd.Environ(), "GIT_OPTIONAL_LOCKS=0")
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = "git exited with " + err.Error()
		}
		return stdout.String(), errGit{msg: msg}
	}
	return stdout.String(), nil
}

// errGit carries a user-presentable git failure message.
type errGit struct{ msg string }

func (e errGit) Error() string { return e.msg }

// handleGitStatus handles GET /v1/git/status — working-tree status snapshot.
func (s *Server) handleGitStatus(w http.ResponseWriter, r *http.Request) {
	root, err := s.workspaceRootByName(r.URL.Query().Get("workspace"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if root == "" {
		writeError(w, http.StatusServiceUnavailable, "no workspace root configured")
		return
	}
	isRepo, err := s.runGit(root, "rev-parse", "--is-inside-work-tree")
	if err != nil || strings.TrimSpace(isRepo) != "true" {
		writeJSON(w, http.StatusOK, map[string]any{"is_repo": false, "entries": []any{}, "root": root})
		return
	}
	branch, _ := s.runGit(root, "rev-parse", "--abbrev-ref", "HEAD")
	raw, err := s.runGit(root, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	entries := parsePorcelain(raw)
	writeJSON(w, http.StatusOK, map[string]any{
		"is_repo":  true,
		"branch":   strings.TrimSpace(branch),
		"entries":  entries,
		"truncated": false,
		"root":     root,
	})
}

// parsePorcelain parses `git status --porcelain=v1` (newline framed) into
// {path, xy} rows.
func parsePorcelain(out string) []map[string]string {
	var rows []map[string]string
	for _, line := range strings.Split(out, "\n") {
		if len(line) < 3 {
			continue
		}
		xy := line[:2]
		path := strings.TrimSpace(line[3:])
		rows = append(rows, map[string]string{"path": path, "xy": xy})
	}
	return rows
}

// handleGitDiff handles GET /v1/git/diff?path=&staged= — unified diff text.
func (s *Server) handleGitDiff(w http.ResponseWriter, r *http.Request) {
	root, err := s.workspaceRootByName(r.URL.Query().Get("workspace"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if root == "" {
		writeError(w, http.StatusServiceUnavailable, "no workspace root configured")
		return
	}
	args := []string{"diff", "--no-ext-diff", "--no-color", "-U3"}
	if r.URL.Query().Get("staged") == "true" {
		args = append(args, "--cached")
	}
	if p := r.URL.Query().Get("path"); p != "" {
		args = append(args, "--", p)
	}
	out, err := s.runGit(root, args...)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"diff": out})
}

// handleGitLog handles GET /v1/git/log?count=&skip= — commit history rows.
func (s *Server) handleGitLog(w http.ResponseWriter, r *http.Request) {
	root, err := s.workspaceRootByName(r.URL.Query().Get("workspace"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if root == "" {
		writeError(w, http.StatusServiceUnavailable, "no workspace root configured")
		return
	}
	count := 30
	if v := r.URL.Query().Get("count"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			count = n
		}
	}
	skip := 0
	if v := r.URL.Query().Get("skip"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			skip = n
		}
	}
	out, err := s.runGit(root,
		"log", "-n", strconv.Itoa(count), "--skip", strconv.Itoa(skip), "--decorate=short",
		"--pretty=format:%h%x1f%s%x1f%an%x1f%ai%x1f%H%x1f%D")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var rows []map[string]string
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\x1f")
		if len(parts) < 2 {
			continue
		}
		row := map[string]string{
			"hash": parts[0], "subject": parts[1],
		}
		if len(parts) > 2 {
			row["author"] = parts[2]
		}
		if len(parts) > 3 {
			row["date"] = parts[3]
		}
		if len(parts) > 4 {
			row["hash_full"] = parts[4]
		}
		if len(parts) > 5 {
			row["refs"] = parts[5]
		}
		rows = append(rows, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{"log": rows})
}

// handleGitBranches handles GET /v1/git/branches — branch names, current first.
func (s *Server) handleGitBranches(w http.ResponseWriter, r *http.Request) {
	root, err := s.workspaceRootByName(r.URL.Query().Get("workspace"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if root == "" {
		writeError(w, http.StatusServiceUnavailable, "no workspace root configured")
		return
	}
	current, _ := s.runGit(root, "rev-parse", "--abbrev-ref", "HEAD")
	raw, err := s.runGit(root, "for-each-ref", "--format=%(refname:short)", "refs/heads")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	names := []string{}
	for _, line := range strings.Split(raw, "\n") {
		if line != "" {
			names = append(names, line)
		}
	}
	cur := strings.TrimSpace(current)
	if cur != "" && !stringContains(names, cur) {
		names = append([]string{cur}, names...)
	}
	writeJSON(w, http.StatusOK, map[string]any{"current": cur, "names": names})
}

func stringContains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// handleGitWorktrees handles GET /v1/git/worktrees — linked checkouts.
func (s *Server) handleGitWorktrees(w http.ResponseWriter, r *http.Request) {
	root, err := s.workspaceRootByName(r.URL.Query().Get("workspace"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if root == "" {
		writeError(w, http.StatusServiceUnavailable, "no workspace root configured")
		return
	}
	raw, err := s.runGit(root, "worktree", "list", "--porcelain")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	rows := []map[string]any{}
	var cur map[string]any
	for _, line := range strings.Split(raw, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			if cur != nil {
				rows = append(rows, cur)
			}
			cur = map[string]any{
				"path":   strings.TrimPrefix(line, "worktree "),
				"branch": "HEAD",
				"current": false,
			}
		case strings.HasPrefix(line, "branch refs/heads/"):
			if cur != nil {
				cur["branch"] = strings.TrimPrefix(line, "branch refs/heads/")
			}
		case strings.HasPrefix(line, "locked"):
			if cur != nil {
				cur["locked"] = true
			}
		}
	}
	if cur != nil {
		rows = append(rows, cur)
	}
	writeJSON(w, http.StatusOK, map[string]any{"worktrees": rows})
}

// handleGitCommit handles POST /v1/git/commit — commit staged changes.
func (s *Server) handleGitCommit(w http.ResponseWriter, r *http.Request) {
	root, err := s.workspaceRootByName(r.URL.Query().Get("workspace"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if root == "" {
		writeError(w, http.StatusServiceUnavailable, "no workspace root configured")
		return
	}
	var req struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.Message == "" {
		writeError(w, http.StatusBadRequest, "message is required")
		return
	}
	if _, err := s.runGit(root, "commit", "-m", req.Message); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "committed"})
}

// handleGitStage handles POST /v1/git/stage?path= — stage the worktree (or one path).
func (s *Server) handleGitStage(w http.ResponseWriter, r *http.Request) {
	root, err := s.workspaceRootByName(r.URL.Query().Get("workspace"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if root == "" {
		writeError(w, http.StatusServiceUnavailable, "no workspace root configured")
		return
	}
	args := []string{"add", "-A"}
	if p := r.URL.Query().Get("path"); p != "" {
		args = append(args, "--", p)
	}
	if _, err := s.runGit(root, args...); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "staged"})
}

// handleGitReset handles POST /v1/git/reset?path= — unstage the index (or one path).
func (s *Server) handleGitReset(w http.ResponseWriter, r *http.Request) {
	root, err := s.workspaceRootByName(r.URL.Query().Get("workspace"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if root == "" {
		writeError(w, http.StatusServiceUnavailable, "no workspace root configured")
		return
	}
	args := []string{"reset", "-q"}
	if p := r.URL.Query().Get("path"); p != "" {
		args = append(args, "--", p)
	}
	if _, err := s.runGit(root, args...); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "reset"})
}

// handleGitCheckout handles POST /v1/git/checkout — switch to a branch.
func (s *Server) handleGitCheckout(w http.ResponseWriter, r *http.Request) {
	root, err := s.workspaceRootByName(r.URL.Query().Get("workspace"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if root == "" {
		writeError(w, http.StatusServiceUnavailable, "no workspace root configured")
		return
	}
	var req struct {
		Branch string `json:"branch"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.Branch == "" {
		writeError(w, http.StatusBadRequest, "branch is required")
		return
	}
	if _, err := s.runGit(root, "checkout", req.Branch); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "checked_out", "branch": req.Branch})
}