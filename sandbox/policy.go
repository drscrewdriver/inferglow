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

package sandbox

import "time"

// SandboxMode represents one of the 5 supported sandbox execution modes.
type SandboxMode string //nolint:revive

// SandboxMode values selecting a sandbox execution backend.
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

// IsolationLevel values describing the granularity of sandbox isolation.
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

// NetworkAccessLevel is the server-side enum describing how much network
// access a sandbox is granted. It is ordered from most to least restrictive:
//
//	none        → no network at all (NetworkDisabled)
//	egress_only → outbound traffic allowed, no inbound
//	full        → unrestricted network
type NetworkAccessLevel string

// NetworkAccessLevel values selecting the network access surface.
const (
	NetworkAccessNone       NetworkAccessLevel = "none"
	NetworkAccessEgressOnly NetworkAccessLevel = "egress_only"
	NetworkAccessFull       NetworkAccessLevel = "full"
)

// Rank returns the permissiveness of a level (0 = most restrictive).
// Unknown / empty values are treated as most restrictive (none) so the
// deny-by-default baseline cannot be widened by an unrecognised value.
func (l NetworkAccessLevel) Rank() int {
	switch l {
	case NetworkAccessFull:
		return 2
	case NetworkAccessEgressOnly:
		return 1
	default:
		return 0
	}
}

// MoreRestrictiveNetwork returns the more restrictive of two access levels.
// An empty b (LLM did not specify a value) yields a unchanged. An empty a is
// treated as NetworkAccessNone (deny-by-default).
func MoreRestrictiveNetwork(a, b NetworkAccessLevel) NetworkAccessLevel {
	if a == "" {
		a = NetworkAccessNone
	}
	if b == "" {
		return a
	}
	if a.Rank() <= b.Rank() {
		return a
	}
	return b
}

// NetworkPolicy describes the network access surface granted to a sandbox.
type NetworkPolicy struct {
	AllowInternet bool
	AllowedPorts  []int
	AllowedHosts  []string
	// Level is the authoritative network access level used by backends to
	// decide whether to disable networking. When left empty, backends
	// treat the policy as not requesting network isolation.
	Level NetworkAccessLevel
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

// ServerPolicyBaseline holds the server-side, deny-by-default caps for the
// fields an LLM is allowed to influence via the sandbox executor input map.
//
// The LLM-generated input may only TIGHTEN a policy relative to this
// baseline; it can never loosen it. A zero-value ServerPolicyBaseline is
// permissive for scalar caps (timeout / max output bytes) and for the path
// allowlist, but NetworkAccess is treated as NetworkAccessNone and
// ApprovalRequired as false when empty. Use DefaultDenyBaseline() to obtain
// the most restrictive baseline explicitly.
type ServerPolicyBaseline struct {
	NetworkAccess    NetworkAccessLevel
	PathAllowlist    []string
	MaxOutputBytes   int64
	Timeout          time.Duration
	ApprovalRequired bool
}

// DefaultDenyBaseline returns the most restrictive server-side baseline:
// no network, no allowed paths, a 1 MiB output cap, a 30s timeout, and
// approval required. Callers that need a less restrictive policy must
// configure the baseline explicitly.
func DefaultDenyBaseline() ServerPolicyBaseline {
	return ServerPolicyBaseline{
		NetworkAccess:    NetworkAccessNone,
		PathAllowlist:    nil,
		MaxOutputBytes:   1 << 20, // 1 MiB
		Timeout:          30 * time.Second,
		ApprovalRequired: true,
	}
}

// IsZero reports whether the baseline is the unset zero value. It is used by
// the executor to fall back to DefaultDenyBaseline() when no baseline was
// configured, preserving deny-by-default semantics.
func (b ServerPolicyBaseline) IsZero() bool {
	return b.NetworkAccess == "" &&
		b.Timeout == 0 &&
		b.MaxOutputBytes == 0 &&
		len(b.PathAllowlist) == 0 &&
		!b.ApprovalRequired
}
