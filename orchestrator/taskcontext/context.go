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

// Package taskcontext provides task-scoped context aggregationation for the
// inferglow orchestrator. It is the Go equivalent of Agently's core/context/
// layer: a collector that gathers context from multiple sources, applies
// budget constraints, and produces a bounded consumption for model prompts.
package taskcontext

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Sentinel errors.
var (
	ErrSourceNotFound  = errors.New("context source not found")
	ErrEntryExists     = errors.New("context entry already exists")
	ErrBudgetExceeded  = errors.New("context budget exceeded")
)

// SourceRef identifies a specific piece of context within a source.
type SourceRef struct {
	// Source is the name of the context source.
	Source string `json:"source"`
	// Ref is the source-specific reference (e.g. file path, key).
	Ref string `json:"ref"`
}

// Descriptor is a lightweight summary of available context.
type Descriptor struct {
	Ref         SourceRef `json:"ref"`
	Description string    `json:"description"`
	SizeHint    int       `json:"size_hint"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// DescriptorPage is a paginated list of descriptors.
type DescriptorPage struct {
	Descriptors []Descriptor `json:"descriptors"`
	NextCursor  string       `json:"next_cursor"`
}

// ContextSourceRead is the content read from a source.
type ContextSourceRead struct {
	Ref      SourceRef `json:"ref"`
	Content  string    `json:"content"`
	Truncated bool    `json:"truncated"`
}

// ContextSource is the interface for pluggable context providers.
type ContextSource interface {
	// EnumerateDescriptors returns a paginated list of available context.
	EnumerateDescriptors(ctx context.Context, cursor string, limit int) (*DescriptorPage, error)
	// ReadExact reads the content identified by ref, up to maxChars.
	ReadExact(ctx context.Context, ref SourceRef, maxChars int) (*ContextSourceRead, error)
}

// ContextEntry is a direct context value added to the TaskContext.
type ContextEntry struct {
	// Key is a unique identifier for this entry.
	Key string `json:"key"`
	// Content is the text content.
	Content string `json:"content"`
	// Priority controls selection order (higher = first).
	Priority int `json:"priority"`
	// Metadata carries optional key-value pairs.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ContextBudget limits the total context consumed.
type ContextBudget struct {
	// MaxChars is the maximum total characters.
	MaxChars int `json:"max_chars"`
	// MaxBlocks is the maximum number of context blocks.
	MaxBlocks int `json:"max_blocks"`
	// MaxBlockChars is the maximum characters per single block.
	MaxBlockChars int `json:"max_block_chars"`
}

// DefaultBudget returns a budget with sensible defaults.
func DefaultBudget() ContextBudget {
	return ContextBudget{
		MaxChars:      100000,
		MaxBlocks:     50,
		MaxBlockChars: 10000,
	}
}

// ContextBlock is a single block of consumed context.
type ContextBlock struct {
	Source   string            `json:"source"`
	Ref      string            `json:"ref"`
	Content  string            `json:"content"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ContextConsumption is the bounded output of a context read.
type ContextConsumption struct {
	Blocks     []ContextBlock `json:"blocks"`
	TotalChars int            `json:"total_chars"`
	Truncated  bool           `json:"truncated"`
}

// TaskContextSnapshot is an immutable snapshot of the context state.
type TaskContextSnapshot struct {
	Entries []ContextEntry `json:"entries"`
	Sources int            `json:"sources"`
}

// TaskContext aggregates context from multiple sources and direct entries.
type TaskContext struct {
	mu      sync.RWMutex
	sources []ContextSource
	entries map[string]ContextEntry
}

// NewTaskContext creates an empty TaskContext.
func NewTaskContext() *TaskContext {
	return &TaskContext{
		entries: make(map[string]ContextEntry),
	}
}

// Attach adds a context source. Returns the receiver for chaining.
func (tc *TaskContext) Attach(source ContextSource) *TaskContext {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.sources = append(tc.sources, source)
	return tc
}

// Put adds or replaces a direct entry. Returns the receiver for chaining.
func (tc *TaskContext) Put(entry ContextEntry) *TaskContext {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.entries[entry.Key] = entry
	return tc
}

// Remove removes a direct entry by key. Returns true if found.
func (tc *TaskContext) Remove(key string) bool {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	_, ok := tc.entries[key]
	if ok {
		delete(tc.entries, key)
	}
	return ok
}

// Snapshot returns an immutable copy of the current state.
func (tc *TaskContext) Snapshot() *TaskContextSnapshot {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	entries := make([]ContextEntry, 0, len(tc.entries))
	for _, e := range tc.entries {
		entries = append(entries, e)
	}
	return &TaskContextSnapshot{
		Entries: entries,
		Sources: len(tc.sources),
	}
}

// ReaderOption configures a ContextReader.
type ReaderOption func(*ContextReader)

// WithBudget sets the context budget.
func WithBudget(budget ContextBudget) ReaderOption {
	return func(r *ContextReader) { r.budget = budget }
}

// Reader creates a ContextReader with the given options.
func (tc *TaskContext) Reader(opts ...ReaderOption) *ContextReader {
	r := &ContextReader{
		tc:     tc,
		budget: DefaultBudget(),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// ContextReader reads context from a TaskContext within budget constraints.
type ContextReader struct {
	tc     *TaskContext
	budget ContextBudget
}

// SetBudget updates the reader's budget.
func (r *ContextReader) SetBudget(budget ContextBudget) {
	r.budget = budget
}

// Refresh is a no-op reserved for future cache invalidation.
func (r *ContextReader) Refresh() {}

// Read consumes context from all sources and entries, respecting the budget.
func (r *ContextReader) Read(ctx context.Context) (*ContextConsumption, error) {
	r.tc.mu.RLock()
	defer r.tc.mu.RUnlock()

	consumption := &ContextConsumption{}

	// 1. Collect direct entries sorted by priority (descending).
	entries := make([]ContextEntry, 0, len(r.tc.entries))
	for _, e := range r.tc.entries {
		entries = append(entries, e)
	}
	// Simple insertion sort by priority descending.
	for i := 1; i < len(entries); i++ {
		j := i
		for j > 0 && entries[j].Priority > entries[j-1].Priority {
			entries[j], entries[j-1] = entries[j-1], entries[j]
			j--
		}
	}

	for _, e := range entries {
		if len(consumption.Blocks) >= r.budget.MaxBlocks {
			consumption.Truncated = true
			break
		}
		content := e.Content
		if r.budget.MaxBlockChars > 0 && len(content) > r.budget.MaxBlockChars {
			content = content[:r.budget.MaxBlockChars]
			consumption.Truncated = true
		}
		if r.budget.MaxChars > 0 && consumption.TotalChars+len(content) > r.budget.MaxChars {
			consumption.Truncated = true
			break
		}
		consumption.Blocks = append(consumption.Blocks, ContextBlock{
			Source:   "entry",
			Ref:      e.Key,
			Content:  content,
			Metadata: e.Metadata,
		})
		consumption.TotalChars += len(content)
	}

	// 2. Collect from sources.
	for _, src := range r.tc.sources {
		if len(consumption.Blocks) >= r.budget.MaxBlocks {
			consumption.Truncated = true
			break
		}
		page, err := src.EnumerateDescriptors(ctx, "", 10)
		if err != nil {
			continue
		}
		for _, desc := range page.Descriptors {
			if len(consumption.Blocks) >= r.budget.MaxBlocks {
				consumption.Truncated = true
				break
			}
			remaining := r.budget.MaxChars - consumption.TotalChars
			if remaining <= 0 {
				consumption.Truncated = true
				break
			}
			maxChars := r.budget.MaxBlockChars
			if maxChars > remaining {
				maxChars = remaining
			}
			read, err := src.ReadExact(ctx, desc.Ref, maxChars)
			if err != nil {
				continue
			}
			consumption.Blocks = append(consumption.Blocks, ContextBlock{
				Source:  desc.Ref.Source,
				Ref:     desc.Ref.Ref,
				Content: read.Content,
			})
			consumption.TotalChars += len(read.Content)
		}
	}

	return consumption, nil
}
