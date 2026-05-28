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
	"regexp"
	"strings"
	"testing"

	"github.com/inferglow/model"
	"github.com/inferglow/session"
)

// testPIIMasker is a lightweight PIIMasker implementation used by agent
// tests to verify input/output masking integration without pulling in the
// real security/pii dependency. It replaces email-like substrings with
// "***" and honors the maskInput/maskOutput toggles (analogous to
// pii.Masker's ApplyOn config).
type testPIIMasker struct {
	maskInput  bool
	maskOutput bool
}

var emailRe = regexp.MustCompile(`\S+@\S+\.\S+`)

func (m *testPIIMasker) MaskInput(text string) string {
	if !m.maskInput {
		return text
	}
	return emailRe.ReplaceAllString(text, "***")
}

func (m *testPIIMasker) MaskOutput(text string) string {
	if !m.maskOutput {
		return text
	}
	return emailRe.ReplaceAllString(text, "***")
}

// TestPIIMasker_InputMasking verifies that user input containing PII is
// redacted before it enters the session history when WithPIIMasker is
// configured for input masking.
func TestPIIMasker_InputMasking(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()

	mockReq := &mockModelRequester{
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

	masker := &testPIIMasker{maskInput: true}
	agent := New(sess, actExt, mockReq, WithPIIMasker(masker))

	_, err := agent.Run(context.Background(), "contact me at alice@example.com")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	// The user message stored in the session must be masked.
	full := sess.GetFullContext()
	var userContent string
	for _, msg := range full {
		if msg.Role == "user" {
			userContent, _ = msg.Content.(string)
		}
	}
	if strings.Contains(userContent, "alice@example.com") {
		t.Errorf("user input not masked in session: %q", userContent)
	}
	if !strings.Contains(userContent, "***") {
		t.Errorf("expected mask char in session, got %q", userContent)
	}
}

// TestPIIMasker_OutputMasking verifies that the LLM's final response is
// redacted before Run returns it to the caller when WithPIIMasker is
// configured for output masking.
func TestPIIMasker_OutputMasking(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()

	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 1)
			ch <- &model.StreamChunk{
				Delta:  `{"next_action":"response","final_response":"reply to alice@example.com"}`,
				IsDone: true,
			}
			close(ch)
			return ch, nil
		},
	}

	masker := &testPIIMasker{maskOutput: true}
	agent := New(sess, actExt, mockReq, WithPIIMasker(masker))

	result, err := agent.Run(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if strings.Contains(result, "alice@example.com") {
		t.Errorf("output not masked: %q", result)
	}
	if !strings.Contains(result, "***") {
		t.Errorf("expected mask char in output, got %q", result)
	}
}

// TestPIIMasker_BothSides verifies maskInput | maskOutput masks both
// the stored input and the returned output.
func TestPIIMasker_BothSides(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()

	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 1)
			ch <- &model.StreamChunk{
				Delta:  `{"next_action":"response","final_response":"seen bob@example.com"}`,
				IsDone: true,
			}
			close(ch)
			return ch, nil
		},
	}

	masker := &testPIIMasker{maskInput: true, maskOutput: true}
	agent := New(sess, actExt, mockReq, WithPIIMasker(masker))

	result, err := agent.Run(context.Background(), "my email is alice@example.com")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	// Output masked
	if strings.Contains(result, "bob@example.com") {
		t.Errorf("output not masked: %q", result)
	}
	// Input masked in session
	for _, msg := range sess.GetFullContext() {
		if msg.Role == "user" {
			if s, ok := msg.Content.(string); ok && strings.Contains(s, "alice@example.com") {
				t.Errorf("input not masked in session: %q", s)
			}
		}
	}
}

// TestPIIMasker_DisabledByDefault verifies that without WithPIIMasker no
// masking occurs (backward compatibility).
func TestPIIMasker_DisabledByDefault(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()

	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 1)
			ch <- &model.StreamChunk{
				Delta:  `{"next_action":"response","final_response":"reply to alice@example.com"}`,
				IsDone: true,
			}
			close(ch)
			return ch, nil
		},
	}

	agent := New(sess, actExt, mockReq)

	result, err := agent.Run(context.Background(), "contact alice@example.com")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !strings.Contains(result, "alice@example.com") {
		t.Errorf("output should not be masked without masker: %q", result)
	}
	for _, msg := range sess.GetFullContext() {
		if msg.Role == "user" {
			if s, ok := msg.Content.(string); ok && !strings.Contains(s, "alice@example.com") {
				t.Errorf("input should not be masked without masker: %q", s)
			}
		}
	}
}
