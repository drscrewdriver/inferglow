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
	"embed"
	"io/fs"
	"net/http"
)

// WebUI DSH build output (vite build from webui-dsh/, the vendored
// dsh-transition-webui layout wired to the InferGlow API) is embedded here.
// Mounted at /webui-dsh/ alongside the browser Web UI (/web/, from webui/)
// and the prototype (/webui2/); same-origin so /v1 API calls need no CORS.
//
//go:embed webui-dsh
var webUIDshFS embed.FS

// handleWebUIDsh serves the embedded WebUI DSH shell, mounted at /webui-dsh/.
// Responses are marked no-cache: the shell is an index.html pointing at
// content-hashed assets, and heuristic caching of a stale shell makes the
// browser load bundles that no longer exist on disk.
func (s *Server) handleWebUIDsh(w http.ResponseWriter, r *http.Request) {
	sub, err := fs.Sub(webUIDshFS, "webui-dsh")
	if err != nil {
		http.Error(w, "webui-dsh assets unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.StripPrefix("/webui-dsh/", http.FileServer(http.FS(sub))).ServeHTTP(w, r)
}
