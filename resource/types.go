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

package resource

import "time"

// Requirement describes what kind of execution resource is needed.
// The ResourceManager uses this to select an appropriate provider and
// to determine whether an existing handle can be reused.
type Requirement struct {
	// Type is the resource type (e.g. "bash", "python", "sqlite").
	Type string `json:"type"`

	// Scope groups resources for batch release (e.g. "execution-123",
	// "task-456"). Resources with the same scope can be released together.
	Scope string `json:"scope,omitempty"`

	// Properties carries additional matching criteria that providers
	// use to differentiate resource variants (e.g. "python_version": "3.12",
	// "database": "app.db").
	Properties map[string]string `json:"properties,omitempty"`

	// Capabilities lists the capabilities the resource must support
	// (e.g. "network", "filesystem", "gpu"). The provider's Capabilities()
	// must be a superset.
	Capabilities []string `json:"capabilities,omitempty"`
}

// ResourceConfig carries creation-time parameters for a Resource.
type ResourceConfig struct {
	// Type mirrors the Requirement.Type that triggered this config.
	Type string `json:"type"`

	// Properties are forwarded from Requirement.Properties.
	Properties map[string]string `json:"properties,omitempty"`

	// Timeout bounds resource creation. Zero means no timeout.
	Timeout time.Duration `json:"timeout,omitempty"`

	// MaxIdleTime bounds how long an idle resource stays alive before
	// the manager reclaims it. Zero means no automatic reclamation.
	MaxIdleTime time.Duration `json:"max_idle_time,omitempty"`
}

// ResourceStatus describes the current state of a resource handle.
type ResourceStatus struct {
	// HandleID is the handle's unique identifier.
	HandleID string `json:"handle_id"`

	// ResourceType is the underlying resource type.
	ResourceType string `json:"resource_type"`

	// State is one of: "creating", "ready", "busy", "idle", "closed", "failed".
	State string `json:"state"`

	// Scope is the handle's release scope.
	Scope string `json:"scope,omitempty"`

	// CreatedAt records when the resource was created.
	CreatedAt time.Time `json:"created_at"`

	// LastUsedAt records the most recent Execute call.
	LastUsedAt time.Time `json:"last_used_at,omitempty"`

	// UseCount tracks how many times Execute has been called.
	UseCount int64 `json:"use_count"`

	// Healthy reflects the result of the most recent HealthCheck.
	Healthy bool `json:"healthy"`

	// HealthError holds the last HealthCheck error, if any.
	HealthError string `json:"health_error,omitempty"`
}

// HandleState enumerates the lifecycle states of a ResourceHandle.
const (
	StateCreating = "creating"
	StateReady    = "ready"
	StateBusy     = "busy"
	StateIdle     = "idle"
	StateClosed   = "closed"
	StateFailed   = "failed"
)
