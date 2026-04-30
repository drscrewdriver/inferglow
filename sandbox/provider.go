package sandbox

import (
	"context"
	"errors"
	"io"
	"time"
)

// Provider is the contract every sandbox backend (docker, gvisor, seatbelt,
// bubblewrap, landlock, windows_runtime, e2b, trusted_local) must satisfy.
type Provider interface {
	Name() string
	Kind() string
	InspectAvailability() (*AvailabilityResult, error)
	CreateHandle(cfg map[string]any, policy *ExecutionPolicy) (Handle, error)
}

// Handle is a long-lived sandbox instance that can start, execute commands,
// and stop. Implementations must be safe for concurrent use after Start.
type Handle interface {
	Start(ctx context.Context) error
	Execute(ctx context.Context, cmd *Command) (*ExecutionResult, error)
	Stop(ctx context.Context) error
	Status() HandleStatus
}

// AvailabilityResult reports whether a Provider is usable on the current
// host and, if so, where its binary lives and what version it reports.
type AvailabilityResult struct {
	Available    bool
	Platform     string
	BinaryPath   string
	Version      string
	ErrorMessage string
}

// HandleStatus is the lifecycle state of a Handle.
type HandleStatus string

const (
	StatusCreated HandleStatus = "created"
	StatusRunning HandleStatus = "running"
	StatusStopped HandleStatus = "stopped"
	StatusError   HandleStatus = "error"
)

// Command is a single process invocation to run inside a sandbox Handle.
type Command struct {
	Argv    []string
	Env     []string
	Workdir string
	Stdin   io.Reader
}

// ExecutionResult is the outcome of running a Command inside a Handle.
type ExecutionResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Duration time.Duration
}

// Sentinel errors returned by the sandbox framework.
var (
	ErrProviderNotFound    = errors.New("provider not found")
	ErrNoAvailableSandbox  = errors.New("no available sandbox")
	ErrProviderUnavailable = errors.New("provider unavailable")
	ErrHandleNotRunning    = errors.New("handle not running")
)
