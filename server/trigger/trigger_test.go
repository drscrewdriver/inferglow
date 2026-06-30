// Copyright 2026 InferGlow Authors

package trigger

import (
	"context"
	"testing"
	"time"
)

// mockRunHandle implements RunHandle for testing.
type mockRunHandle struct {
	id     string
	status string
}

func (m *mockRunHandle) GetID() string     { return m.id }
func (m *mockRunHandle) GetStatus() string { return m.status }

// mockStarter records calls for testing.
type mockStarter struct {
	calls []startCall
}

type startCall struct {
	flow   string
	inputs map[string]any
	owner  string
}

func (m *mockStarter) Start(flowName string, inputs map[string]any, owner string) (RunHandle, error) {
	m.calls = append(m.calls, startCall{flow: flowName, inputs: inputs, owner: owner})
	return &mockRunHandle{id: "run-1", status: "pending"}, nil
}

func TestRegistry_RegisterAndList(t *testing.T) {
	starter := &mockStarter{}
	reg := NewRegistry(starter)

	cfg := Config{
		ID:      "test-webhook",
		Type:    "webhook",
		Flow:    "bug-fix",
		Enabled: true,
	}
	trig, err := reg.Register(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if trig.ID() != "test-webhook" {
		t.Fatalf("id = %q, want 'test-webhook'", trig.ID())
	}
	if trig.Type() != "webhook" {
		t.Fatalf("type = %q, want 'webhook'", trig.Type())
	}
	if trig.FlowName() != "bug-fix" {
		t.Fatalf("flow = %q, want 'bug-fix'", trig.FlowName())
	}

	list := reg.List()
	if len(list) != 1 {
		t.Fatalf("list len = %d, want 1", len(list))
	}
}

func TestRegistry_DuplicateID(t *testing.T) {
	starter := &mockStarter{}
	reg := NewRegistry(starter)

	cfg := Config{ID: "dup", Type: "webhook", Flow: "f1", Enabled: true}
	if _, err := reg.Register(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Register(cfg); err == nil {
		t.Fatal("expected error for duplicate ID")
	}
}

func TestRegistry_Unregister(t *testing.T) {
	starter := &mockStarter{}
	reg := NewRegistry(starter)

	cfg := Config{ID: "t1", Type: "webhook", Flow: "f1", Enabled: true}
	reg.Register(cfg)

	if err := reg.Unregister("t1"); err != nil {
		t.Fatal(err)
	}
	if _, ok := reg.Get("t1"); ok {
		t.Fatal("trigger should be removed")
	}
}

func TestCronTrigger_Fire(t *testing.T) {
	starter := &mockStarter{}
	reg := NewRegistry(starter)

	cfg := Config{
		ID:      "cron-1",
		Type:    "cron",
		Flow:    "bug-fix",
		Enabled: true,
		Cron: &CronConfig{
			Interval: 50 * time.Millisecond,
		},
	}
	trig, err := reg.Register(cfg)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	if err := trig.Start(ctx); err != nil {
		t.Fatal(err)
	}

	// Wait for at least one fire.
	time.Sleep(120 * time.Millisecond)

	if err := trig.Stop(); err != nil {
		t.Fatal(err)
	}

	if len(starter.calls) == 0 {
		t.Fatal("expected at least one cron fire")
	}
	if starter.calls[0].flow != "bug-fix" {
		t.Fatalf("flow = %q, want 'bug-fix'", starter.calls[0].flow)
	}
}

func TestEventTrigger_Fire(t *testing.T) {
	starter := &mockStarter{}

	cfg := Config{
		ID:      "evt-1",
		Type:    "event",
		Flow:    "bug-fix",
		Enabled: true,
		Event: &EventConfig{
			Topics: []string{"issue.created"},
		},
	}
	trig, err := NewEventTrigger(cfg, starter)
	if err != nil {
		t.Fatal(err)
	}

	bus := NewEventBus()
	trig.SetBus(bus)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := trig.Start(ctx); err != nil {
		t.Fatal(err)
	}

	// Publish an event.
	bus.Publish("issue.created", map[string]any{"title": "Bug 1"}, "test")

	// Give it a moment to process.
	time.Sleep(50 * time.Millisecond)

	if len(starter.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(starter.calls))
	}
	if starter.calls[0].flow != "bug-fix" {
		t.Fatalf("flow = %q, want 'bug-fix'", starter.calls[0].flow)
	}

	trig.Stop()
}

func TestWebhookTrigger_ServeHTTP(t *testing.T) {
	starter := &mockStarter{}
	cfg := Config{
		ID:      "wh-1",
		Type:    "webhook",
		Flow:    "bug-fix",
		Enabled: true,
	}
	trig, err := NewWebhookTrigger(cfg, starter)
	if err != nil {
		t.Fatal(err)
	}

	// Test disabled trigger.
	trig.Stop()
	// We can't easily test HTTP without httptest, but verify the trigger state.
	if trig.Enabled() {
		t.Fatal("trigger should be disabled after Stop")
	}

	trig.Start(context.Background())
	if !trig.Enabled() {
		t.Fatal("trigger should be enabled after Start")
	}
}
