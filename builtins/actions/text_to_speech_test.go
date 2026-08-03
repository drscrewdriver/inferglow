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

package actions

import (
	"context"
	"errors"
	"testing"

	"github.com/inferglow/action"
	"github.com/inferglow/model"
)

func TestTextToSpeechActionSpec(t *testing.T) {
	a := NewTextToSpeechAction(nil)
	if a.Name != TextToSpeechActionID {
		t.Errorf("Name = %q, want %q", a.Name, TextToSpeechActionID)
	}
	if a.Executor == nil {
		t.Error("Executor should not be nil")
	}
}

func TestTextToSpeechActionRegistry(t *testing.T) {
	r := action.NewRegistry()
	if err := r.Register(NewTextToSpeechAction(nil)); err != nil {
		t.Fatalf("Register error: %v", err)
	}
	if !r.Has(TextToSpeechActionID) {
		t.Errorf("registry missing %q", TextToSpeechActionID)
	}
}

func TestTextToSpeechExecutorSuccess(t *testing.T) {
	a := NewTextToSpeechAction(nil)
	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"text": "Hello, world!",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	if res.Status != "success" {
		t.Errorf("Status = %q, want success", res.Status)
	}
	result, ok := res.Result.(map[string]any)
	if !ok {
		t.Fatalf("Result not map[string]any: %T", res.Result)
	}
	if result["text"] != "Hello, world!" {
		t.Errorf("text = %v", result["text"])
	}
	if result["mime_type"] != "audio/mp3" {
		t.Errorf("mime_type = %v", result["mime_type"])
	}
	if len(res.ContentBlocks) != 1 {
		t.Fatalf("expected 1 ContentBlock, got %d", len(res.ContentBlocks))
	}
	if res.ContentBlocks[0].Type != model.ContentAudio {
		t.Errorf("ContentBlock type = %q, want audio", res.ContentBlocks[0].Type)
	}
}

func TestTextToSpeechExecutorSuccessWithOptions(t *testing.T) {
	a := NewTextToSpeechAction(nil)
	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"text":  "Hello!",
		"voice": "alloy",
		"model": "tts-1",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
}

func TestTextToSpeechExecutorMissingText(t *testing.T) {
	a := NewTextToSpeechAction(nil)
	res, _ := a.Executor.Execute(context.Background(), map[string]any{})
	if res.OK {
		t.Errorf("expected OK=false for missing text")
	}
	if res.Error != "text_to_speech: text is required" {
		t.Errorf("Error = %q", res.Error)
	}
}

func TestTextToSpeechExecutorSynthesizerError(t *testing.T) {
	mock := &MockSpeechSynthesizer{Err: errors.New("synthesis failed")}
	a := NewTextToSpeechAction(mock)
	res, _ := a.Executor.Execute(context.Background(), map[string]any{
		"text": "Hello!",
	})
	if res.OK {
		t.Errorf("expected OK=false when synthesizer errors")
	}
	if res.Error != "text_to_speech: synthesis failed" {
		t.Errorf("Error = %q", res.Error)
	}
}

func TestTextToSpeechExecutorNilSynthesizer(t *testing.T) {
	// Passing nil should use MockSpeechSynthesizer internally.
	a := NewTextToSpeechAction(nil)
	res, _ := a.Executor.Execute(context.Background(), map[string]any{
		"text": "test speech",
	})
	if !res.OK {
		t.Errorf("expected OK=true with nil synthesizer (uses MockSpeechSynthesizer)")
	}
}

func TestTextToSpeechExecutorURLResult(t *testing.T) {
	// Custom synthesizer that returns a URL instead of inline bytes.
	urlSynth := &speechURLSynthesizer{}
	a := NewTextToSpeechAction(urlSynth)
	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"text": "remote audio",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	if len(res.ContentBlocks) != 1 {
		t.Fatalf("expected 1 ContentBlock, got %d", len(res.ContentBlocks))
	}
	if res.ContentBlocks[0].Type != model.ContentAudio {
		t.Errorf("ContentBlock type = %q, want audio", res.ContentBlocks[0].Type)
	}
	if res.ContentBlocks[0].URL != "https://example.com/audio.mp3" {
		t.Errorf("URL = %q", res.ContentBlocks[0].URL)
	}
}

// speechURLSynthesizer returns a result with a URL and no inline data.
type speechURLSynthesizer struct{}

func (s *speechURLSynthesizer) Synthesize(ctx context.Context, text string, opts SpeechSynthOptions) (*SpeechSynthResult, error) {
	return &SpeechSynthResult{
		URL: "https://example.com/audio.mp3",
	}, nil
}