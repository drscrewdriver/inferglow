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
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// producedFile is one output file surfaced to the sidebar's produced-files card.
type producedFile struct {
	Path     string `json:"path"`
	Bytes    int64  `json:"bytes"`
	Modified string `json:"modified"`
}

// handleProducedFiles handles GET /v1/produced-files?run_id=&path=&limit= —
// list the produced/modified files inside the workspace root, newest first.
// run_id is echoed back for correlation when provided.
func (s *Server) handleProducedFiles(w http.ResponseWriter, r *http.Request) {
	ws, err := s.newFileWorkspace()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	limit := 200
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, aerr := strconv.Atoi(v); aerr == nil && n > 0 {
			limit = n
		}
	}
	base := ws.Root()
	sub := r.URL.Query().Get("path")
	if sub == "" {
		sub = "."
	}
	subAbs, err := ws.SafePath(sub)
	if err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	files := []producedFile{}
	_ = filepath.WalkDir(subAbs, func(p string, d fs.DirEntry, werr error) error {
		if werr != nil || d == nil {
			return nil
		}
		if d.IsDir() {
			// Skip only SCM/dependency metadata, never descent into them.
			// Build output dirs (out/dist/build) must NOT be skipped here:
			// they are exactly where produced artifacts live.
			if name := strings.ToLower(d.Name()); name == ".git" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if info, serr := os.Stat(p); serr == nil {
			rel, rerr := filepath.Rel(base, p)
			if rerr != nil {
				return nil
			}
			files = append(files, producedFile{
				Path:     filepath.ToSlash(rel),
				Bytes:    info.Size(),
				Modified: info.ModTime().UTC().Format("2006-01-02T15:04:05Z07:00"),
			})
		}
		return nil
	})
	// Newest modified first.
	sort.SliceStable(files, func(i, j int) bool { return files[i].Modified > files[j].Modified })
	if len(files) > limit {
		files = files[:limit]
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"run_id": r.URL.Query().Get("run_id"),
		"files":  files,
		"count":  len(files),
	})
}