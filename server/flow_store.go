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

	"github.com/inferglow/flow/flowdef"
	"github.com/inferglow/flow/stage"
	"github.com/inferglow/storage"
)

// FlowStore is a thread-safe registry of declarative flow definitions.
// It supports hot-loading: flows can be registered, listed, and retrieved
// at runtime without restarting the server.
// The backing KV storage is provided by the generic storage.Map primitive.
type FlowStore struct {
	*storage.Map[string, *flowdef.FlowDef]
	stages  *stage.Registry
	adapter *flowdef.Adapter
}

// NewFlowStore creates a FlowStore backed by the given stage registry.
func NewFlowStore(stages *stage.Registry) *FlowStore {
	return &FlowStore{
		Map:     storage.NewMap[string, *flowdef.FlowDef](),
		stages:  stages,
		adapter: flowdef.NewAdapter(stages),
	}
}

// Register validates and stores a flow definition. The flow's metadata.name
// is used as the key. Returns an error if validation fails.
func (fs *FlowStore) Register(def *flowdef.FlowDef) error {
	if err := flowdef.Validate(def); err != nil {
		return fmt.Errorf("flow store: validate: %w", err)
	}
	fs.Map.Set(def.Metadata.Name, def) // concurrency handled by the underlying Map
	return nil
}

// Get returns the flow definition for the given name, or nil if not found.
func (fs *FlowStore) Get(name string) (*flowdef.FlowDef, bool) {
	return fs.Map.Get(name)
}

// List returns the names of all registered flows.
func (fs *FlowStore) List() []string {
	return fs.Map.Keys()
}

// Adapter returns the flowdef.Adapter for compiling FlowDefs into executable
// *flow.Flow instances.
func (fs *FlowStore) Adapter() *flowdef.Adapter {
	return fs.adapter
}

// Stages returns the stage registry.
func (fs *FlowStore) Stages() *stage.Registry {
	return fs.stages
}
