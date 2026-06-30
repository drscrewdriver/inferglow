// Copyright 2026 InferGlow Authors

package trigger

import "errors"

// Sentinel errors for trigger validation.
var (
	ErrMissingFlow         = errors.New("trigger requires a flow name")
	ErrMissingCronConfig   = errors.New("cron trigger requires cron configuration")
	ErrMissingEventTopics  = errors.New("event trigger requires at least one topic")
)
