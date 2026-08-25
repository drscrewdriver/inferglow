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
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
// MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO
// EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES
// OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
// ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
// DEALINGS IN THE SOFTWARE.

package model

import (
	"reflect"
	"testing"
)

func TestTranslateEffortOpenAI(t *testing.T) {
	lm := EffortLevelMap{"low": "low", "medium": "medium", "high": "high"}
	got := TranslateEffort(EffortOpenAI, "high", lm)
	want := map[string]any{"reasoning_effort": "high"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("openai high = %v, want %v", got, want)
	}
	// ""/auto → nil
	if got := TranslateEffort(EffortOpenAI, "", lm); got != nil {
		t.Fatalf("empty level should be nil, got %v", got)
	}
	if got := TranslateEffort(EffortOpenAI, "auto", lm); got != nil {
		t.Fatalf("auto level should be nil, got %v", got)
	}
	// nil value = not offered
	lm["max"] = nil
	if got := TranslateEffort(EffortOpenAI, "max", lm); got != nil {
		t.Fatalf("nil-valued level should be nil, got %v", got)
	}
	// undeclared level passthrough (openai format allows it)
	if got := TranslateEffort(EffortOpenAI, "xhigh", EffortLevelMap{"high": "high"}); got == nil {
		t.Fatal("undeclared level should passthrough on openai format")
	}
}

func TestTranslateEffortDeepSeek(t *testing.T) {
	// DSH llm-deepseek authoritative map: off/low/high/max, thinking wire.
	lm := EffortLevelMap{"low": "low", "high": "high", "max": "max"}
	got := TranslateEffort(EffortDeepSeek, "max", lm)
	want := map[string]any{
		"thinking":        map[string]any{"type": "enabled"},
		"reasoning_effort": "max",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("deepseek max = %v, want %v", got, want)
	}
	// off → explicit thinking disabled (via EffortOffWire)
	off := EffortOffWire(EffortDeepSeek)
	wantOff := map[string]any{"thinking": map[string]any{"type": "disabled"}}
	if !reflect.DeepEqual(off, wantOff) {
		t.Fatalf("deepseek off = %v, want %v", off, wantOff)
	}
	// medium not in deepseek map → nil (deepseek does not passthrough)
	if got := TranslateEffort(EffortDeepSeek, "medium", lm); got != nil {
		t.Fatalf("deepseek medium should be nil (not offered), got %v", got)
	}
}

func TestTranslateEffortOpenRouter(t *testing.T) {
	lm := EffortLevelMap{"high": "high"}
	got := TranslateEffort(EffortOpenRouter, "high", lm)
	want := map[string]any{"reasoning": map[string]any{"effort": "high"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("openrouter high = %v, want %v", got, want)
	}
	// off → effort:"none"
	off := EffortOffWire(EffortOpenRouter)
	wantOff := map[string]any{"reasoning": map[string]any{"effort": "none"}}
	if !reflect.DeepEqual(off, wantOff) {
		t.Fatalf("openrouter off = %v, want %v", off, wantOff)
	}
}

func TestTranslateEffortQwen(t *testing.T) {
	lm := EffortLevelMap{"low": "low", "high": "high"}
	got := TranslateEffort(EffortQwen, "low", lm)
	want := map[string]any{"enable_thinking": true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("qwen low = %v, want %v", got, want)
	}
	if off := EffortOffWire(EffortQwen); !reflect.DeepEqual(off, map[string]any{"enable_thinking": false}) {
		t.Fatalf("qwen off = %v", off)
	}
}

func TestTranslateEffortGoogle(t *testing.T) {
	// gemini-3.1-pro map: low='LOW', high='HIGH' (uppercase wire values)
	lm := EffortLevelMap{"low": "LOW", "high": "HIGH"}
	got := TranslateEffort(EffortGoogle, "low", lm)
	want := map[string]any{"thinkingConfig": map[string]any{"thinkingLevel": "LOW"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("google low = %v, want %v", got, want)
	}
}

func TestTranslateEffortAnthropic(t *testing.T) {
	// claude-opus-4-7: xhigh/max
	lm := EffortLevelMap{"xhigh": "xhigh", "max": "max"}
	got := TranslateEffort(EffortAnthropic, "max", lm)
	want := map[string]any{
		"thinking": map[string]any{"type": "enabled"},
		"effort":   "max",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("anthropic max = %v, want %v", got, want)
	}
	// low not offered → nil
	if got := TranslateEffort(EffortAnthropic, "low", lm); got != nil {
		t.Fatalf("anthropic low should be nil, got %v", got)
	}
}

func TestTranslateEffortMistralBedrockString(t *testing.T) {
	lm := EffortLevelMap{"high": "high"}
	// mistral
	if got := TranslateEffort(EffortMistral, "high", lm); !reflect.DeepEqual(got, map[string]any{"reasoningEffort": "high"}) {
		t.Fatalf("mistral high = %v", got)
	}
	// bedrock
	if got := TranslateEffort(EffortBedrock, "high", lm); !reflect.DeepEqual(got, map[string]any{"output_config": map[string]any{"effort": "high"}}) {
		t.Fatalf("bedrock high = %v", got)
	}
	// string-thinking
	if got := TranslateEffort(EffortString, "high", lm); !reflect.DeepEqual(got, map[string]any{"thinking": "high"}) {
		t.Fatalf("string high = %v", got)
	}
	if off := EffortOffWire(EffortString); !reflect.DeepEqual(off, map[string]any{"thinking": "none"}) {
		t.Fatalf("string off = %v", off)
	}
}

func TestEffortOffWireNilFormats(t *testing.T) {
	// Formats without an explicit off wire → nil (absence is the off state).
	for _, f := range []EffortWireFormat{EffortOpenAI, EffortGoogle, EffortAnthropic, EffortMistral, EffortBedrock, EffortAntLing} {
		if got := EffortOffWire(f); got != nil {
			t.Fatalf("format %s off wire should be nil, got %v", f, got)
		}
	}
}
