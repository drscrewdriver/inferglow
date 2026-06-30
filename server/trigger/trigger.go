// Copyright 2026 InferGlow Authors

// Package trigger provides external trigger mechanisms for automatically
// executing flow runs. Three trigger types are supported:
//
//   - WebhookTrigger: HTTP endpoint that creates a run when called
//   - CronTrigger: Schedule-based periodic run creation
//   - EventTrigger: Internal event bus subscription that creates runs
//
// All triggers use a RunStarter interface to create runs, allowing them
// to work with any run management system.
package trigger

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// RunStarter is the interface needed to create a new run.
// RunManager implements this interface.
type RunStarter interface {
	Start(flowName string, inputs map[string]any, owner string) (RunHandle, error)
}

// RunHandle is a minimal handle for a running flow execution.
type RunHandle interface {
	GetID() string
	GetStatus() string
}

// StarterFunc adapts a plain function to the RunStarter interface.
type StarterFunc func(flowName string, inputs map[string]any, owner string) (RunHandle, error)

func (f StarterFunc) Start(flowName string, inputs map[string]any, owner string) (RunHandle, error) {
	return f(flowName, inputs, owner)
}

// Trigger is the common interface for all trigger types.
type Trigger interface {
	// ID returns the unique identifier of this trigger.
	ID() string
	// Type returns the trigger type (webhook, cron, event).
	Type() string
	// FlowName returns the name of the flow this trigger is bound to.
	FlowName() string
	// Start activates the trigger.
	Start(ctx context.Context) error
	// Stop deactivates the trigger.
	Stop() error
	// Enabled returns whether the trigger is currently active.
	Enabled() bool
}

// Config holds the configuration for a trigger.
type Config struct {
	ID       string         `json:"id" yaml:"id"`
	Type     string         `json:"type" yaml:"type"` // webhook, cron, event
	Flow     string         `json:"flow" yaml:"flow"`
	Enabled  bool           `json:"enabled" yaml:"enabled"`
	Cron     *CronConfig    `json:"cron,omitempty" yaml:"cron,omitempty"`
	Event    *EventConfig   `json:"event,omitempty" yaml:"event,omitempty"`
	Webhook  *WebhookConfig `json:"webhook,omitempty" yaml:"webhook,omitempty"`
	Defaults map[string]any `json:"defaults,omitempty" yaml:"defaults,omitempty"`
}

// CronConfig holds cron trigger configuration.
type CronConfig struct {
	Expr     string         `json:"expr" yaml:"expr"` // cron expression or interval
	Interval time.Duration  `json:"interval,omitempty" yaml:"interval,omitempty"`
	Inputs   map[string]any `json:"inputs,omitempty" yaml:"inputs,omitempty"`
}

// EventConfig holds event trigger configuration.
type EventConfig struct {
	Topics []string       `json:"topics" yaml:"topics"` // event topics to subscribe
	Filter string         `json:"filter,omitempty" yaml:"filter,omitempty"` // CEL/JS filter expr
	Inputs map[string]any `json:"inputs,omitempty" yaml:"inputs,omitempty"`
}

// WebhookConfig holds webhook trigger configuration.
type WebhookConfig struct {
	Path    string         `json:"path,omitempty" yaml:"path,omitempty"` // custom path suffix
	Inputs  map[string]any `json:"inputs,omitempty" yaml:"inputs,omitempty"`
	Secret  string         `json:"secret,omitempty" yaml:"secret,omitempty"` // HMAC verification
}

// Registry manages all triggers.
type Registry struct {
	mu       sync.RWMutex
	triggers map[string]Trigger
	starter  RunStarter
}

// NewRegistry creates a trigger registry with the given run starter.
func NewRegistry(starter RunStarter) *Registry {
	return &Registry{
		triggers: make(map[string]Trigger),
		starter:  starter,
	}
}

// Register creates and registers a trigger from config.
func (r *Registry) Register(cfg Config) (Trigger, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.triggers[cfg.ID]; exists {
		return nil, fmt.Errorf("trigger %q already exists", cfg.ID)
	}

	var t Trigger
	var err error

	switch cfg.Type {
	case "webhook":
		t, err = NewWebhookTrigger(cfg, r.starter)
	case "cron":
		t, err = NewCronTrigger(cfg, r.starter)
	case "event":
		t, err = NewEventTrigger(cfg, r.starter)
	default:
		return nil, fmt.Errorf("unknown trigger type: %s", cfg.Type)
	}

	if err != nil {
		return nil, err
	}

	r.triggers[cfg.ID] = t
	return t, nil
}

// Unregister removes and stops a trigger.
func (r *Registry) Unregister(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	t, ok := r.triggers[id]
	if !ok {
		return fmt.Errorf("trigger %q not found", id)
	}

	if err := t.Stop(); err != nil {
		return err
	}

	delete(r.triggers, id)
	return nil
}

// Get returns a trigger by ID.
func (r *Registry) Get(id string) (Trigger, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.triggers[id]
	return t, ok
}

// List returns all registered triggers.
func (r *Registry) List() []Trigger {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Trigger, 0, len(r.triggers))
	for _, t := range r.triggers {
		result = append(result, t)
	}
	return result
}

// StartAll starts all enabled triggers.
func (r *Registry) StartAll(ctx context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, t := range r.triggers {
		if t.Enabled() {
			if err := t.Start(ctx); err != nil {
				return fmt.Errorf("start trigger %s: %w", t.ID(), err)
			}
		}
	}
	return nil
}

// StopAll stops all triggers.
func (r *Registry) StopAll() error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, t := range r.triggers {
		if err := t.Stop(); err != nil {
			return fmt.Errorf("stop trigger %s: %w", t.ID(), err)
		}
	}
	return nil
}
