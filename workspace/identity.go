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

package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sync"
)

// ContentIdentity captures the identity of a file at a point in time.
type ContentIdentity struct {
	// Path is the file path relative to workspace root.
	Path string `json:"path"`
	// Digest is the SHA-256 hex digest of the file content.
	Digest string `json:"digest"`
	// Size is the file size in bytes.
	Size int64 `json:"size"`
	// ContentVersion is a monotonically increasing version string.
	ContentVersion string `json:"content_version"`
}

// IdentityCatalog tracks content identities for workspace files.
type IdentityCatalog struct {
	mu       sync.RWMutex
	identities map[string]*ContentIdentity
	versionSeq int
}

// NewIdentityCatalog creates an empty catalog.
func NewIdentityCatalog() *IdentityCatalog {
	return &IdentityCatalog{
		identities: make(map[string]*ContentIdentity),
	}
}

// ObservePath computes the identity of a file by reading it from disk.
func (c *IdentityCatalog) ObservePath(path string) (*ContentIdentity, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("observe %s: %w", path, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("observe %s: read: %w", path, err)
	}
	h := sha256.Sum256(data)
	digest := hex.EncodeToString(h[:])

	c.mu.Lock()
	defer c.mu.Unlock()

	existing, ok := c.identities[path]
	var version string
	if ok && existing.Digest == digest {
		return existing, nil
	}
	c.versionSeq++
	version = fmt.Sprintf("v%d", c.versionSeq)

	id := &ContentIdentity{
		Path:           path,
		Digest:         digest,
		Size:           info.Size(),
		ContentVersion: version,
	}
	c.identities[path] = id
	return id, nil
}

// GetVersion returns the current content version for a path, or empty string.
func (c *IdentityCatalog) GetVersion(path string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	id, ok := c.identities[path]
	if !ok {
		return ""
	}
	return id.ContentVersion
}

// GetIdentity returns the full identity for a path, or nil.
func (c *IdentityCatalog) GetIdentity(path string) *ContentIdentity {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.identities[path]
}

// ListIdentities returns all tracked identities.
func (c *IdentityCatalog) ListIdentities() []*ContentIdentity {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]*ContentIdentity, 0, len(c.identities))
	for _, id := range c.identities {
		result = append(result, id)
	}
	return result
}
