package sandbox

import "time"

// SandboxMode represents one of the 5 supported sandbox execution modes.
type SandboxMode string

const (
	ModeTrustedLocal SandboxMode = "trusted_local"
	ModeLocal        SandboxMode = "local"
	ModeDocker       SandboxMode = "docker"
	ModeGVisor       SandboxMode = "gvisor"
	ModeAuto         SandboxMode = "auto"
)

// String returns the string form of the sandbox mode.
func (m SandboxMode) String() string { return string(m) }

// IsolationLevel represents the granularity of process isolation provided
// by a sandbox provider.
type IsolationLevel string

const (
	LevelProcess   IsolationLevel = "process"
	LevelContainer IsolationLevel = "container"
	LevelVM        IsolationLevel = "vm"
)

// ResourceLimit constrains CPU, memory, disk and process-count usage
// inside a sandbox.
type ResourceLimit struct {
	CPUShares   int64
	MemoryBytes int64
	DiskBytes   int64
	NPROC       int
}

// NetworkPolicy describes the network access surface granted to a sandbox.
type NetworkPolicy struct {
	AllowInternet bool
	AllowedPorts  []int
	AllowedHosts  []string
}

// MountEntry describes a single bind mount into the sandbox filesystem.
type MountEntry struct {
	Source      string
	Destination string
	ReadOnly    bool
}

// FilesystemPolicy describes filesystem-level restrictions for a sandbox.
type FilesystemPolicy struct {
	ReadOnlyRoot bool
	Mounts       []MountEntry
	AllowedPaths []string
	DeniedPaths  []string
}

// ExecutionPolicy is the complete description of how a sandbox should be
// configured and executed.
type ExecutionPolicy struct {
	SandboxMode      SandboxMode
	ResourceLimit    ResourceLimit
	NetworkAccess    NetworkPolicy
	FilesystemAccess FilesystemPolicy
	AllowedCommands  []string
	Timeout          time.Duration
	IsolationLevel   IsolationLevel
}

// DefaultPolicy returns the most permissive default policy: a trusted-local
// sandbox running at process isolation level, with all other fields zero.
func DefaultPolicy() ExecutionPolicy {
	return ExecutionPolicy{
		SandboxMode:    ModeTrustedLocal,
		IsolationLevel: LevelProcess,
	}
}
