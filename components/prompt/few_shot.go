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

package prompt

import (
	"context"
	"fmt"

	"github.com/inferglow/model"
)

// Example is a single input/output pair used for few-shot prompting.
type Example struct {
	// Input is the user message for this example.
	Input string
	// Output is the expected assistant response.
	Output string
}

// FewShotTemplate renders a system prompt followed by a series of
// input/output examples as alternating user/assistant messages, and
// finally the actual user input. This implements the ChatTemplate
// interface so it can be used anywhere a template is expected.
//
// Example:
//
//	tmpl := NewFewShotTemplate("You are a translator.", []Example{
//	    {Input: "Hello", Output: "Bonjour"},
//	    {Input: "Goodbye", Output: "Au revoir"},
//	})
//	msgs, _ := tmpl.Format(ctx, map[string]any{"input": "Thanks"})
//	// msgs: [system, user("Hello"), assistant("Bonjour"),
//	//        user("Goodbye"), assistant("Au revoir"), user("Thanks")]
type FewShotTemplate struct {
	systemPrompt string
	examples     []Example
}

// NewFewShotTemplate creates a FewShotTemplate with the given system prompt
// and example pairs.
func NewFewShotTemplate(systemPrompt string, examples []Example) *FewShotTemplate {
	return &FewShotTemplate{
		systemPrompt: systemPrompt,
		examples:     examples,
	}
}

// Format renders the few-shot prompt. The vars map must contain an "input"
// key whose value is the actual user message. The rendered messages are:
//  1. system message (the system prompt)
//  2. Alternating user/assistant messages for each example
//  3. Final user message from vars["input"]
func (t *FewShotTemplate) Format(ctx context.Context, vars map[string]any) ([]model.ChatMessage, error) {
	messages := make([]model.ChatMessage, 0, 1+len(t.examples)*2+1)

	// System prompt.
	messages = append(messages, model.ChatMessage{
		Role:    string(model.RoleSystem),
		Content: t.systemPrompt,
	})

	// Example pairs.
	for i, ex := range t.examples {
		messages = append(messages, model.ChatMessage{
			Role:    string(model.RoleUser),
			Content: ex.Input,
		})
		messages = append(messages, model.ChatMessage{
			Role:    string(model.RoleAssistant),
			Content: ex.Output,
		})
		_ = i
	}

	// Actual user input.
	input, ok := vars["input"]
	if !ok {
		return nil, fmt.Errorf("few_shot: missing 'input' variable")
	}
	messages = append(messages, model.ChatMessage{
		Role:    string(model.RoleUser),
		Content: fmt.Sprint(input),
	})

	return messages, nil
}
