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
	"embed"
	"io/fs"
	"net/http"
)

// The React GUI build output (vite build from web/) is embedded here.
// The build artifacts are checked into the repo so `go build` works without
// a Node toolchain, mirroring the existing dashboard.html pattern.
//
//go:embed webui
var guiFS embed.FS

// handleGUI serves the embedded React GUI shell. It is registered on the root
// mux outside the API middleware chain (same tier as /dashboard). The /gui/
// prefix is stripped so the embedded FS resolves from its root.
func (s *Server) handleGUI(w http.ResponseWriter, r *http.Request) {
	sub, err := fs.Sub(guiFS, "webui")
	if err != nil {
		http.Error(w, "gui assets unavailable", http.StatusInternalServerError)
		return
	}
	http.StripPrefix("/gui/", http.FileServer(http.FS(sub))).ServeHTTP(w, r)
}
