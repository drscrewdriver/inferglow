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

package model

import (
	"context"
	"testing"
)

// TestGenerateRequestData_JSONSchemaMode verifies that when the caller requests
// force_json AND supplies an OutputSchema with non-empty Properties, the
// provider emits a response_format of type "json_schema" with strict:true and
// an embedded schema. vLLM/SGLang XGrammar and OpenAI structured output both
// consume this format for token-level constrained decoding.
func TestGenerateRequestData_JSONSchemaMode(t *testing.T) {
	provider := &OpenAICompatibleProvider{Model: "gpt-4"}

	req := &ModelRequest{
		Input:   "test",
		Options: map[string]any{"force_json": true},
		Output: &OutputSchema{
			Type:       "object",
			Properties: map[string]any{"name": map[string]any{"type": "string"}},
			Required:   []string{"name"},
		},
	}

	data, err := provider.GenerateRequestData(context.Background(), req)
	if err != nil {
		t.Fatalf("GenerateRequestData failed: %v", err)
	}

	rf, ok := data.Options["response_format"]
	if !ok {
		t.Fatal("expected response_format in data.Options")
	}
	rfMap, ok := rf.(map[string]any)
	if !ok {
		t.Fatalf("response_format should be a map, got %T", rf)
	}
	if rfMap["type"] != "json_schema" {
		t.Errorf("response_format.type = %v, want \"json_schema\"", rfMap["type"])
	}
	js, ok := rfMap["json_schema"]
	if !ok {
		t.Fatal("expected json_schema field in response_format")
	}
	jsMap, ok := js.(map[string]any)
	if !ok {
		t.Fatalf("json_schema should be a map, got %T", js)
	}
	if jsMap["strict"] != true {
		t.Errorf("json_schema.strict = %v, want true", jsMap["strict"])
	}
	if jsMap["schema"] == nil {
		t.Error("json_schema.schema must not be nil")
	}
}

// TestGenerateRequestData_JSONObjectFallback verifies that when force_json is
// set but no OutputSchema (or one without Properties) is provided, the
// provider degrades to {"type":"json_object"}.
func TestGenerateRequestData_JSONObjectFallback(t *testing.T) {
	provider := &OpenAICompatibleProvider{Model: "gpt-4"}

	req := &ModelRequest{
		Input:   "test",
		Options: map[string]any{"force_json": true},
		Output:  nil,
	}

	data, err := provider.GenerateRequestData(context.Background(), req)
	if err != nil {
		t.Fatalf("GenerateRequestData failed: %v", err)
	}

	rf, ok := data.Options["response_format"]
	if !ok {
		t.Fatal("expected response_format in data.Options")
	}
	rfMap, ok := rf.(map[string]any)
	if !ok {
		t.Fatalf("response_format should be a map, got %T", rf)
	}
	if rfMap["type"] != "json_object" {
		t.Errorf("response_format.type = %v, want \"json_object\"", rfMap["type"])
	}
}

// TestGenerateRequestData_ForcedJSONObjectMode verifies that when the caller
// explicitly sets response_format_mode=json_object, the provider degrades to
// json_object even if Output.Properties is non-empty.
func TestGenerateRequestData_ForcedJSONObjectMode(t *testing.T) {
	provider := &OpenAICompatibleProvider{Model: "gpt-4"}

	req := &ModelRequest{
		Input: "test",
		Options: map[string]any{
			"force_json":            true,
			"response_format_mode": "json_object",
		},
		Output: &OutputSchema{
			Type:       "object",
			Properties: map[string]any{"name": map[string]any{"type": "string"}},
		},
	}

	data, err := provider.GenerateRequestData(context.Background(), req)
	if err != nil {
		t.Fatalf("GenerateRequestData failed: %v", err)
	}

	rf, ok := data.Options["response_format"]
	if !ok {
		t.Fatal("expected response_format in data.Options")
	}
	rfMap, ok := rf.(map[string]any)
	if !ok {
		t.Fatalf("response_format should be a map, got %T", rf)
	}
	if rfMap["type"] != "json_object" {
		t.Errorf("response_format.type = %v, want \"json_object\" (forced degradation)", rfMap["type"])
	}
}
