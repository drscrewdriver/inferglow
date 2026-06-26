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

package eval

import (
	"context"
	"sync"
	"time"

	"github.com/inferglow/model"
)

// ScriptedProvider is a mock model provider that returns pre-recorded
// responses in sequence. It implements model.ModelRequester so it can be
// passed directly to agent.New for evaluation runs.
type ScriptedProvider struct {
	// Responses is the ordered list of responses to return. Each call to
	// RequestModel consumes the next response. When exhausted, the last
	// response is repeated indefinitely.
	Responses []ScriptedResponse

	// ResponseDelay adds an artificial delay before each stream delivery.
	// Zero means no delay (default).
	ResponseDelay time.Duration

	// Name is the provider name returned by Name().
	ProviderName string

	mu    sync.Mutex
	index int
	calls int
}

// ScriptedResponse is a single pre-recorded response in the sequence.
type ScriptedResponse struct {
	// Content is the delta text sent in the stream chunk.
	Content string

	// ToolCalls are optional tool calls sent alongside the content.
	ToolCalls []model.ToolCall

	// Usage is the optional usage info attached to the final chunk.
	Usage *model.UsageInfo
}

// Name returns the provider name.
func (s *ScriptedProvider) Name() string {
	if s.ProviderName != "" {
		return s.ProviderName
	}
	return "scripted"
}

// GenerateRequestData converts the model request to request data.
// For the scripted provider, it simply passes through the model name.
func (s *ScriptedProvider) GenerateRequestData(_ context.Context, req *model.ModelRequest) (*model.RequestData, error) {
	return &model.RequestData{
		Model: req.Model,
	}, nil
}

// RequestModel returns a stream channel with the next scripted response.
func (s *ScriptedProvider) RequestModel(ctx context.Context, _ *model.RequestData) (<-chan *model.StreamChunk, error) {
	s.mu.Lock()
	idx := s.index
	if idx < len(s.Responses)-1 {
		s.index++
	}
	s.calls++
	s.mu.Unlock()

	if s.ResponseDelay > 0 {
		select {
		case <-time.After(s.ResponseDelay):
		case <-ctx.Done():
			ch := make(chan *model.StreamChunk)
			close(ch)
			return ch, ctx.Err()
		}
	}

	ch := make(chan *model.StreamChunk, 2)
	resp := s.Responses[idx]
	chunk := &model.StreamChunk{
		Delta:  resp.Content,
		Tools:  resp.ToolCalls,
		Usage:  resp.Usage,
		IsDone: true,
	}
	ch <- chunk
	close(ch)
	return ch, nil
}

// BroadcastResponse is a no-op implementation to satisfy model.ModelRequester.
func (s *ScriptedProvider) BroadcastResponse(_ context.Context, _ <-chan *model.StreamChunk) (<-chan *model.ResultEvent, error) {
	ch := make(chan *model.ResultEvent)
	close(ch)
	return ch, nil
}

// CallCount returns the number of times RequestModel was called.
func (s *ScriptedProvider) CallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// Compile-time interface check.
var _ model.ModelRequester = (*ScriptedProvider)(nil)
