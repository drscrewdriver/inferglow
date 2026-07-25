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
	"sync"
	"time"

	"github.com/inferglow/workspace"
)

// WorkspaceInfo is the API-facing summary of an opened workspace (spec C-7).
type WorkspaceInfo struct {
	Name    string    `json:"name"`
	Root    string    `json:"root"`
	Created time.Time `json:"created_at"`
}

// WorkspaceProvider is the server-side seam for workspace operations. The
// schema-level workspace.Provider does not exist in this repository, so the
// server defines this thin local interface and backs it with the concrete
// workspace.Workspace read surface (Root/ListDir/Stat). Handlers depend only
// on this interface, keeping C-7 decoupled from the underlying module.
type WorkspaceProvider interface {
	// Open opens (or re-opens) a workspace rooted at rootDir under the given
	// name and returns its info. An existing name is replaced by the new root.
	Open(name, rootDir string) (*WorkspaceInfo, error)
	// Get returns a previously opened workspace's info, or ok=false.
	Get(name string) (*WorkspaceInfo, bool)
	// List returns all opened workspaces.
	List() []*WorkspaceInfo
	// Close removes a workspace from the registry by name.
	Close(name string) error
	// ListDir lists the entries underneath relPath within the named workspace.
	ListDir(name, relPath string) ([]string, error)
}

// DefaultWorkspaceProvider is the built-in in-memory implementation that wraps
// workspace.New for each named workspace. It never writes through the managed
// workspaces, only exposing the read-only surface used by the C-7 handlers.
type DefaultWorkspaceProvider struct {
	mu         sync.RWMutex
	workspaces map[string]*workspace.Workspace
	infos      map[string]*WorkspaceInfo
}

// NewWorkspaceProvider creates an empty DefaultWorkspaceProvider.
func NewWorkspaceProvider() *DefaultWorkspaceProvider {
	return &DefaultWorkspaceProvider{
		workspaces: make(map[string]*workspace.Workspace),
		infos:      make(map[string]*WorkspaceInfo),
	}
}

// Open opens a workspace via workspace.New if not already present. Re-opening
// an existing name keeps its original root and info; opening a new name evokes
// a fresh sandbox over rootDir.
func (p *DefaultWorkspaceProvider) Open(name, rootDir string) (*WorkspaceInfo, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if ws, ok := p.workspaces[name]; ok {
		ws.Root() // ensure non-nil; keep existing binding
		return p.infos[name], nil
	}
	ws, err := workspace.New(workspace.Config{RootDir: rootDir})
	if err != nil {
		return nil, err
	}
	info := &WorkspaceInfo{
		Name:    name,
		Root:    ws.Root(),
		Created: time.Now(),
	}
	p.workspaces[name] = ws
	p.infos[name] = info
	return info, nil
}

// Get returns a workspace's info if it has been opened.
func (p *DefaultWorkspaceProvider) Get(name string) (*WorkspaceInfo, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	info, ok := p.infos[name]
	return info, ok
}

// List returns all opened workspace infos, ordered by name.
func (p *DefaultWorkspaceProvider) List() []*WorkspaceInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]*WorkspaceInfo, 0, len(p.infos))
	for _, info := range p.infos {
		out = append(out, info)
	}
	// stable name ordering for deterministic list responses
	for i := 1; i < len(out); i++ {
		j := i
		for j > 0 && out[j].Name < out[j-1].Name {
			out[j], out[j-1] = out[j-1], out[j]
			j--
		}
	}
	return out
}

// Close removes a workspace binding by name.
func (p *DefaultWorkspaceProvider) Close(name string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.workspaces[name]; !ok {
		return nil
	}
	delete(p.workspaces, name)
	delete(p.infos, name)
	return nil
}

// ListDir lists entries under relPath inside the named workspace.
func (p *DefaultWorkspaceProvider) ListDir(name, relPath string) ([]string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	ws, ok := p.workspaces[name]
	if !ok {
		return nil, nil
	}
	return ws.ListDir(relPath)
}
