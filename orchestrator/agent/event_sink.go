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

package agent

import (
	"context"
	"fmt"
	"sync"
)

// EventKind identifies the type of agent event.
type EventKind int

const (
	// EventRunStart signals the beginning of Agent.Run.
	EventRunStart EventKind = iota
	// EventRunEnd signals the completion of Agent.Run.
	EventRunEnd
	// EventLLMStart signals the start of an LLM invocation.
	EventLLMStart
	// EventLLMEnd signals the end of an LLM invocation.
	EventLLMEnd
	// EventToolStart signals the start of a tool execution.
	EventToolStart
	// EventToolEnd signals the end of a tool execution.
	EventToolEnd
	// EventToken carries a text delta from the LLM stream.
	EventToken
	// EventReasoning carries a reasoning delta from the LLM stream.
	EventReasoning
	// EventError carries an error that occurred during execution.
	EventError
	// EventApproval signals a tool blocked by approval policy.
	EventApproval
	// EventCompression signals that context compression occurred.
	EventCompression
)

// String returns a human-readable name for the event kind.
func (k EventKind) String() string {
	switch k {
	case EventRunStart:
		return "run_start"
	case EventRunEnd:
		return "run_end"
	case EventLLMStart:
		return "llm_start"
	case EventLLMEnd:
		return "llm_end"
	case EventToolStart:
		return "tool_start"
	case EventToolEnd:
		return "tool_end"
	case EventToken:
		return "token"
	case EventReasoning:
		return "reasoning"
	case EventError:
		return "error"
	case EventApproval:
		return "approval"
	case EventCompression:
		return "compression"
	default:
		return "unknown"
	}
}

// AgentEvent is a typed event emitted during agent execution.
type AgentEvent struct {
	Kind     EventKind
	Text     string            // token/reasoning delta or response text
	ToolName string            // for EventToolStart/EventToolEnd
	Round    int               // for EventLLMStart/EventLLMEnd
	Tokens   int               // for EventLLMEnd: approximate completion token count
	Err      error             // for EventError/EventRunEnd
	Status   string            // ActionResult status: "blocked"/"success"/"error"
	Metadata map[string]string // generic extension fields (recordID, sandboxMode, etc.)
}

// EventSink receives agent events. Implementations must be safe for
// sequential use from the agent goroutine. For concurrent consumers,
// wrap with a mutex or use NewChannelSink.
type EventSink interface {
	Emit(AgentEvent)
}

// FuncEventSink adapts a function to the EventSink interface.
type FuncEventSink func(AgentEvent)

// Emit implements EventSink.
func (f FuncEventSink) Emit(e AgentEvent) { f(e) }

// DiscardSink is an EventSink that drops all events (zero overhead).
var DiscardSink EventSink = FuncEventSink(func(AgentEvent) {})

// channelSink is an EventSink backed by a buffered channel.
type channelSink struct {
	ch chan AgentEvent
}

// Emit sends the event to the channel. Non-blocking for EventToken/
// EventReasoning (drops on full buffer to avoid stalling the LLM stream);
// blocking for lifecycle events to guarantee delivery.
func (s *channelSink) Emit(e AgentEvent) {
	switch e.Kind {
	case EventToken, EventReasoning:
		// Non-blocking: drop token events if consumer is slow.
		select {
		case s.ch <- e:
		default:
		}
	default:
		// Blocking: guarantee lifecycle event delivery.
		s.ch <- e
	}
}

// NewChannelSink creates an EventSink backed by a buffered channel.
// The returned channel delivers events in emission order. Token/reasoning
// events are dropped when the buffer is full; lifecycle events block.
// Close the channel by calling the returned close function when done.
// The close function is idempotent — safe to call multiple times.
func NewChannelSink(buf int) (EventSink, <-chan AgentEvent, func()) {
	if buf <= 0 {
		buf = 256
	}
	ch := make(chan AgentEvent, buf)
	sink := &channelSink{ch: ch}
	var once sync.Once
	closeFn := func() { once.Do(func() { close(ch) }) }
	return sink, ch, closeFn
}

// CallbacksFromSink maps an EventSink to AgentCallbacks, allowing any
// EventSink to be used with WithCallbacks. Each callback hook emits the
// corresponding AgentEvent to the sink.
func CallbacksFromSink(sink EventSink) *AgentCallbacks {
	if sink == nil {
		return nil
	}
	return &AgentCallbacks{
		OnRunStart: func(ctx context.Context, userMessage string) {
			sink.Emit(AgentEvent{Kind: EventRunStart, Text: userMessage})
		},
		OnRunEnd: func(ctx context.Context, response string, err error) {
			sink.Emit(AgentEvent{Kind: EventRunEnd, Text: response, Err: err})
		},
		OnLLMCallStart: func(ctx context.Context, round int) {
			sink.Emit(AgentEvent{Kind: EventLLMStart, Round: round})
		},
		OnLLMCallEnd: func(ctx context.Context, round int, tokens int) {
			sink.Emit(AgentEvent{Kind: EventLLMEnd, Round: round, Tokens: tokens})
		},
		OnToolCallStart: func(ctx context.Context, toolName string) {
			sink.Emit(AgentEvent{Kind: EventToolStart, ToolName: toolName})
		},
		OnToolCallEnd: func(ctx context.Context, toolName string, err error) {
			sink.Emit(AgentEvent{Kind: EventToolEnd, ToolName: toolName, Err: err})
		},
		OnToken: func(ctx context.Context, delta string) {
			sink.Emit(AgentEvent{Kind: EventToken, Text: delta})
		},
		OnReasoning: func(ctx context.Context, delta string) {
			sink.Emit(AgentEvent{Kind: EventReasoning, Text: delta})
		},
		OnApprovalRequired: func(ctx context.Context, toolName, recordID string) {
			sink.Emit(AgentEvent{
				Kind:     EventApproval,
				ToolName: toolName,
				Metadata: map[string]string{"recordID": recordID},
			})
		},
		OnCompression: func(ctx context.Context, stepsCompressed int) {
			sink.Emit(AgentEvent{
				Kind:     EventCompression,
				Metadata: map[string]string{"stepsCompressed": fmt.Sprintf("%d", stepsCompressed)},
			})
		},
	}
}
