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

// Behavior tests for the C-8 Knowledge Base store and HTTP handlers.

package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inferglow/rag"
)

// TestKBStoreCreateListIngestSearch covers the store layer: create, list, get,
// ingest (with splitting), search and delete.
func TestKBStoreCreateListIngestSearch(t *testing.T) {
	ks := NewKBStore(nil)

	if err := ks.Create("docs", "product docs"); err != nil {
		t.Fatal(err)
	}
	if err := ks.Create("docs", "dup"); err == nil {
		t.Fatal("expected duplicate create to fail")
	}

	rec, err := ks.Get("docs")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Name != "docs" || rec.Description != "product docs" {
		t.Fatalf("Get returned unexpected record: %+v", rec)
	}

	// Ingest raw content -> split into chunks.
	added, err := ks.Ingest(context.Background(), "docs", []rag.Document{
		{Content: "The InferGlow framework supports graph-based flows and RAG pipelines."},
	})
	if err != nil {
		t.Fatal(err)
	}
	if added != 1 {
		t.Fatalf("added = %d, want 1", added)
	}
	if rec, _ := ks.Get("docs"); rec.DocCount != 1 {
		t.Fatalf("DocCount = %d, want 1", rec.DocCount)
	}

	// Search should return the document.
	results, err := ks.Search(context.Background(), "docs", "graph-based flows", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Score <= 0 {
		t.Fatalf("search returned unexpected results: %+v", results)
	}

	// Search on a missing KB -> error.
	if _, err := ks.Search(context.Background(), "nope", "x", 5); err == nil {
		t.Fatal("expected search on missing KB to fail")
	}

	// Delete.
	if err := ks.Delete("docs"); err != nil {
		t.Fatal(err)
	}
	if _, err := ks.Get("docs"); err == nil {
		t.Fatal("expected Get after delete to fail")
	}
	if got := ks.List(); len(got) != 0 {
		t.Fatalf("List after delete = %v, want empty", got)
	}
}

// TestKBHandlerEndToEnd drives the HTTP layer end-to-end.
func TestKBHandlerEndToEnd(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore())
	srv.SetKBStore(NewKBStore(nil))

	// Create.
	req := httptest.NewRequest("POST", "/v1/knowledge-bases",
		strings.NewReader(`{"name":"kb1","description":"first"}`))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d (%s)", w.Code, w.Body.String())
	}

	// List.
	req = httptest.NewRequest("GET", "/v1/knowledge-bases", nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"name":"kb1"`) {
		t.Fatalf("list: want 200 with kb1, got %d (%s)", w.Code, w.Body.String())
	}

	// Ingest.
	req = httptest.NewRequest("POST", "/v1/knowledge-bases/kb1/ingest",
		strings.NewReader(`{"content":"InferGlow supports graph-based flows and RAG pipelines."}`))
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("ingest: want 200, got %d (%s)", w.Code, w.Body.String())
	}

	// Search.
	req = httptest.NewRequest("POST", "/v1/knowledge-bases/kb1/search",
		strings.NewReader(`{"query":"graph-based flows","limit":5}`))
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("search: want 200, got %d (%s)", w.Code, w.Body.String())
	}
	var results []rag.SearchResult
	if err := json.NewDecoder(w.Body).Decode(&results); err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("search returned no results")
	}

	// Delete.
	req = httptest.NewRequest("DELETE", "/v1/knowledge-bases/kb1", nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("delete: want 200, got %d", w.Code)
	}

	// Get after delete -> 404.
	req = httptest.NewRequest("GET", "/v1/knowledge-bases/kb1", nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("get after delete: want 404, got %d", w.Code)
	}
}

// TestKBUnconfigured503 asserts handlers return 503 when the store is not wired.
func TestKBUnconfigured503(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore()) // no SetKBStore
	req := httptest.NewRequest("GET", "/v1/knowledge-bases", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", w.Code)
	}
}