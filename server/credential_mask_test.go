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

// Behavior tests for the C-6 credential store and its data-masking rules. The
// key invariant: the raw secret is never present in any JSON serialization.

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestMaskSecret verifies the keep-first/last-4 + mask-middle rule and the
// collapse behavior for short secrets.
func TestMaskSecret(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"a", "****"},
		{"abcd", "****"},
		{"abcdefgh", "****"},
		{"abcdefghij", "abcd****ghij"},
		{"abcdefghijkl", "abcd****ijkl"},
	}
	for _, c := range cases {
		if got := maskSecret(c.in); got != c.want {
			t.Errorf("maskSecret(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestCredentialStoreMaskedPreview confirms Create computes the masked preview
// at write time and that the store list/get never expose SecretValue.
func TestCredentialStoreMaskedPreview(t *testing.T) {
	cs := NewCredentialStore()
	id, err := cs.Create(CredentialRecord{
		Name:        "gh",
		Provider:    "github",
		Username:    "octocat",
		SecretValue: "super-secret-token-abc",
	})
	if err != nil {
		t.Fatal(err)
	}
	rec := cs.Get(id)
	if rec.SecretMasked == nil {
		t.Fatal("expected SecretMasked to be populated")
	}
	if *rec.SecretMasked == rec.SecretValue {
		t.Fatal("masked preview must not equal the raw secret")
	}
	if rec.SecretValue != "super-secret-token-abc" {
		t.Fatal("in-memory raw secret should be preserved for internal use")
	}

	// Serialization must not contain the raw secret anywhere.
	raw, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "super-secret-token-abc") {
		t.Fatalf("serialized credential leaked the raw secret: %s", raw)
	}
	if !strings.Contains(string(raw), *rec.SecretMasked) {
		t.Fatalf("serialized credential missing masked preview: %s", raw)
	}
}

// TestCredentialStoreValidate rejects records missing required fields.
func TestCredentialStoreValidate(t *testing.T) {
	cs := NewCredentialStore()
	if _, err := cs.Create(CredentialRecord{Provider: "github"}); err == nil {
		t.Fatal("expected error when name is empty")
	}
	if _, err := cs.Create(CredentialRecord{Name: "gh"}); err == nil {
		t.Fatal("expected error when provider is empty")
	}
}

// TestCredentialHandlerNeverLeaksSecret drives the HTTP layer end-to-end and
// asserts the raw secret is absent from create/list/get responses.
func TestCredentialHandlerNeverLeaksSecret(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore())
	srv.SetCredentialStore(NewCredentialStore())

	const secret = "shh-this-is-the-real-secret-value"
	body := `{"name":"gh","provider":"github","username":"octocat","secret":"` + secret + `"}`
	req := httptest.NewRequest("POST", "/v1/credentials", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d (%s)", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), secret) {
		t.Fatal("create response leaked the raw secret")
	}

	var created struct {
		ID     string `json:"id"`
		Secret string `json:"secret"`
	}
	_ = json.NewDecoder(w.Body).Decode(&created)
	if created.ID == "" {
		t.Fatal("missing credential id")
	}

	// Get must likewise not leak.
	req = httptest.NewRequest("GET", "/v1/credentials/"+created.ID, nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get: want 200, got %d", w.Code)
	}
	if strings.Contains(w.Body.String(), secret) {
		t.Fatal("get response leaked the raw secret")
	}

	// List must not leak either.
	req = httptest.NewRequest("GET", "/v1/credentials", nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list: want 200, got %d", w.Code)
	}
	if strings.Contains(w.Body.String(), secret) {
		t.Fatal("list response leaked the raw secret")
	}
}

// TestCredentialUnconfigured503 asserts handlers return 503 when the store is
// not wired, so unassembled servers degrade gracefully.
func TestCredentialUnconfigured503(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore()) // no SetCredentialStore
	req := httptest.NewRequest("GET", "/v1/credentials", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", w.Code)
	}
}
