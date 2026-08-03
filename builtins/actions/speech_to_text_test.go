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
)

func TestSpeechToTextActionSpec(t *testing.T) {
	a := NewSpeechToTextAction(nil)
	if a.Name != SpeechToTextActionID {
		t.Errorf("Name = %q, want %q", a.Name, SpeechToTextActionID)
	}
	if a.Executor == nil {
		t.Error("Executor should not be nil")
	}
}

func TestSpeechToTextActionRegistry(t *testing.T) {
	r := action.NewRegistry()
	if err := r.Register(NewSpeechToTextAction(nil)); err != nil {
		t.Fatalf("Register error: %v", err)
	}
	if !r.Has(SpeechToTextActionID) {
		t.Errorf("registry missing %q", SpeechToTextActionID)
	}
}

func TestSpeechToTextExecutorSuccessString(t *testing.T) {
	a := NewSpeechToTextAction(nil)
	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"audio_data": "fake-base64-audio-data",
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
	if result["text"] != "This is a mock transcription of the provided audio." {
		t.Errorf("text = %q", result["text"])
	}
	if result["duration"] != 5.0 {
		t.Errorf("duration = %v", result["duration"])
	}
}

func TestSpeechToTextExecutorSuccessBytes(t *testing.T) {
	a := NewSpeechToTextAction(nil)
	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"audio_data": []byte{0x00, 0x01, 0x02},
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
}

func TestSpeechToTextExecutorSuccessWithOptions(t *testing.T) {
	mock := &MockSpeechTranscriber{Transcription: "custom transcription"}
	a := NewSpeechToTextAction(mock)
	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"audio_data": "test-audio",
		"language":   "en",
		"model":      "whisper-1",
		"mime_type":  "audio/mp3",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	result, ok := res.Result.(map[string]any)
	if !ok {
		t.Fatalf("Result not map[string]any: %T", res.Result)
	}
	if result["text"] != "custom transcription" {
		t.Errorf("text = %q", result["text"])
	}
}

func TestSpeechToTextExecutorMissingAudioData(t *testing.T) {
	a := NewSpeechToTextAction(nil)
	res, _ := a.Executor.Execute(context.Background(), map[string]any{})
	if res.OK {
		t.Errorf("expected OK=false for missing audio_data")
	}
	if res.Error != "speech_to_text: audio_data is required (base64 string or bytes)" {
		t.Errorf("Error = %q", res.Error)
	}
}

func TestSpeechToTextExecutorEmptyAudioData(t *testing.T) {
	a := NewSpeechToTextAction(nil)
	res, _ := a.Executor.Execute(context.Background(), map[string]any{
		"audio_data": "",
	})
	if res.OK {
		t.Errorf("expected OK=false for empty audio_data")
	}
	if res.Error != "speech_to_text: audio_data cannot be empty" {
		t.Errorf("Error = %q", res.Error)
	}
}

func TestSpeechToTextExecutorTranscriberError(t *testing.T) {
	mock := &MockSpeechTranscriber{Err: errors.New("transcription failed")}
	a := NewSpeechToTextAction(mock)
	res, _ := a.Executor.Execute(context.Background(), map[string]any{
		"audio_data": "some-audio",
	})
	if res.OK {
		t.Errorf("expected OK=false when transcriber errors")
	}
	if res.Error != "speech_to_text: transcription failed" {
		t.Errorf("Error = %q", res.Error)
	}
}

func TestSpeechToTextExecutorNilTranscriber(t *testing.T) {
	// Passing nil should use MockSpeechTranscriber internally.
	a := NewSpeechToTextAction(nil)
	res, _ := a.Executor.Execute(context.Background(), map[string]any{
		"audio_data": "test-audio",
	})
	if !res.OK {
		t.Errorf("expected OK=true with nil transcriber (uses MockSpeechTranscriber)")
	}
}

func TestSpeechToTextExecutorWrongType(t *testing.T) {
	a := NewSpeechToTextAction(nil)
	res, _ := a.Executor.Execute(context.Background(), map[string]any{
		"audio_data": 123,
	})
	if res.OK {
		t.Errorf("expected OK=false for wrong audio_data type")
	}
	if res.Error != "speech_to_text: audio_data is required (base64 string or bytes)" {
		t.Errorf("Error = %q", res.Error)
	}
}