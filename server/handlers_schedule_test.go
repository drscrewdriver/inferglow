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

// Behavior tests for the C-5 scheduler handlers (CRUD + start/stop).

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestScheduleCreateAndGet drives the schedule CRUD path over HTTP.
func TestScheduleCreateAndGet(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore())
	srv.SetScheduleStore(NewScheduleStore())

	body := `{"name":"nightly","flow":"sync","interval_ms":60000,"stateful":true,"enabled":false}`
	req := httptest.NewRequest("POST", "/v1/schedules", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d (%s)", w.Code, w.Body.String())
	}
	var created struct {
		ID         string `json:"id"`
		IntervalMS int64  `json:"interval_ms"`
		Stateful   bool   `json:"stateful"`
		Enabled    bool   `json:"enabled"`
	}
	_ = json.NewDecoder(w.Body).Decode(&created)
	if created.ID == "" {
		t.Fatal("missing schedule id")
	}
	if created.IntervalMS != 60000 {
		t.Fatalf("interval_ms = %d, want 60000", created.IntervalMS)
	}

	req = httptest.NewRequest("GET", "/v1/schedules/"+created.ID, nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get: want 200, got %d", w.Code)
	}
}

// TestScheduleStartAndStop verifies the lifecycle endpoints toggle the enabled
// flag via the trigger registry.
func TestScheduleStartAndStop(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore())
	srv.SetScheduleStore(NewScheduleStore())

	id, err := srv.scheduleStore.Create(ScheduleRecord{
		Name:     "poll",
		Flow:     "ingest",
		Interval: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("POST", "/v1/schedules/"+id+"/start", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("start: want 200, got %d (%s)", w.Code, w.Body.String())
	}
	if rec := srv.scheduleStore.Get(id); rec == nil || !rec.Enabled {
		t.Fatal("expected schedule to be enabled after start")
	}

	req = httptest.NewRequest("POST", "/v1/schedules/"+id+"/stop", nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("stop: want 200, got %d (%s)", w.Code, w.Body.String())
	}
	if rec := srv.scheduleStore.Get(id); rec == nil || rec.Enabled {
		t.Fatal("expected schedule to be disabled after stop")
	}
}

// TestScheduleDeleteUnregisters removes a schedule and confirms it is gone.
func TestScheduleDelete(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore())
	srv.SetScheduleStore(NewScheduleStore())

	id, err := srv.scheduleStore.Create(ScheduleRecord{Name: "x", Flow: "f", Interval: 500})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("DELETE", "/v1/schedules/"+id, nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("delete: want 200, got %d", w.Code)
	}
	if srv.scheduleStore.Get(id) != nil {
		t.Fatal("schedule still present after delete")
	}
}

// TestScheduleInvalidInterval rejects a non-positive interval.
func TestScheduleInvalidInterval(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore())
	srv.SetScheduleStore(NewScheduleStore())

	body := `{"name":"bad","flow":"f","interval_ms":0}`
	req := httptest.NewRequest("POST", "/v1/schedules", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d (%s)", w.Code, w.Body.String())
	}
}

// TestScheduleUnconfigured503 asserts 503 when no schedule store is wired.
func TestScheduleUnconfigured503(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore()) // no SetScheduleStore
	req := httptest.NewRequest("GET", "/v1/schedules", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", w.Code)
	}
}
