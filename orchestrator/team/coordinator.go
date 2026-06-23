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

package team

import (
	"context"
	"fmt"
	"strings"

	"github.com/inferglow/orchestrator/middleware"
)

// CoordinatorOption configures a Coordinator.
type CoordinatorOption func(*Coordinator)

// WithMaxRounds sets the maximum number of coordination rounds.
func WithMaxRounds(n int) CoordinatorOption {
	return func(c *Coordinator) {
		if n > 0 {
			c.maxRounds = n
		}
	}
}

// WithMiddleware adds unified middlewares to the coordinator.
func WithMiddleware(mws ...middleware.Middleware) CoordinatorOption {
	return func(c *Coordinator) {
		c.middlewares = append(c.middlewares, mws...)
	}
}

// Coordinator orchestrates multiple Agent instances for collaborative tasks.
type Coordinator struct {
	members     []Member
	bus         *messageBus
	maxRounds   int
	middlewares []middleware.Middleware
	memberMap   map[string]*Member // role → Member for quick lookup
}

// NewCoordinator creates a Coordinator with the given members and options.
// Default maxRounds is 3 if not specified.
func NewCoordinator(members []Member, opts ...CoordinatorOption) *Coordinator {
	c := &Coordinator{
		members:   members,
		bus:       newMessageBus(),
		maxRounds: 3,
		memberMap: make(map[string]*Member, len(members)),
	}
	for i := range members {
		c.memberMap[members[i].Role] = &members[i]
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Round executes one coordination round: each member processes the task
// in order, with results passed via the messageBus. Returns the consolidated
// Result after all members have contributed.
//
// The coordination loop is wrapped with middleware.Chain so that cross-cutting
// concerns (tracing, audit, rate limiting) can be applied uniformly.
func (c *Coordinator) Round(ctx context.Context, task string) (*Result, error) {
	// Build the core handler.
	core := c.coordinationLoop

	// Wrap with middlewares.
	handler := middleware.Chain(c.middlewares...)(core)

	// Execute.
	out, err := handler(ctx, &middleware.Input{
		Messages: []middleware.Message{{Role: "user", Content: task}},
	})
	if err != nil {
		return nil, fmt.Errorf("team round: %w", err)
	}

	// Build result from output metadata.
	result := &Result{
		MemberOutputs: make(map[string]string),
		Rounds:        1,
	}
	if out != nil {
		if resp, ok := out.Metadata["final_response"].(string); ok {
			result.FinalResponse = resp
		}
		if outputs, ok := out.Metadata["member_outputs"].(map[string]string); ok {
			result.MemberOutputs = outputs
		}
	}
	return result, nil
}

// coordinationLoop is the core dispatch logic wrapped by middleware.
func (c *Coordinator) coordinationLoop(ctx context.Context, input *middleware.Input) (*middleware.Output, error) {
	task := ""
	if len(input.Messages) > 0 {
		task = input.Messages[len(input.Messages)-1].Content
	}

	memberOutputs := make(map[string]string)
	var lastResponse string

	for _, m := range c.members {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Build the prompt: task + any handoff context from previous members.
		prompt := task
		if msgs := c.bus.MessagesTo(m.Role); len(msgs) > 0 {
			var ctx_parts []string
			for _, msg := range msgs {
				ctx_parts = append(ctx_parts, fmt.Sprintf("[%s→%s]: %s", msg.From, msg.To, msg.Content))
			}
			prompt = task + "\n\nContext from team:\n" + strings.Join(ctx_parts, "\n")
		}

		resp, err := m.Agent.Run(ctx, prompt)
		if err != nil {
			return nil, fmt.Errorf("member %s: %w", m.Role, err)
		}

		memberOutputs[m.Role] = resp
		lastResponse = resp

		// Post to bus for downstream members.
		c.bus.Post(Message{
			From:    m.Role,
			To:      c.nextMember(m.Role),
			Content: resp,
		})
	}

	return &middleware.Output{
		Messages: []middleware.Message{{Role: "assistant", Content: lastResponse}},
		Metadata: map[string]any{
			"final_response":  lastResponse,
			"member_outputs":  memberOutputs,
		},
	}, nil
}

// nextMember returns the role of the next member in the list (wrapping around).
// Returns empty string if there is only one member.
func (c *Coordinator) nextMember(currentRole string) string {
	for i, m := range c.members {
		if m.Role == currentRole {
			if i+1 < len(c.members) {
				return c.members[i+1].Role
			}
			return ""
		}
	}
	return ""
}

// Bus returns the Coordinator's messageBus for inspection/testing.
func (c *Coordinator) Bus() *messageBus {
	return c.bus
}
