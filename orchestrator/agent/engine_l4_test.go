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
	"testing"

	"github.com/inferglow/model"
	"github.com/inferglow/session"
)

// TestExecuteLoop_L4ValidationRetry verifies that when outputSchema is
// configured with Required fields, executeLoop validates the LLM output
// against the schema and retries when validation fails. The first model
// call returns JSON missing the required "next_action" field; the second
// call returns valid JSON. The test asserts the model was called exactly
// twice (1 initial + 1 retry) and the final decision reflects the valid
// response.
func TestExecuteLoop_L4ValidationRetry(t *testing.T) {
	sess := NewSessionExtension(session.NewSession("test", 10000))
	actExt := NewActionExtension()

	callCount := 0
	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			callCount++
			ch := make(chan *model.StreamChunk, 1)
			if callCount == 1 {
				// First call: valid JSON but missing required field "next_action"
				ch <- &model.StreamChunk{
					Delta:  `{"final_response":"ok"}`,
					IsDone: true,
				}
			} else {
				// Second call: valid JSON with required field
				ch <- &model.StreamChunk{
					Delta:  `{"next_action":"response","final_response":"ok"}`,
					IsDone: true,
				}
			}
			close(ch)
			return ch, nil
		},
	}

	engine := &Engine{
		session:   sess,
		actionExt: actExt,
		modelReq:  mockReq,
		outputSchema: &model.OutputSchema{
			Type: "object",
			Properties: map[string]any{
				"next_action": map[string]any{"type": "string"},
			},
			Required: []string{"next_action"},
		},
	}

	decision, err := engine.executeLoop(context.Background(), "test", 1, "system")
	if err != nil {
		t.Fatalf("executeLoop failed: %v", err)
	}

	if callCount != 2 {
		t.Errorf("expected model to be called 2 times (1 initial + 1 retry), got %d", callCount)
	}
	if decision.NextAction != "response" {
		t.Errorf("Expected NextAction response, got %q", decision.NextAction)
	}
	if decision.FinalResponse != "ok" {
		t.Errorf("Expected FinalResponse ok, got %q", decision.FinalResponse)
	}
}
