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

	"github.com/inferglow/action"
	"github.com/inferglow/model"
	"github.com/inferglow/session"
)

// TestStreamRun_MissingNameField_NoPanic verifies that buildToolsFromActions
// (the helper used by StreamRun to convert action maps into ToolDefinitions)
// returns an error rather than panicking when an action map is missing the
// "name" key. Before the BUG-NEW-2 fix, the direct type assertion
// `act["name"].(string)` panicked with "interface conversion: interface is
// nil, not string".
//
// Note: ListActions() always returns well-formed maps because the Action
// struct enforces typed fields (Name string, Description string, Schema
// map[string]any). The malformed-map scenarios tested here exercise the
// defensive comma-ok pattern in buildToolsFromActions directly, since
// StreamRun delegates the conversion to this helper.
func TestStreamRun_MissingNameField_NoPanic(t *testing.T) {
	actions := []map[string]any{
		{
			"description": "a test action",
			"schema":      map[string]any{"type": "object"},
			// "name" key intentionally missing
		},
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("buildToolsFromActions panicked on missing 'name' field: %v", r)
		}
	}()

	tools, err := buildToolsFromActions(actions)
	if err == nil {
		t.Fatalf("expected error for missing 'name' field, got nil (tools=%v)", tools)
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("error message should contain 'name', got: %v", err)
	}
}

// TestStreamRun_NameFieldNotString_NoPanic verifies that buildToolsFromActions
// returns an error rather than panicking when the "name" field is present
// but is not a string (e.g., an int). Before the fix, the direct type
// assertion `act["name"].(string)` panicked with "interface conversion:
// interface is int, not string".
func TestStreamRun_NameFieldNotString_NoPanic(t *testing.T) {
	actions := []map[string]any{
		{
			"name":        42, // int, not string
			"description": "a test action",
			"schema":      map[string]any{"type": "object"},
		},
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("buildToolsFromActions panicked on non-string 'name' field: %v", r)
		}
	}()

	tools, err := buildToolsFromActions(actions)
	if err == nil {
		t.Fatalf("expected error for non-string 'name' field, got nil (tools=%v)", tools)
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("error message should contain 'name', got: %v", err)
	}
	if !strings.Contains(err.Error(), "int") {
		t.Errorf("error message should contain 'int' (the actual type), got: %v", err)
	}
}

// TestStreamRun_SchemaFieldNotMap_NoPanic verifies that buildToolsFromActions
// returns an error rather than panicking when the "schema" field is present
// but is not a map[string]any (e.g., a string). Before the fix, the direct
// type assertion `act["schema"].(map[string]any)` panicked with "interface
// conversion: interface is string, not map[string]interface {}".
func TestStreamRun_SchemaFieldNotMap_NoPanic(t *testing.T) {
	actions := []map[string]any{
		{
			"name":        "test_action",
			"description": "a test action",
			"schema":      "not-a-map", // string, not map[string]any
		},
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("buildToolsFromActions panicked on non-map 'schema' field: %v", r)
		}
	}()

	tools, err := buildToolsFromActions(actions)
	if err == nil {
		t.Fatalf("expected error for non-map 'schema' field, got nil (tools=%v)", tools)
	}
	if !strings.Contains(err.Error(), "schema") {
		t.Errorf("error message should contain 'schema', got: %v", err)
	}
}

// TestStreamRun_ValidActions_BehaviorUnchanged verifies that StreamRun
// continues to work correctly when all action fields have valid types.
// This is a regression test ensuring the BUG-NEW-2 fix (comma-ok pattern
// in buildToolsFromActions) does not break the happy path: StreamRun must
// still return a non-nil stream with no error.
func TestStreamRun_ValidActions_BehaviorUnchanged(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()

	// Register a valid action whose Name/Description/Schema fields are
	// all the correct types expected by buildToolsFromActions.
	actionInst, err := action.New("test_action", "A test action",
		func(ctx context.Context, input map[string]any) (any, error) {
			return "hello", nil
		})
	if err != nil {
		t.Fatalf("Failed to create action: %v", err)
	}
	if err := actExt.Register(actionInst); err != nil {
		t.Fatalf("Failed to register action: %v", err)
	}

	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 1)
			ch <- &model.StreamChunk{Delta: "ok", IsDone: true}
			close(ch)
			return ch, nil
		},
	}

	agent := New(sess, actExt, mockReq)
	stream, err := agent.StreamRun(context.Background(), "Hi")
	if err != nil {
		t.Fatalf("StreamRun returned error for valid actions: %v", err)
	}
	if stream == nil {
		t.Fatal("StreamRun returned nil stream for valid actions")
	}

	// Drain the stream to ensure it is usable end-to-end.
	for range stream {
	}
}
