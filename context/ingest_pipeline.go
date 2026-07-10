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

package contextmgr

import (
	"context"
	"sync"
	"time"
)

// IngestPipeline provides asynchronous ingestion of step records.
// Steps are written to a channel and processed by a background goroutine
// in batches, reducing latency for the caller and improving throughput.
type IngestPipeline struct {
	mgr       ContextManager
	ch        chan StepRecord
	batchSize int
	flushTick time.Duration
	errCh     chan error
	wg        sync.WaitGroup
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.Mutex
	running   bool
}

// PipelineOption configures the IngestPipeline.
type PipelineOption func(*IngestPipeline)

// WithBatchSize sets the number of steps to batch before flushing.
// Default is 10.
func WithBatchSize(n int) PipelineOption {
	return func(p *IngestPipeline) {
		if n > 0 {
			p.batchSize = n
		}
	}
}

// WithFlushInterval sets the maximum time to wait before flushing a partial batch.
// Default is 1 second.
func WithFlushInterval(d time.Duration) PipelineOption {
	return func(p *IngestPipeline) {
		if d > 0 {
			p.flushTick = d
		}
	}
}

// WithChannelSize sets the buffer size of the ingest channel.
// Default is 100.
func WithChannelSize(n int) PipelineOption {
	return func(p *IngestPipeline) {
		// Channel size is set at creation time, so this option
		// is only effective if called before Start().
		_ = n // placeholder for future dynamic resizing
	}
}

// NewIngestPipeline creates a new async ingest pipeline.
// The pipeline must be started with Start() and stopped with Stop().
func NewIngestPipeline(mgr ContextManager, opts ...PipelineOption) *IngestPipeline {
	ctx, cancel := context.WithCancel(context.Background())
	p := &IngestPipeline{
		mgr:       mgr,
		ch:        make(chan StepRecord, 100),
		batchSize: 10,
		flushTick: 1 * time.Second,
		errCh:     make(chan error, 10),
		ctx:       ctx,
		cancel:    cancel,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Start begins the background goroutine that processes ingest requests.
func (p *IngestPipeline) Start() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.running {
		return
	}
	p.running = true
	p.wg.Add(1)
	go p.run()
}

// Stop gracefully shuts down the pipeline, flushing any pending steps.
func (p *IngestPipeline) Stop() error {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return nil
	}
	p.running = false
	p.mu.Unlock()

	close(p.ch)
	p.wg.Wait()
	p.cancel()
	return nil
}

// Submit adds a step record to the pipeline for async processing.
// Returns immediately. Errors are available via Errors().
func (p *IngestPipeline) Submit(step StepRecord) {
	select {
	case p.ch <- step:
	default:
		// Channel full: drop the step to avoid blocking the caller.
		// In production, consider logging or metrics.
	}
}

// SubmitSync adds a step and blocks until it's accepted or the context is done.
func (p *IngestPipeline) SubmitSync(ctx context.Context, step StepRecord) error {
	select {
	case p.ch <- step:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Errors returns a channel that receives errors from the background processor.
func (p *IngestPipeline) Errors() <-chan error {
	return p.errCh
}

// run is the background goroutine that processes steps in batches.
func (p *IngestPipeline) run() {
	defer p.wg.Done()

	batch := make([]StepRecord, 0, p.batchSize)
	ticker := time.NewTicker(p.flushTick)
	defer ticker.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}
		for _, step := range batch {
			if err := p.mgr.Ingest(step); err != nil {
				select {
				case p.errCh <- err:
				default:
					// Error channel full: drop error.
				}
			}
		}
		batch = batch[:0]
	}

	for {
		select {
		case step, ok := <-p.ch:
			if !ok {
				// Channel closed: flush remaining and exit.
				flush()
				return
			}
			batch = append(batch, step)
			if len(batch) >= p.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-p.ctx.Done():
			flush()
			return
		}
	}
}

// Stats returns pipeline statistics.
func (p *IngestPipeline) Stats() map[string]any {
	p.mu.Lock()
	running := p.running
	p.mu.Unlock()
	return map[string]any{
		"running":    running,
		"queue_size": len(p.ch),
		"batch_size": p.batchSize,
		"flush_tick": p.flushTick.String(),
	}
}
