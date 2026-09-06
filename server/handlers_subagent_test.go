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
	"net/http/httptest"
	"testing"

	"github.com/inferglow/builtins/actions"
)

func TestHandleListSubagents_RegistryWiring(t *testing.T) {
	// Nil registry → 503 (the webui shows the panel's error state).
	srv := &Server{}
	rec := httptest.NewRecorder()
	srv.handleListSubagents(rec, httptest.NewRequest("GET", "/v1/subagents", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("nil registry: status = %d, want 503", rec.Code)
	}

	// Wired registry: records listed, newest first, filterable by session.
	reg := actions.NewSubagentRegistry()
	SetSubagentRegistry(reg)
	defer SetSubagentRegistry(nil)
	a := reg.Start("sess-1", "task one", "")
	b := reg.Start("sess-2", "task two", "")
	reg.Finish(b.ID, true, "ok", "")

	rec = httptest.NewRecorder()
	srv.handleListSubagents(rec, httptest.NewRequest("GET", "/v1/subagents", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("wired: status = %d, want 200", rec.Code)
	}
	var body struct {
		Spawns []actions.SpawnRecord `json:"spawns"`
		Count  int                   `json:"count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Count != 2 || len(body.Spawns) != 2 {
		t.Fatalf("count = %d, want 2 (body %s)", body.Count, rec.Body.String())
	}
	if body.Spawns[0].ID != b.ID {
		t.Errorf("newest-first violated: first = %s, want %s", body.Spawns[0].ID, b.ID)
	}

	rec = httptest.NewRecorder()
	srv.handleListSubagents(rec, httptest.NewRequest("GET", "/v1/subagents?session=sess-1", nil))
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode filtered: %v", err)
	}
	if body.Count != 1 || body.Spawns[0].ParentSession != "sess-1" || body.Spawns[0].Status != actions.SpawnStatusRunning {
		t.Fatalf("filtered = %+v, want the single running sess-1 row", body.Spawns)
	}
	_ = a
}
