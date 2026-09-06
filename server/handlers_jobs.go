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
	"net/http"
	"time"
)

// handleListJobs handles GET /v1/jobs — global background-job listing across
// all runs (Spec B, reusing the RunManager's RunJob tracking).
func (s *Server) handleListJobs(w http.ResponseWriter, _ *http.Request) {
	if s.runMgr == nil {
		writeError(w, http.StatusServiceUnavailable, "run manager not configured")
		return
	}
	jobs := s.runMgr.AllJobs()
	writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs, "count": len(jobs)})
}

// handleGetJob handles GET /v1/jobs/{id} — lookup a single background job.
func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	if s.runMgr == nil {
		writeError(w, http.StatusServiceUnavailable, "run manager not configured")
		return
	}
	id := r.PathValue("id")
	job, ok := s.runMgr.FindJob(id)
	if !ok {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	writeJSON(w, http.StatusOK, job)
}

// handleJobsStream handles GET /v1/jobs/stream — SSE live polling of background
// jobs. When ?run_id= is provided only that run's events are forwarded;
// otherwise events from all runs are aggregated.
func (s *Server) handleJobsStream(w http.ResponseWriter, r *http.Request) {
	if s.runMgr == nil {
		writeError(w, http.StatusServiceUnavailable, "run manager not configured")
		return
	}
	runID := r.URL.Query().Get("run_id")

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}
	flusher.Flush()

	// Subscriptions: the requested run, or every live run.
	type sub struct {
		ch      <-chan RunEvent
		cleanup func()
	}
	subs := []sub{}
	if runID != "" {
		ch, cleanup, err := s.runMgr.Subscribe(runID)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		subs = append(subs, sub{ch, cleanup})
	} else {
		for _, h := range s.runMgr.List("") {
			if ch, cleanup, err := s.runMgr.Subscribe(h.GetID()); err == nil {
				subs = append(subs, sub{ch, cleanup})
			}
		}
	}
	for _, s := range subs {
		defer s.cleanup()
	}

	forward := func(ev RunEvent) {
		writeSSEEvent(w, ev.Type, ev.Data)
		flusher.Flush()
	}

	ctx := r.Context()
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		// Non-blocking drain across all subscriptions.
		progressed := false
		for _, s := range subs {
			select {
			case ev, ok := <-s.ch:
				if ok {
					forward(ev)
					progressed = true
				}
			default:
			}
		}
		if progressed || len(subs) == 0 {
			// Either we made progress (continue draining) or there is nothing
			// to subscribe to (fall back to the blocking select below, which
			// only has the keep-alive ticker and the request context).
			if progressed {
				continue
			}
		}
		if len(subs) > 0 {
			select {
			case ev, ok := <-subs[0].ch:
				if ok {
					forward(ev)
				}
			case <-ticker.C:
				writeSSEEvent(w, "ping", map[string]string{"ts": time.Now().UTC().Format(time.RFC3339)})
				flusher.Flush()
			case <-ctx.Done():
				return
			}
			continue
		}
		select {
		case <-ticker.C:
			writeSSEEvent(w, "ping", map[string]string{"ts": time.Now().UTC().Format(time.RFC3339)})
			flusher.Flush()
		case <-ctx.Done():
			return
		}
	}
}