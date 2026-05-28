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
	"strings"
	"testing"

	"github.com/inferglow/model"
	"github.com/inferglow/session"
)

// TestExecuteLoop_L3PromptInjection verifies that when outputSchema is
// configured on the Engine, executeLoop injects the L3 schema prompt
// (formatSchemaInstruction) into req.System before calling
// GenerateRequestData. The L3 prompt is a fallback for providers that
// cannot enforce json_schema-level response_format.
func TestExecuteLoop_L3PromptInjection(t *testing.T) {
	sess := NewSessionExtension(session.NewSession("test", 10000))
	actExt := NewActionExtension()

	var capturedReq *model.ModelRequest
	mockReq := &mockModelRequester{
		requestFn: func(ctx context.Context, req *model.ModelRequest) {
			capturedReq = req
		},
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 1)
			ch <- &model.StreamChunk{
				Delta:  `{"next_action":"response","final_response":"ok"}`,
				IsDone: true,
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
		},
	}

	decision, err := engine.executeLoop(context.Background(), "test", 1, "system")
	if err != nil {
		t.Fatalf("executeLoop failed: %v", err)
	}
	_ = decision

	if capturedReq == nil {
		t.Fatal("expected req to be captured")
	}
	if !strings.Contains(capturedReq.System, "[输出格式要求]") {
		t.Errorf("expected req.System to contain L3 schema prompt, got: %s", capturedReq.System)
	}
}
