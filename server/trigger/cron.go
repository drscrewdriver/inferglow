// Copyright 2026 InferGlow Authors

package trigger

import (
	"context"
	"sync"
	"time"
)

// CronTrigger creates runs on a schedule.
type CronTrigger struct {
	cfg      Config
	starter  RunStarter
	enabled  bool
	interval time.Duration
	cancel   context.CancelFunc
	mu       sync.Mutex
}

// NewCronTrigger creates a cron trigger from config.
func NewCronTrigger(cfg Config, starter RunStarter) (*CronTrigger, error) {
	if cfg.Flow == "" {
		return nil, ErrMissingFlow
	}
	if cfg.Cron == nil {
		return nil, ErrMissingCronConfig
	}

	interval := cfg.Cron.Interval
	if interval == 0 {
		// Default to 1 hour if no interval specified.
		interval = time.Hour
	}

	return &CronTrigger{
		cfg:      cfg,
		starter:  starter,
		enabled:  cfg.Enabled,
		interval: interval,
	}, nil
}

func (c *CronTrigger) ID() string       { return c.cfg.ID }
func (c *CronTrigger) Type() string     { return "cron" }
func (c *CronTrigger) FlowName() string { return c.cfg.Flow }
func (c *CronTrigger) Enabled() bool    { return c.enabled }

// Start begins the cron schedule loop.
func (c *CronTrigger) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.cancel != nil {
		c.mu.Unlock()
		return nil // already running
	}
	ctx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	c.enabled = true
	c.mu.Unlock()

	go c.run(ctx)
	return nil
}

// Stop halts the cron schedule.
func (c *CronTrigger) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.enabled = false
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	return nil
}

// run is the main cron loop.
func (c *CronTrigger) run(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.fire(ctx)
		}
	}
}

// fire creates a new run.
func (c *CronTrigger) fire(ctx context.Context) {
	inputs := make(map[string]any)

	// Apply default inputs.
	if c.cfg.Defaults != nil {
		for k, v := range c.cfg.Defaults {
			inputs[k] = v
		}
	}
	if c.cfg.Cron.Inputs != nil {
		for k, v := range c.cfg.Cron.Inputs {
			inputs[k] = v
		}
	}

	// Add cron metadata.
	inputs["_trigger"] = map[string]any{
		"type":       "cron",
		"trigger_id": c.cfg.ID,
		"fired_at":   time.Now().UTC().Format(time.RFC3339),
	}

	_, _ = c.starter.Start(c.cfg.Flow, inputs, "trigger:"+c.cfg.ID)
}

// Interval returns the cron interval (for API inspection).
func (c *CronTrigger) Interval() time.Duration {
	return c.interval
}
