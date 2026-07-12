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
	"github.com/inferglow/model"
)

// TextToSpeechActionID is the registered Action name for TTS.
const TextToSpeechActionID = "text_to_speech"

// TextToSpeechSpec is the ActionSpec for text_to_speech.
var TextToSpeechSpec = &action.ActionSpec{
	ActionID:         TextToSpeechActionID,
	Name:             "TextToSpeech",
	Description:      "Convert text to speech audio using OpenAI TTS, ElevenLabs, or compatible API.",
	SideEffectLevel:  action.SideEffectWrite,
	ApprovalRequired: false,
	SandboxRequired:  false,
	ReplaySafe:       true,
	ExposeToModel:    true,
	Tags:             []string{"multimodal", "audio", "tts", "builtin"},
	Kwargs: map[string]any{
		"text":  map[string]any{"type": "string", "required": true, "description": "Text to convert to speech"},
		"voice": map[string]any{"type": "string", "required": false, "description": "Voice name, e.g. 'alloy', 'echo', 'shimmer'"},
		"model": map[string]any{"type": "string", "required": false, "description": "Model name, e.g. 'tts-1', 'tts-1-hd'"},
	},
}

// SpeechSynthesizer is the abstraction for TTS backends.
type SpeechSynthesizer interface {
	Synthesize(ctx context.Context, text string, opts SpeechSynthOptions) (*SpeechSynthResult, error)
}

// SpeechSynthOptions carries optional parameters for TTS.
type SpeechSynthOptions struct {
	Voice string // e.g. "alloy", "echo"
	Model string // e.g. "tts-1"
}

// SpeechSynthResult is the output of a TTS call.
type SpeechSynthResult struct {
	AudioData []byte // raw audio bytes (MP3/WAV/OGG)
	MIMEType  string // e.g. "audio/mp3"
	URL       string // remote URL (if provider returns URL)
}

// MockSpeechSynthesizer is a deterministic mock for testing.
type MockSpeechSynthesizer struct {
	Err error
}

func (m *MockSpeechSynthesizer) Synthesize(ctx context.Context, text string, opts SpeechSynthOptions) (*SpeechSynthResult, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	// Return minimal MP3 frame as placeholder
	return &SpeechSynthResult{
		AudioData: []byte{0xff, 0xfb, 0x90, 0x00}, // MP3 frame header
		MIMEType:  "audio/mp3",
	}, nil
}

// textToSpeechExecutor binds a SpeechSynthesizer to the ActionExecutor contract.
type textToSpeechExecutor struct {
	synthesizer SpeechSynthesizer
}

func (e *textToSpeechExecutor) Execute(ctx context.Context, input map[string]any) (*action.ActionResult, error) {
	if e == nil || e.synthesizer == nil {
		return &action.ActionResult{
			OK:     false,
			Status: "error",
			Error:  "text_to_speech: no speech synthesizer configured",
		}, nil
	}
	text, _ := input["text"].(string)
	if text == "" {
		return &action.ActionResult{
			OK:     false,
			Status: "error",
			Error:  "text_to_speech: text is required",
		}, nil
	}
	opts := SpeechSynthOptions{}
	if v, ok := input["voice"].(string); ok {
		opts.Voice = v
	}
	if m, ok := input["model"].(string); ok {
		opts.Model = m
	}
	result, err := e.synthesizer.Synthesize(ctx, text, opts)
	if err != nil {
		return &action.ActionResult{
			OK:     false,
			Status: "error",
			Error:  fmt.Sprintf("text_to_speech: %s", err.Error()),
		}, nil
	}
	// Build ContentBlock for the generated audio
	var blocks []model.ContentBlock
	if len(result.AudioData) > 0 {
		blocks = append(blocks, model.AudioBlock(result.MIMEType, result.AudioData))
	} else if result.URL != "" {
		blocks = append(blocks, model.AudioURLBlock(result.URL))
	}
	return &action.ActionResult{
		OK:            true,
		Status:        "success",
		Result:        map[string]any{"text": text, "mime_type": result.MIMEType},
		ContentBlocks: blocks,
	}, nil
}

// NewTextToSpeechAction builds an Action for text-to-speech.
// If synthesizer is nil, a MockSpeechSynthesizer is used.
func NewTextToSpeechAction(synthesizer SpeechSynthesizer) *action.Action {
	if synthesizer == nil {
		synthesizer = &MockSpeechSynthesizer{}
	}
	return &action.Action{
		Name:        TextToSpeechActionID,
		Description: "Convert text to speech audio.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text":  map[string]any{"type": "string", "description": "Text to convert"},
				"voice": map[string]any{"type": "string", "description": "Voice name (e.g. alloy, echo)"},
				"model": map[string]any{"type": "string", "description": "Model name (e.g. tts-1)"},
			},
			"required": []string{"text"},
		},
		Executor: &textToSpeechExecutor{synthesizer: synthesizer},
		Tags:     []string{"multimodal", "audio", "tts", "builtin"},
	}
}
