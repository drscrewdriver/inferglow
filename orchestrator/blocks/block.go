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

// Package blocks provides structured, composable execution blocks for the
// inferglow orchestrator. It is the Go equivalent of Agently's
// core/application/Blocks + builtins/blocks: high-level building blocks
// (Reason, Act, Intent) that compile into flow.Operator graphs.
package blocks

import (
	"context"
	"errors"

	"github.com/inferglow/flow"
)

// Sentinel errors.
var (
	ErrBlockNotFound = errors.New("block not found")
	ErrBlockExists   = errors.New("block already registered")
	ErrBlockExecFail = errors.New("block execution failed")
)

// FlowBlock is the interface for composable execution blocks.
type FlowBlock interface {
	// Name returns the block's unique name.
	Name() string
	// BuildOperators compiles the block into flow operators.
	BuildOperators(ctx context.Context, blueprint *BlockBlueprint) ([]*flow.Operator, error)
	// Execute runs the block with the given input.
	Execute(ctx context.Context, input any) (any, error)
}

// BlockBlueprint describes a sequence of block references with configuration.
type BlockBlueprint struct {
	// Blocks is the ordered list of block references.
	Blocks []BlockRef `json:"blocks"`
}

// BlockRef is a reference to a named block with configuration.
type BlockRef struct {
	// BlockName is the name of the block to invoke.
	BlockName string `json:"block_name"`
	// Config carries block-specific configuration.
	Config map[string]any `json:"config,omitempty"`
}
