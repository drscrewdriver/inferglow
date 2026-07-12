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
	"fmt"

	"github.com/inferglow/action"
)

// SpeechToTextActionID is the registered Action name for STT.
const SpeechToTextActionID = "speech_to_text"

// SpeechToTextSpec is the ActionSpec for speech_to_text.
var SpeechToTextSpec = &action.ActionSpec{
	ActionID:         SpeechToTextActionID,
	Name:             "SpeechToText",
	Description:      "Transcribe audio to text using Whisper, Deepgram, or compatible API.",
	SideEffectLevel:  action.SideEffectRead,
	ApprovalRequired: false,
	SandboxRequired:  false,
	ReplaySafe:       true,
	ExposeToModel:    true,
	Tags:             []string{"multimodal", "audio", "stt", "transcription", "builtin"},
	Kwargs: map[string]any{
		"audio_data": map[string]any{"type": "string", "required": true, "description": "Base64-encoded audio data or URL to audio file"},
		"language":   map[string]any{"type": "string", "required": false, "description": "Language code, e.g. 'en', 'zh', 'ja'"},
		"model":      map[string]any{"type": "string", "required": false, "description": "Model name, e.g. 'whisper-1', 'deepgram-nova-2'"},
	},
}

// SpeechTranscriber is the abstraction for STT backends.
type SpeechTranscriber interface {
	Transcribe(ctx context.Context, audioData []byte, opts SpeechTranscribeOptions) (*SpeechTranscribeResult, error)
}

// SpeechTranscribeOptions carries optional parameters for STT.
type SpeechTranscribeOptions struct {
	Language string // e.g. "en", "zh"
	Model    string // e.g. "whisper-1"
	MIMEType string // e.g. "audio/mp3"
}

// SpeechTranscribeResult is the output of an STT call.
type SpeechTranscribeResult struct {
	Text     string  // transcribed text
	Duration float64 // audio duration in seconds
	Words    []Word  // word-level timestamps (optional)
}

// Word represents a word-level transcription with timing.
type Word struct {
	Text  string  `json:"text"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// MockSpeechTranscriber is a deterministic mock for testing.
type MockSpeechTranscriber struct {
	Transcription string
	Err           error
}

func (m *MockSpeechTranscriber) Transcribe(ctx context.Context, audioData []byte, opts SpeechTranscribeOptions) (*SpeechTranscribeResult, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	text := m.Transcription
	if text == "" {
		text = "This is a mock transcription of the provided audio."
	}
	return &SpeechTranscribeResult{
		Text:     text,
		Duration: 5.0,
	}, nil
}

// speechToTextExecutor binds a SpeechTranscriber to the ActionExecutor contract.
type speechToTextExecutor struct {
	transcriber SpeechTranscriber
}

func (e *speechToTextExecutor) Execute(ctx context.Context, input map[string]any) (*action.ActionResult, error) {
	if e == nil || e.transcriber == nil {
		return &action.ActionResult{
			OK:     false,
			Status: "error",
			Error:  "speech_to_text: no speech transcriber configured",
		}, nil
	}
	// Accept audio_data as base64 string or raw bytes
	var audioData []byte
	switch v := input["audio_data"].(type) {
	case string:
		audioData = []byte(v) // In production, decode base64
	case []byte:
		audioData = v
	default:
		return &action.ActionResult{
			OK:     false,
			Status: "error",
			Error:  "speech_to_text: audio_data is required (base64 string or bytes)",
		}, nil
	}
	if len(audioData) == 0 {
		return &action.ActionResult{
			OK:     false,
			Status: "error",
			Error:  "speech_to_text: audio_data cannot be empty",
		}, nil
	}
	opts := SpeechTranscribeOptions{}
	if l, ok := input["language"].(string); ok {
		opts.Language = l
	}
	if m, ok := input["model"].(string); ok {
		opts.Model = m
	}
	if mt, ok := input["mime_type"].(string); ok {
		opts.MIMEType = mt
	}
	result, err := e.transcriber.Transcribe(ctx, audioData, opts)
	if err != nil {
		return &action.ActionResult{
			OK:     false,
			Status: "error",
			Error:  fmt.Sprintf("speech_to_text: %s", err.Error()),
		}, nil
	}
	return &action.ActionResult{
		OK:     true,
		Status: "success",
		Result: map[string]any{
			"text":     result.Text,
			"duration": result.Duration,
			"words":    result.Words,
		},
	}, nil
}

// NewSpeechToTextAction builds an Action for speech-to-text.
// If transcriber is nil, a MockSpeechTranscriber is used.
func NewSpeechToTextAction(transcriber SpeechTranscriber) *action.Action {
	if transcriber == nil {
		transcriber = &MockSpeechTranscriber{}
	}
	return &action.Action{
		Name:        SpeechToTextActionID,
		Description: "Transcribe audio to text.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"audio_data": map[string]any{"type": "string", "description": "Base64-encoded audio or URL"},
				"language":   map[string]any{"type": "string", "description": "Language code (e.g. en, zh)"},
				"model":      map[string]any{"type": "string", "description": "Model name (e.g. whisper-1)"},
			},
			"required": []string{"audio_data"},
		},
		Executor: &speechToTextExecutor{transcriber: transcriber},
		Tags:     []string{"multimodal", "audio", "stt", "transcription", "builtin"},
	}
}
