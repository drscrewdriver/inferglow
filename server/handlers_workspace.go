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
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/inferglow/workspace"
)

// handleCreateWorkspace handles POST /v1/workspaces — open a workspace rooted
// at a given directory (spec C-7).
func (s *Server) handleCreateWorkspace(w http.ResponseWriter, r *http.Request) {
	if s.wsProvider == nil {
		writeError(w, http.StatusServiceUnavailable, "workspace provider not configured")
		return
	}
	if !s.canAccess(r, "workspace", "", "create") {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}
	var req struct {
		Name    string `json:"name"`
		RootDir string `json:"root_dir"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.Name == "" || req.RootDir == "" {
		writeError(w, http.StatusBadRequest, "name and root_dir are required")
		return
	}
	info, err := s.wsProvider.Open(req.Name, req.RootDir)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// R8: runtime registrations survive restarts.
	if s.cfg.UsageDataDir != "" {
		s.PersistWorkspaces(filepath.Join(s.cfg.UsageDataDir, "workspaces.json"))
	}
	writeJSON(w, http.StatusCreated, info)
}

// handleListWorkspaces handles GET /v1/workspaces — list all opened workspaces.
func (s *Server) handleListWorkspaces(w http.ResponseWriter, r *http.Request) {
	if s.wsProvider == nil {
		writeError(w, http.StatusServiceUnavailable, "workspace provider not configured")
		return
	}
	if !s.canAccess(r, "workspace", "", "list") {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}
	writeJSON(w, http.StatusOK, s.wsProvider.List())
}

// handleGetWorkspace handles GET /v1/workspaces/{name} — return a single
// workspace's info.
func (s *Server) handleGetWorkspace(w http.ResponseWriter, r *http.Request) {
	if s.wsProvider == nil {
		writeError(w, http.StatusServiceUnavailable, "workspace provider not configured")
		return
	}
	name := r.PathValue("id")
	if !s.canAccess(r, "workspace", name, "read") {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}
	info, ok := s.wsProvider.Get(name)
	if !ok {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// handleDeleteWorkspace handles DELETE /v1/workspaces/{name} — remove a
// workspace binding.
func (s *Server) handleDeleteWorkspace(w http.ResponseWriter, r *http.Request) {
	if s.wsProvider == nil {
		writeError(w, http.StatusServiceUnavailable, "workspace provider not configured")
		return
	}
	name := r.PathValue("id")
	if !s.canAccess(r, "workspace", name, "delete") {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}
	if err := s.wsProvider.Close(name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "closed", "name": name})
}

// handleListWorkspaceFiles handles GET /v1/workspaces/{name}/files — list the
// entries underneath an optional path within the named workspace (spec C-7,
// optional file-listing surface, routed via ListDir).
func (s *Server) handleListWorkspaceFiles(w http.ResponseWriter, r *http.Request) {
	if s.wsProvider == nil {
		writeError(w, http.StatusServiceUnavailable, "workspace provider not configured")
		return
	}
	name := r.PathValue("id")
	if !s.canAccess(r, "workspace", name, "read") {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}
	_, ok := s.wsProvider.Get(name)
	if !ok {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}
	rel := r.URL.Query().Get("path")
	files, err := s.wsProvider.ListDir(name, rel)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"name":  name,
		"path":  rel,
		"files": files,
	})
}

// --- Workspace file management (Spec B) ---
//
// The endpoints below back the sidebar file tree / @file feature. All paths
// are relative to a shared workspace root and confined to it via the
// workspace.Workspace boundary (SafePath rejects traversal).

// workspaceRoot resolves the default workspace root: the first opened
// workspace (C-7 provider, name-ordered) or, when none is open, the process
// working directory.
func (s *Server) workspaceRoot() string {
	root, _ := s.workspaceRootByName("")
	return root
}

// workspaceRootByName resolves the fs root for a named workspace. An empty
// name keeps the default chain (first registered → CWD); a non-empty name
// must match a registered workspace exactly — a typo must fail loudly rather
// than silently showing a different workspace's files.
func (s *Server) workspaceRootByName(name string) (string, error) {
	if s.wsProvider != nil {
		if name != "" {
			if info, ok := s.wsProvider.Get(name); ok {
				return info.Root, nil
			}
			return "", fmt.Errorf("workspace %q not found", name)
		}
		if ws := s.wsProvider.List(); len(ws) > 0 {
			return ws[0].Root, nil
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd, nil
	}
	return "", errors.New("no workspace root configured")
}

// newFileWorkspace returns a workspace.Workspace confined to the server's
// workspace root. Callers own the returned instance (stateless).
func (s *Server) newFileWorkspace() (*workspace.Workspace, error) {
	return s.newFileWorkspaceNamed("")
}

// newFileWorkspaceNamed is newFileWorkspace for an explicitly selected
// workspace (the ?workspace= / body workspace parameter).
func (s *Server) newFileWorkspaceNamed(name string) (*workspace.Workspace, error) {
	root, err := s.workspaceRootByName(name)
	if err != nil {
		return nil, err
	}
	return workspace.New(workspace.Config{RootDir: root})
}

// WorkspaceSeed is one name→root pair for startup seeding.
type WorkspaceSeed struct {
	Name string
	Root string
}

// SeedWorkspaces registers startup workspace pairs into the provider
// (-workspace flags / YAML workspaces section). Entries whose root does not
// exist are logged and skipped — a missing test directory must not take the
// whole server down.
func (s *Server) SeedWorkspaces(seeds []WorkspaceSeed) {
	if s.wsProvider == nil {
		log.Println("workspace seeding skipped: no provider configured")
		return
	}
	// R8: the registry persists (data/workspaces.json). Runtime registrations
	// (UI form, flags) survive restarts, so the sidebar grouping cannot break
	// because a restart used fewer -workspace flags than the last run.
	if s.cfg.UsageDataDir != "" {
		var restored []WorkspaceSeed
		if LoadWorkspaceSnapshot(filepath.Join(s.cfg.UsageDataDir, "workspaces.json"), s.wsProvider) {
			log.Printf("已从快照恢复 workspace 注册表 (data=%s)", s.cfg.UsageDataDir)
		}
		_ = restored
	}
	for _, seed := range seeds {
		if seed.Name == "" || seed.Root == "" {
			continue
		}
		// Open does not validate the root; require an existing directory so a
		// typo'd seed cannot shadow the default chain with a dead workspace.
		if info, err := os.Stat(seed.Root); err != nil || !info.IsDir() {
			log.Printf("workspace %q → %s skipped: root is not an existing directory", seed.Name, seed.Root)
			continue
		}
		info, err := s.wsProvider.Open(seed.Name, seed.Root)
		if err != nil {
			log.Printf("workspace %q → %s skipped: %v", seed.Name, seed.Root, err)
			continue
		}
		log.Printf("workspace %q → %s", info.Name, info.Root)
	}
	if s.cfg.UsageDataDir != "" {
		s.PersistWorkspaces(filepath.Join(s.cfg.UsageDataDir, "workspaces.json"))
	}
}

// PersistWorkspaces writes the current registry to path (called after every
// successful seed/registration so restarts restore the exact set).
func (s *Server) PersistWorkspaces(path string) {
	if s.wsProvider == nil {
		return
	}
	type wsSnapshot struct {
		Workspaces []WorkspaceSeed `json:"workspaces"`
	}
	snap := wsSnapshot{Workspaces: []WorkspaceSeed{}}
	for _, info := range s.wsProvider.List() {
		snap.Workspaces = append(snap.Workspaces, WorkspaceSeed{Name: info.Name, Root: info.Root})
	}
	data, err := json.MarshalIndent(snap, "", " ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		log.Printf("workspace snapshot write: %v", err)
		return
	}
	_ = os.Rename(tmp, path)
}

// LoadWorkspaceSnapshot restores a workspace registry snapshot. Missing or
// corrupt file = false (no-op).
func LoadWorkspaceSnapshot(path string, provider WorkspaceProvider) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	type wsSnapshot struct {
		Workspaces []WorkspaceSeed `json:"workspaces"`
	}
	var snap wsSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		log.Printf("workspace snapshot %s corrupt (%v) — ignored", path, err)
		return false
	}
	for _, seed := range snap.Workspaces {
		if seed.Name == "" || seed.Root == "" {
			continue
		}
		if info, err := os.Stat(seed.Root); err != nil || !info.IsDir() {
			log.Printf("workspace %q → %s skipped on restore: root missing", seed.Name, seed.Root)
			continue
		}
		if _, err := provider.Open(seed.Name, seed.Root); err != nil {
			log.Printf("workspace %q restore failed: %v", seed.Name, err)
		}
	}
	return true
}

// fsEntry is one row of a directory listing.
type fsEntry struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	IsDir  bool   `json:"is_dir"`
	Hidden bool   `json:"hidden"`
}

// handleWorkspaceTree handles GET /v1/workspace/tree (alias /v1/fs/tree) —
// single-level directory listing inside the workspace root.
func (s *Server) handleWorkspaceTree(w http.ResponseWriter, r *http.Request) {
	ws, err := s.newFileWorkspaceNamed(r.URL.Query().Get("workspace"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	rel := r.URL.Query().Get("path")
	names, err := ws.ListDir(rel)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	entries := make([]fsEntry, 0, len(names))
	for _, name := range names {
		info, err := ws.Stat(filepath.Join(rel, name))
		if err != nil {
			continue
		}
		entries = append(entries, fsEntry{
			Name:   name,
			Path:   filepath.ToSlash(filepath.Join(rel, name)),
			IsDir:  info.IsDir(),
			Hidden: strings.HasPrefix(name, "."),
		})
	}
	// Directory-first, name (case-insensitive) ordering — VSCode order.
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"root":      ws.Root(),
		"path":      rel,
		"entries":   entries,
		"truncated": false,
	})
}

// handleWorkspaceRead handles GET /v1/workspace/read (alias /v1/fs/read) —
// read a file's text content within the workspace root.
func (s *Server) handleWorkspaceRead(w http.ResponseWriter, r *http.Request) {
	ws, err := s.newFileWorkspaceNamed(r.URL.Query().Get("workspace"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	rel := r.URL.Query().Get("path")
	if rel == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	data, err := ws.ReadFile(rel)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"root":    ws.Root(),
		"path":    filepath.ToSlash(rel),
		"content": string(data),
		"bytes":   len(data),
	})
}

// handleWorkspaceWrite handles POST /v1/workspace/write (alias /v1/fs/write)
// — create/overwrite a file within the workspace root.
func (s *Server) handleWorkspaceWrite(w http.ResponseWriter, r *http.Request) {
	ws, err := s.newFileWorkspaceNamed(r.URL.Query().Get("workspace"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	if err := ws.WriteFile(req.Path, []byte(req.Content)); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"path":   filepath.ToSlash(req.Path),
		"bytes":  len(req.Content),
		"status": "written",
	})
}

// handleWorkspaceRename handles POST /v1/workspace/rename (alias
// /v1/fs/rename) — rename or move a file/dir within the workspace root.
func (s *Server) handleWorkspaceRename(w http.ResponseWriter, r *http.Request) {
	ws, err := s.newFileWorkspaceNamed(r.URL.Query().Get("workspace"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.From == "" || req.To == "" {
		writeError(w, http.StatusBadRequest, "from and to are required")
		return
	}
	fromAbs, err := ws.SafePath(req.From)
	if err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	toAbs, err := ws.SafePath(req.To)
	if err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	if err := os.MkdirAll(filepath.Dir(toAbs), 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := os.Rename(fromAbs, toAbs); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"from":   filepath.ToSlash(req.From),
		"to":     filepath.ToSlash(req.To),
		"status": "renamed",
	})
}

// handleWorkspaceDelete handles POST /v1/workspace/delete (alias /v1/fs/delete)
// — remove a file or directory tree within the workspace root.
func (s *Server) handleWorkspaceDelete(w http.ResponseWriter, r *http.Request) {
	ws, err := s.newFileWorkspaceNamed(r.URL.Query().Get("workspace"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req struct {
		Path string `json:"path"`
	}
	path := r.URL.Query().Get("path")
	if r.Body != nil && path == "" {
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Path != "" {
			path = req.Path
		}
	}
	if path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	if err := ws.RemoveAll(path); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"path":   filepath.ToSlash(path),
		"status": "deleted",
	})
}

// fsSearchSkipDirs are directory names never matched nor descended during a
// filename search (aligned with DSH fs-search.ts).
var fsSearchSkipDirs = map[string]bool{
	".git": true, "node_modules": true, ".pnpm-store": true, ".yarn": true,
	".turbo": true, ".turbopack": true, ".next": true, ".nuxt": true,
	".output": true, ".cache": true, ".parcel-cache": true, "coverage": true,
	"dist": true, "build": true, "out": true, ".umi": true, ".umi-production": true, ".dumi": true,
}

// handleWorkspaceSearch handles GET /v1/workspace/search (alias /v1/fs/search)
// — recursive case-insensitive filename substring search within the root.
func (s *Server) handleWorkspaceSearch(w http.ResponseWriter, r *http.Request) {
	ws, err := s.newFileWorkspaceNamed(r.URL.Query().Get("workspace"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	q := r.URL.Query().Get("q")
	opt := r.URL.Query().Get("limit")
	maxMatches := 200
	if n, err := strconv.Atoi(opt); err == nil && n > 0 {
		maxMatches = n
	}
	maxVisited := 100_000
	needle := strings.ToLower(strings.TrimSpace(q))
	matches := []string{}
	truncated := false
	visited := 0
	var walk func(dir string) error
	walk = func(dir string) error {
		if truncated {
			return nil
		}
		names, err := ws.ListDir(dir)
		if err != nil {
			return nil // unreadable levels are skipped
		}
		for _, name := range names {
			visited++
			if visited > maxVisited {
				truncated = true
				return nil
			}
			rel := name
			if dir != "" {
				rel = filepath.Join(dir, name)
			}
			info, err := ws.Stat(rel)
			if err != nil {
				continue
			}
			isDir := info.IsDir()
			if isDir && fsSearchSkipDirs[strings.ToLower(name)] {
				continue
			}
			if strings.Contains(strings.ToLower(name), needle) {
				matches = append(matches, filepath.ToSlash(rel))
				if len(matches) >= maxMatches {
					truncated = true
					return nil
				}
			}
			if isDir {
				if err := walk(rel); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if needle == "" {
		matches = []string{}
	} else {
		_ = walk("")
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"query":     q,
		"matches":   matches,
		"truncated": truncated,
	})
}

// handleWorkspaceUpload handles POST /v1/workspace/upload (alias /v1/fs/upload)
// — write an uploaded file into the workspace root. Accepts either a raw body
// with ?path=, or multipart/form-data with "path" (fallback ?path=) and
// "file" fields.
func (s *Server) handleWorkspaceUpload(w http.ResponseWriter, r *http.Request) {
	ws, err := s.newFileWorkspaceNamed(r.URL.Query().Get("workspace"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	target := r.URL.Query().Get("path")
	data := []byte{}

	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/form-data") {
		// Multipart: filename/path come from fields.
		if err := r.ParseMultipartForm(10 << 20); err != nil { // 10 MiB
			writeError(w, http.StatusBadRequest, "invalid multipart body: "+err.Error())
			return
		}
		if p := r.FormValue("path"); p != "" {
			target = p
		}
		var file multipart.File
		if fhs, ok := r.MultipartForm.File["file"]; ok && len(fhs) > 0 {
			file, err = fhs[0].Open()
			if err != nil {
				writeError(w, http.StatusBadRequest, "cannot open upload: "+err.Error())
				return
			}
			defer file.Close()
			data, err = io.ReadAll(file)
			if err != nil {
				writeError(w, http.StatusBadRequest, "cannot read upload: "+err.Error())
				return
			}
		}
	} else {
		data, err = io.ReadAll(io.LimitReader(r.Body, 10<<20+1)) // 10 MiB cap
		if err != nil {
			writeError(w, http.StatusBadRequest, "cannot read body: "+err.Error())
			return
		}
	}
	if target == "" {
		writeError(w, http.StatusBadRequest, "path is required (query or form field)")
		return
	}
	if err := ws.WriteFile(target, data); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"path": filepath.ToSlash(target),
		"size": len(data),
	})
}
