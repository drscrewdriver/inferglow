// Copyright 2026 InferGlow Authors

package cli

import (
	"context"
	"testing"

	"github.com/inferglow/model"
)

// TestThinkingInjectRequester — enable_thinking injection with caller-key
// precedence, mirroring the server-side optionsInjectRequester test.
func TestThinkingInjectRequester(t *testing.T) {
	cfg := DefaultCLIConfig()
	cfg.LLM = LLMConfig{Endpoint: "http://127.0.0.1:1", Model: "m", APIKey: "k", Provider: "openai"}
	inner, err := buildModelRequester(cfg)
	if err != nil {
		t.Fatal(err)
	}
	wrapped := &thinkingInjectRequester{inner: inner}

	d, err := wrapped.GenerateRequestData(context.Background(), &model.ModelRequest{Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	kwargs, ok := d.Options["chat_template_kwargs"].(map[string]any)
	if !ok || kwargs["enable_thinking"] != true {
		t.Fatalf("chat_template_kwargs missing: %+v", d.Options)
	}

	d2, err := wrapped.GenerateRequestData(context.Background(), &model.ModelRequest{
		Model:   "m",
		Options: map[string]any{"chat_template_kwargs": "caller-set"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if d2.Options["chat_template_kwargs"] != "caller-set" {
		t.Fatalf("caller key overwritten: %+v", d2.Options)
	}
}
