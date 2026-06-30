// Copyright 2026 InferGlow Authors

package trigger

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Event represents an internal event that can trigger flows.
type Event struct {
	Topic     string         `json:"topic"`
	Data      map[string]any `json:"data,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
	Source    string         `json:"source,omitempty"`
}

// EventBus is a simple pub/sub for internal events.
type EventBus struct {
	mu          sync.RWMutex
	subscribers map[string][]chan Event
}

// NewEventBus creates a new event bus.
func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[string][]chan Event),
	}
}

// Publish sends an event to all subscribers of that topic.
func (eb *EventBus) Publish(topic string, data map[string]any, source string) {
	ev := Event{
		Topic:     topic,
		Data:      data,
		Timestamp: time.Now(),
		Source:    source,
	}

	eb.mu.RLock()
	subs := eb.subscribers[topic]
	eb.mu.RUnlock()

	for _, ch := range subs {
		select {
		case ch <- ev:
		default:
			// Drop if subscriber is slow.
		}
	}
}

// Subscribe returns a channel that receives events for the given topic.
func (eb *EventBus) Subscribe(topic string) chan Event {
	ch := make(chan Event, 32)
	eb.mu.Lock()
	eb.subscribers[topic] = append(eb.subscribers[topic], ch)
	eb.mu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber channel.
func (eb *EventBus) Unsubscribe(topic string, ch chan Event) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	subs := eb.subscribers[topic]
	for i, s := range subs {
		if s == ch {
			eb.subscribers[topic] = append(subs[:i], subs[i+1:]...)
			close(ch)
			return
		}
	}
}

// EventTrigger creates runs when events are published to subscribed topics.
type EventTrigger struct {
	cfg     Config
	starter RunStarter
	bus     *EventBus
	enabled bool
	chans   []chan Event
	cancel  context.CancelFunc
	mu      sync.Mutex
}

// NewEventTrigger creates an event trigger from config.
func NewEventTrigger(cfg Config, starter RunStarter) (*EventTrigger, error) {
	if cfg.Flow == "" {
		return nil, ErrMissingFlow
	}
	if cfg.Event == nil || len(cfg.Event.Topics) == 0 {
		return nil, ErrMissingEventTopics
	}
	return &EventTrigger{
		cfg:     cfg,
		starter: starter,
		enabled: cfg.Enabled,
	}, nil
}

func (e *EventTrigger) ID() string       { return e.cfg.ID }
func (e *EventTrigger) Type() string     { return "event" }
func (e *EventTrigger) FlowName() string { return e.cfg.Flow }
func (e *EventTrigger) Enabled() bool    { return e.enabled }

// SetBus sets the event bus for this trigger.
func (e *EventTrigger) SetBus(bus *EventBus) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.bus = bus
}

// Start subscribes to events and begins processing.
func (e *EventTrigger) Start(ctx context.Context) error {
	e.mu.Lock()
	if e.bus == nil {
		e.mu.Unlock()
		return fmt.Errorf("event trigger %q: no event bus set", e.cfg.ID)
	}
	ctx, cancel := context.WithCancel(ctx)
	e.cancel = cancel
	e.enabled = true

	// Subscribe to all configured topics.
	for _, topic := range e.cfg.Event.Topics {
		ch := e.bus.Subscribe(topic)
		e.chans = append(e.chans, ch)
		go e.listen(ctx, ch)
	}
	e.mu.Unlock()
	return nil
}

// Stop unsubscribes from all topics.
func (e *EventTrigger) Stop() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.enabled = false
	if e.cancel != nil {
		e.cancel()
		e.cancel = nil
	}
	if e.bus != nil {
		for i, topic := range e.cfg.Event.Topics {
			if i < len(e.chans) {
				e.bus.Unsubscribe(topic, e.chans[i])
			}
		}
	}
	e.chans = nil
	return nil
}

// listen processes incoming events.
func (e *EventTrigger) listen(ctx context.Context, ch chan Event) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			e.fire(ctx, ev)
		}
	}
}

// fire creates a run from an event.
func (e *EventTrigger) fire(ctx context.Context, ev Event) {
	inputs := make(map[string]any)

	// Apply default inputs.
	if e.cfg.Defaults != nil {
		for k, v := range e.cfg.Defaults {
			inputs[k] = v
		}
	}
	if e.cfg.Event.Inputs != nil {
		for k, v := range e.cfg.Event.Inputs {
			inputs[k] = v
		}
	}

	// Inject event data.
	inputs["_event"] = map[string]any{
		"topic":     ev.Topic,
		"data":      ev.Data,
		"source":    ev.Source,
		"timestamp": ev.Timestamp.Format(time.RFC3339),
	}
	inputs["_trigger"] = map[string]any{
		"type":       "event",
		"trigger_id": e.cfg.ID,
	}

	_, _ = e.starter.Start(e.cfg.Flow, inputs, "trigger:"+e.cfg.ID)
}
