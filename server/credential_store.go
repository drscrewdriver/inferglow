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
	"fmt"
	"sync"
	"time"

	"github.com/inferglow/storage"
)

// CredentialRecord describes a stored credential (spec C-6). The raw secret is
// held in-memory only and is dropped from every JSON serialization via the
// `json:"-"` tag; all API responses expose the masked form instead, derived by
// maskSecret. A nil SecretMasked means the record was created with an empty
// secret.
type CredentialRecord struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Provider     string    `json:"provider"`
	Username     string    `json:"username,omitempty"`
	SecretValue  string    `json:"-"`                // never serialized
	SecretMasked *string   `json:"secret,omitempty"` // masked preview
	CreatedAt    time.Time `json:"created_at"`
}

// CredentialStore is an in-memory store for credentials, mirroring the
// TeamStore template (team_store.go) over the generic storage.Map primitive.
type CredentialStore struct {
	*storage.Map[string, *CredentialRecord]
	metaMu sync.RWMutex // guards nextID + the id-assembly critical section
	nextID int
}

// NewCredentialStore creates an empty CredentialStore.
func NewCredentialStore() *CredentialStore {
	return &CredentialStore{
		Map: storage.NewMap[string, *CredentialRecord](),
	}
}

// Create adds a new credential and returns its ID. The masked preview is
// computed at write time so handlers never need to (and never can) reveal the
// raw secret in a response automatically.
func (cs *CredentialStore) Create(rec CredentialRecord) (string, error) {
	if rec.Name == "" {
		return "", fmt.Errorf("name is required")
	}
	if rec.Provider == "" {
		return "", fmt.Errorf("provider is required")
	}

	cs.metaMu.Lock()
	defer cs.metaMu.Unlock()

	cs.nextID++
	id := fmt.Sprintf("cred-%d", cs.nextID)
	rec.ID = id
	rec.CreatedAt = time.Now()
	if rec.SecretValue != "" {
		m := maskSecret(rec.SecretValue)
		rec.SecretMasked = &m
	}

	cs.Map.Set(id, &rec)
	return id, nil
}

// Get returns a credential by ID, or nil if not found.
func (cs *CredentialStore) Get(id string) *CredentialRecord {
	v, _ := cs.Map.Get(id)
	return v
}

// List returns all credential records.
func (cs *CredentialStore) List() []*CredentialRecord {
	return cs.Map.Values()
}

// Delete removes a credential by ID.
func (cs *CredentialStore) Delete(id string) error {
	if _, ok := cs.Map.Get(id); !ok {
		return fmt.Errorf("credential %q not found", id)
	}
	cs.Map.Delete(id)
	return nil
}
