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
