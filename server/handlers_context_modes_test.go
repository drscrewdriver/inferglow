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
	"net/http/httptest"
	"testing"
)

// TestHandleContextModes_ComposedShape verifies the R10 surface: base modes
// and orthogonal improvement passes are two separate axes.
func TestHandleContextModes_ComposedShape(t *testing.T) {
	srv := &Server{}
	rec := httptest.NewRecorder()
	srv.handleContextModes(rec, httptest.NewRequest("GET", "/v1/context/modes", nil))
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	var body struct {
		Modes []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Default bool   `json:"default"`
		} `json:"modes"`
		Improvements []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Default bool   `json:"default"`
		} `json:"improvements"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Modes) != 5 {
		t.Fatalf("modes = %d, want 5", len(body.Modes))
	}
	defs := 0
	for _, m := range body.Modes {
		if m.Default {
			defs++
			if m.ID != "hybrid" {
				t.Errorf("default mode = %q, want hybrid", m.ID)
			}
		}
	}
	if defs != 1 {
		t.Fatalf("default modes = %d, want 1", defs)
	}
	if len(body.Improvements) != 1 || body.Improvements[0].ID != "tool_denoise" || !body.Improvements[0].Default {
		t.Fatalf("improvements = %+v, want tool_denoise default-on", body.Improvements)
	}
}
