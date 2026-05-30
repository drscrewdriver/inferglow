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

// Package taskdag provides dynamic task graph (DAG) execution for the
// inferglow orchestrator. It is the Go equivalent of Agently's
// core/orchestration/TaskDAG: model-generated task graphs with
// dependency resolution, topological ordering, and parallel execution.
package taskdag

import "errors"

// Sentinel errors.
var (
	ErrCycleDetected   = errors.New("task DAG contains cycle")
	ErrNodeNotFound    = errors.New("task node not found")
	ErrDuplicateNode   = errors.New("duplicate task node")
	ErrHandlerNotFound = errors.New("handler not found for task kind")
	ErrNodeFailed      = errors.New("task node failed")
)

// TaskNode represents a single node in the task DAG.
type TaskNode struct {
	// ID is the unique identifier for this node.
	ID string `json:"id"`
	// Kind classifies the node type (e.g. "model", "action", "local").
	Kind string `json:"kind"`
	// Binding is the specific handler binding (e.g. model name, action name).
	Binding string `json:"binding,omitempty"`
	// DependsOn lists the IDs of predecessor nodes.
	DependsOn []string `json:"depends_on,omitempty"`
	// Optional marks the node as skippable if dependencies fail.
	Optional bool `json:"optional,omitempty"`
	// Input carries node-specific configuration.
	Input map[string]any `json:"input,omitempty"`
}

// TaskDAG is a directed acyclic graph of task nodes.
type TaskDAG struct {
	// ID is the unique identifier for this DAG.
	ID string `json:"id"`
	// Tasks is the list of nodes.
	Tasks []TaskNode `json:"tasks"`
	// Outputs maps semantic output names to node IDs that produce them.
	Outputs map[string]string `json:"outputs,omitempty"`
}
