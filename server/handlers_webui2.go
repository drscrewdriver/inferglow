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

// WebUI2 build output (vite build from webui2/) is embedded here. It is the
// layout rebuilt against the inferglow-prototype reference, kept separate from
// the original Desktop GUI (/gui/, from web/) and the browser Web UI (/web/,
// from webui/).
//
//go:embed webui2
var webUI2FS embed.FS

// handleWebUI2 serves the embedded WebUI2 shell, mounted at /webui2/.
func (s *Server) handleWebUI2(w http.ResponseWriter, r *http.Request) {
	sub, err := fs.Sub(webUI2FS, "webui2")
	if err != nil {
		http.Error(w, "webui2 assets unavailable", http.StatusInternalServerError)
		return
	}
	http.StripPrefix("/webui2/", http.FileServer(http.FS(sub))).ServeHTTP(w, r)
}
