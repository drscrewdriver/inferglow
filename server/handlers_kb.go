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

	"github.com/inferglow/rag"
)

// handleCreateKnowledgeBase handles POST /v1/knowledge-bases — create a KB.
func (s *Server) handleCreateKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	if s.kbStore == nil {
		writeError(w, http.StatusServiceUnavailable, "knowledge base store not configured")
		return
	}
	if !s.canAccess(r, "knowledge-base", "", "create") {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if err := s.kbStore.Create(req.Name, req.Description); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	rec, err := s.kbStore.Get(req.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, rec)
}

// handleListKnowledgeBases handles GET /v1/knowledge-bases — list KBs.
func (s *Server) handleListKnowledgeBases(w http.ResponseWriter, r *http.Request) {
	if s.kbStore == nil {
		writeError(w, http.StatusServiceUnavailable, "knowledge base store not configured")
		return
	}
	if !s.canAccess(r, "knowledge-base", "", "list") {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}
	writeJSON(w, http.StatusOK, s.kbStore.List())
}

// handleGetKnowledgeBase handles GET /v1/knowledge-bases/{name} — get a KB.
func (s *Server) handleGetKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	if s.kbStore == nil {
		writeError(w, http.StatusServiceUnavailable, "knowledge base store not configured")
		return
	}
	name := r.PathValue("name")
	if !s.canAccess(r, "knowledge-base", name, "read") {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}
	rec, err := s.kbStore.Get(name)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

// handleDeleteKnowledgeBase handles DELETE /v1/knowledge-bases/{name}.
func (s *Server) handleDeleteKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	if s.kbStore == nil {
		writeError(w, http.StatusServiceUnavailable, "knowledge base store not configured")
		return
	}
	name := r.PathValue("name")
	if !s.canAccess(r, "knowledge-base", name, "delete") {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}
	if err := s.kbStore.Delete(name); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "name": name})
}

// handleIngestKnowledgeBase handles POST /v1/knowledge-bases/{name}/ingest.
// It accepts either raw content (split via the rag splitter) or a list of
// pre-chunked documents, then stores them via the KB vector store.
func (s *Server) handleIngestKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	if s.kbStore == nil {
		writeError(w, http.StatusServiceUnavailable, "knowledge base store not configured")
		return
	}
	name := r.PathValue("name")
	if !s.canAccess(r, "knowledge-base", name, "ingest") {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}
	var req struct {
		Content      string `json:"content"`
		Splitter     string `json:"splitter,omitempty"`      // recursive | markdown | token | ""
		ChunkSize    int    `json:"chunk_size,omitempty"`    // default 1000
		ChunkOverlap int    `json:"chunk_overlap,omitempty"` // default 100
		Documents    []struct {
			Content  string         `json:"content"`
			Source   string         `json:"source,omitempty"`
			Metadata map[string]any `json:"metadata,omitempty"`
		} `json:"documents,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	var docs []rag.Document
	if len(req.Documents) > 0 {
		for _, d := range req.Documents {
			docs = append(docs, rag.Document{
				Content:  d.Content,
				Source:   d.Source,
				Metadata: d.Metadata,
			})
		}
	} else if req.Content != "" {
		chunks, err := splitKBContent(req.Content, req.Splitter, req.ChunkSize, req.ChunkOverlap)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		for _, c := range chunks {
			docs = append(docs, rag.Document{Content: c, Source: "ingest"})
		}
	} else {
		writeError(w, http.StatusBadRequest, "either content or documents is required")
		return
	}

	added, err := s.kbStore.Ingest(r.Context(), name, docs)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"added": added, "name": name})
}

// handleSearchKnowledgeBase handles POST /v1/knowledge-bases/{name}/search.
func (s *Server) handleSearchKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	if s.kbStore == nil {
		writeError(w, http.StatusServiceUnavailable, "knowledge base store not configured")
		return
	}
	name := r.PathValue("name")
	if !s.canAccess(r, "knowledge-base", name, "search") {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}
	var req struct {
		Query string `json:"query"`
		Limit int    `json:"limit,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.Query == "" {
		writeError(w, http.StatusBadRequest, "query is required")
		return
	}
	if req.Limit <= 0 {
		req.Limit = 5
	}
	results, err := s.kbStore.Search(r.Context(), name, req.Query, req.Limit)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, results)
}

// splitKBContent splits text via the selected rag splitter. It returns the raw
// text unchanged when no splitter is selected.
func splitKBContent(content, splitter string, chunkSize, overlap int) ([]string, error) {
	if content == "" {
		return nil, nil
	}
	if chunkSize <= 0 {
		chunkSize = 1000
	}
	switch splitter {
	case "markdown":
		return rag.NewMarkdownSplitter(chunkSize).Split(content)
	case "token":
		return rag.NewTokenSplitter(chunkSize, overlap).Split(content)
	case "recursive", "":
		return rag.NewRecursiveCharacterTextSplitter(chunkSize, overlap).Split(content)
	default:
		return nil, nil
	}
}