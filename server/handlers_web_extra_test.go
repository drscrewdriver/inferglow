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
	"strings"
	"testing"

	"github.com/inferglow/approval"
	"github.com/inferglow/flow/stage"
)

// --- Phase 1, Task 3: session list filtering / grouping ---

func newSessionServer(t *testing.T) *Server {
	t.Helper()
	srv := NewServer(DefaultConfig(), newMockStore())
	srv.SetSessionStore(NewSessionStore())
	return srv
}

func TestHandleListSessions_FilterGroupPinnedQ(t *testing.T) {
	srv := newSessionServer(t)

	// Two sessions in group "工作", one pinned; one in group "研究".
	create := func(body string) {
		t.Helper()
		req := httptest.NewRequest("POST", "/v1/sessions", strings.NewReader(body))
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("create %s -> %d", body, w.Code)
		}
	}
	create(`{"agent_id":"a1","title":"GUI alpha"}`)
	create(`{"agent_id":"a1","title":"GUI beta"}`)
	create(`{"agent_id":"a2","title":"research delta"}`)

	// Mark the first as group 工作 + pinned, second as group 工作.
	srv.Handler().ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequest("PATCH", "/v1/sessions/sess-1", strings.NewReader(`{"group":"工作","pinned":true}`)))
	srv.Handler().ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequest("PATCH", "/v1/sessions/sess-2", strings.NewReader(`{"group":"工作"}`)))

	// group filter
	var out struct {
		Sessions []*SessionRecord `json:"sessions"`
		Groups   map[string][]*SessionRecord `json:"groups"`
		Count    int `json:"count"`
	}
	do := func(path string, dst any) {
		t.Helper()
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/v1/sessions"+path, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s -> %d (%s)", path, w.Code, w.Body.String())
		}
		if err := json.Unmarshal(w.Body.Bytes(), dst); err != nil {
			t.Fatal(err)
		}
	}

	do("", &out)
	if out.Count != 3 {
		t.Fatalf("all: count = %d, want 3", out.Count)
	}

	// group filter (URL-encoded 工作).
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/v1/sessions?group=%E5%B7%A5%E4%BD%9C", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("group -> %d", w.Code)
	}
	var grp struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &grp); err != nil {
		t.Fatal(err)
	}
	if grp.Count != 2 {
		t.Fatalf("group=工作: count = %d, want 2", grp.Count)
	}

	do("?pinned=true", &out)
	if out.Count != 1 || !out.Sessions[0].Pinned {
		t.Fatalf("pinned: count=%d first=%+v, want 1 pinned first", out.Count, out.Sessions[0])
	}

	do("?q=research", &out)
	if out.Count != 1 || out.Sessions[0].Title != "research delta" {
		t.Fatalf("q: count=%d sessions=%+v", out.Count, out.Sessions)
	}
}

// --- Phase 1, Task 1: run queue management ---

func newRunServer(t *testing.T) *Server {
	t.Helper()
	stages := stage.NewRegistry()
	flowStore := NewFlowStore(stages)
	if err := flowStore.Register(validFlowDef("f1")); err != nil {
		t.Fatal(err)
	}
	srv := NewServerWithFlows(DefaultConfig(), newMockStore(), flowStore)
	return srv
}

func TestRunManager_QueueCRUD(t *testing.T) {
	// Start a run via the HTTP server (flow "f1" pre-registered).
	srv := newRunServer(t)
	handle, err := srv.runMgr.Start("f1", nil, "u")
	if err != nil {
		t.Fatal(err)
	}
	id := handle.ID

	it1, err := srv.runMgr.QueuePush(id, RunQueueTierLater, "msg A", "")
	if err != nil {
		t.Fatal(err)
	}
	it2, err := srv.runMgr.QueuePush(id, RunQueueTierNext, "msg B", "")
	if err != nil {
		t.Fatal(err)
	}
	// now tier should land at the front.
	_, _ = srv.runMgr.QueuePush(id, RunQueueTierNow, "urgent", "")

	q, _ := srv.runMgr.QueueList(id)
	if len(q) != 3 {
		t.Fatalf("len = %d, want 3", len(q))
	}
	if q[0].Text != "urgent" {
		t.Fatalf("front = %q, want urgent (now tier first)", q[0].Text)
	}
	if q[0].Tier != RunQueueTierNow {
		t.Fatalf("front tier = %q, want now", q[0].Tier)
	}

	// edit it2 (the next-tier item at index 1).
	edited, err := srv.runMgr.QueueEdit(id, it2.ID, RunQueueTierNow, "msg B2")
	if err != nil {
		t.Fatal(err)
	}
	if edited.Text != "msg B2" || edited.Tier != RunQueueTierNow {
		t.Fatalf("edited = %+v", edited)
	}

	// steer it2 to front.
	steered, err := srv.runMgr.QueueSteer(id, it2.ID, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if steered.ID != it2.ID {
		t.Fatal("steer returned wrong item")
	}

	// remove it1.
	if err := srv.runMgr.QueueRemove(id, it1.ID); err != nil {
		t.Fatal(err)
	}
	q, _ = srv.runMgr.QueueList(id)
	if len(q) != 2 {
		t.Fatalf("after remove len = %d, want 2", len(q))
	}

	// clear.
	if err := srv.runMgr.QueueClear(id); err != nil {
		t.Fatal(err)
	}
	q, _ = srv.runMgr.QueueList(id)
	if len(q) != 0 {
		t.Fatalf("after clear len = %d, want 0", len(q))
	}
}

func TestRunQueue_HTTPPatch(t *testing.T) {
	srv := newRunServer(t)
	handle, err := srv.runMgr.Start("f1", nil, "u")
	if err != nil {
		t.Fatal(err)
	}
	id := handle.ID

	patch := func(body string) (int, string) {
		t.Helper()
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, httptest.NewRequest("PATCH", "/v1/runs/"+id+"/queue", strings.NewReader(body)))
		return w.Code, w.Body.String()
	}
	if code, _ := patch(`{"kind":"push","tier":"later","text":"hello"}`); code != http.StatusOK {
		t.Fatalf("push -> %d", code)
	}
	if code, _ := patch(`{"kind":"bad","tier":"later","text":"x"}`); code != http.StatusBadRequest {
		t.Fatalf("bad kind -> %d", code)
	}
	queue, err := srv.runMgr.QueueList(id)
	if err != nil || len(queue) != 1 {
		t.Fatalf("queue = %+v err=%v, want 1 item", queue, err)
	}
}

// --- Phase 1, Task 2: run jobs ---

func TestRunManager_Jobs(t *testing.T) {
	srv := newRunServer(t)
	handle, err := srv.runMgr.Start("f1", nil, "u")
	if err != nil {
		t.Fatal(err)
	}
	id := handle.ID

	job, err := srv.runMgr.TrackJob(id, "compile", "building go binary")
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != "ongoing" {
		t.Fatalf("status = %q, want ongoing", job.Status)
	}
	done, err := srv.runMgr.UpdateJob(id, job.ID, "completed", "")
	if err != nil {
		t.Fatal(err)
	}
	if done.Status != "completed" || done.Duration == "" || done.FinishedAt == nil {
		t.Fatalf("done = %+v, want completed with duration+finished_at", done)
	}

	// HTTP GET /v1/runs/{id}/jobs
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/v1/runs/"+id+"/jobs", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET jobs -> %d", w.Code)
	}
	var resp struct {
		Jobs  []*RunJob `json:"jobs"`
		Count int       `json:"count"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Count != 1 || len(resp.Jobs) != 1 {
		t.Fatalf("jobs = %+v, want 1", resp)
	}
}

// --- Phase 1, Task 5: approval decision bridge ---

func TestRunInput_ApprovalDecision(t *testing.T) {
	srv := newRunServer(t)
	srv.SetApprovalManager(approval.NewPolicyApprovalManager())
	handle, err := srv.runMgr.Start("f1", nil, "u")
	if err != nil {
		t.Fatal(err)
	}
	id := handle.ID

	// Submit an approval request → stays pending (no handler).
	submit := httptest.NewRecorder()
	srv.Handler().ServeHTTP(submit, httptest.NewRequest("POST", "/v1/approvals",
		strings.NewReader(`{"source":"old","capability":"bash_execute","subject":"rm -rf /tmp"}`)))
	if submit.Code != http.StatusCreated {
		t.Fatalf("submit -> %d (%s)", submit.Code, submit.Body.String())
	}
	var rec struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(submit.Body.Bytes(), &rec); err != nil {
		t.Fatal(err)
	}
	if rec.ID == "" {
		t.Fatal("expected a pending record id")
	}

	// Decide via POST /v1/runs/{id}/input.
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, httptest.NewRequest("POST", "/v1/runs/"+id+"/input",
		strings.NewReader(`{"record_id":"`+rec.ID+`","approve":true,"justification":"dev approved"}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("decision -> %d (%s)", w.Code, w.Body.String())
	}

	// The generated job should be tracked on the run.
	jobs, err := srv.runMgr.Jobs(id)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, j := range jobs {
		if j.Kind == "approval.sandbox_permissions" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected approval job to be tracked, jobs = %+v", jobs)
	}
}

func TestRunInput_QueueMessage(t *testing.T) {
	srv := newRunServer(t)
	handle, err := srv.runMgr.Start("f1", nil, "u")
	if err != nil {
		t.Fatal(err)
	}
	id := handle.ID

	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, httptest.NewRequest("POST", "/v1/runs/"+id+"/input",
		strings.NewReader(`{"message":"give it a try","preempt_mode":"next"}`)))
	if w.Code != http.StatusAccepted {
		t.Fatalf("input -> %d (%s)", w.Code, w.Body.String())
	}
	var resp struct {
		Item *RunQueueItem `json:"item"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Item == nil || resp.Item.Tier != RunQueueTierNext {
		t.Fatalf("item = %+v, want next tier", resp.Item)
	}
}