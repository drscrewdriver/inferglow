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

package model

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

// AllProvidersFailedError is returned by FailoverModelRequester when every
// provider in the priority list has failed. The Errors slice contains the
// error returned by each provider that was attempted, in priority order.
type AllProvidersFailedError struct {
	Errors []error // errors from each provider, in priority order
}

// Error implements the error interface. It returns "all providers failed"
// followed by the concatenated messages of all underlying errors.
func (e *AllProvidersFailedError) Error() string {
	if e == nil || len(e.Errors) == 0 {
		return "all providers failed"
	}
	parts := make([]string, 0, len(e.Errors))
	for _, err := range e.Errors {
		if err != nil {
			parts = append(parts, err.Error())
		}
	}
	if len(parts) == 0 {
		return "all providers failed"
	}
	return "all providers failed: " + strings.Join(parts, "; ")
}

// Unwrap returns the underlying errors, supporting Go 1.20+ multi-error
// unwrapping so that errors.Is and errors.As traverse every provider error.
func (e *AllProvidersFailedError) Unwrap() []error {
	if e == nil {
		return nil
	}
	return e.Errors
}

// FailoverConfig configures the failover behavior of FailoverModelRequester.
// A zero-value FailoverConfig uses the defaults (MaxFailures=3,
// CooldownDuration=30s).
type FailoverConfig struct {
	// MaxFailures is the number of consecutive failures that triggers a
	// cooldown for a provider (default 3). When a provider's failCount
	// reaches MaxFailures it is placed in cooldown for CooldownDuration.
	MaxFailures int
	// CooldownDuration is how long a provider is skipped after entering
	// cooldown (default 30s). After the cooldown expires the provider is
	// eligible again and its failure count is reset on the next request.
	CooldownDuration time.Duration
}

// Default failover parameters.
const (
	DefaultFailoverMaxFailures = 3
	DefaultFailoverCooldown    = 30 * time.Second
)

// modelProviderEntry tracks the runtime health of a single provider within
// the FailoverModelRequester. All fields are protected by the parent's mutex.
type modelProviderEntry struct {
	requester   ModelRequester
	name        string
	failCount   int       // consecutive failure count
	lastFailAt  time.Time // time of last failure
	cooldownEnd time.Time // time until which this provider is skipped
	healthy     bool      // false while in cooldown
}

// FailoverModelRequester wraps multiple ModelRequester instances and
// automatically fails over to the next provider when the current one fails.
// Providers are ordered by priority (index 0 = highest priority).
//
// When a provider fails, its consecutive failure count is incremented. Once
// the count reaches MaxFailures the provider enters a cooldown period during
// which it is skipped. After the cooldown expires the provider is eligible
// again: the next request resets its failure count and healthy state before
// retrying it. A successful call also resets the provider's health.
//
// All methods are safe for concurrent use.
type FailoverModelRequester struct {
	providers []modelProviderEntry // ordered by priority (index 0 = highest)
	mu        sync.RWMutex
	config    FailoverConfig
	lastIndex int // index of last successfully used provider (-1 = none)
}

// ProviderStatus is a snapshot of a provider's health within the
// FailoverModelRequester, returned by GetProviderStatus.
type ProviderStatus struct {
	Name        string
	Healthy     bool
	FailCount   int
	CooldownEnd time.Time // zero value means not in cooldown
}

// normalizeFailoverConfig fills in defaults for zero-value fields.
func normalizeFailoverConfig(config FailoverConfig) FailoverConfig {
	if config.MaxFailures <= 0 {
		config.MaxFailures = DefaultFailoverMaxFailures
	}
	if config.CooldownDuration <= 0 {
		config.CooldownDuration = DefaultFailoverCooldown
	}
	return config
}

// NewFailoverModelRequester creates a FailoverModelRequester that wraps the
// given providers in priority order (index 0 = highest priority). The
// providers slice is copied to avoid external mutation. A zero-value config
// uses the defaults (MaxFailures=3, CooldownDuration=30s). Provider names are
// taken from each provider's Name() method.
func NewFailoverModelRequester(providers []ModelRequester, config FailoverConfig) *FailoverModelRequester {
	config = normalizeFailoverConfig(config)
	entries := make([]modelProviderEntry, len(providers))
	for i, p := range providers {
		name := ""
		if p != nil {
			name = p.Name()
		}
		entries[i] = modelProviderEntry{
			requester: p,
			name:      name,
			healthy:   true,
		}
	}
	return &FailoverModelRequester{
		providers: entries,
		config:    config,
		lastIndex: -1,
	}
}

// NewFailoverModelRequesterNamed creates a FailoverModelRequester from a map
// of named providers. Providers are ordered alphabetically by name for
// deterministic priority ordering. The map key is used as the provider's
// authoritative name (overriding the provider's own Name()).
func NewFailoverModelRequesterNamed(providers map[string]ModelRequester, config FailoverConfig) *FailoverModelRequester {
	config = normalizeFailoverConfig(config)
	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	sort.Strings(names)
	entries := make([]modelProviderEntry, len(names))
	for i, name := range names {
		entries[i] = modelProviderEntry{
			requester: providers[name],
			name:      name,
			healthy:   true,
		}
	}
	return &FailoverModelRequester{
		providers: entries,
		config:    config,
		lastIndex: -1,
	}
}

// Name returns "failover".
func (f *FailoverModelRequester) Name() string {
	return "failover"
}

// GenerateRequestData implements ModelRequester. It tries providers in
// priority order, skipping any that are in cooldown. The first provider to
// succeed wins; its health is reset and its index is recorded for subsequent
// BroadcastResponse delegation. If all available providers fail,
// AllProvidersFailedError is returned.
func (f *FailoverModelRequester) GenerateRequestData(ctx context.Context, req *ModelRequest) (*RequestData, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	available := f.availableIndices()
	if len(available) == 0 {
		return nil, &AllProvidersFailedError{}
	}
	var errs []error
	for _, idx := range available {
		f.tryRecover(idx)
		data, err := f.providers[idx].requester.GenerateRequestData(ctx, req)
		if err != nil {
			f.recordFailure(idx)
			errs = append(errs, err)
			continue
		}
		f.recordSuccess(idx)
		f.setLastIndex(idx)
		return data, nil
	}
	return nil, &AllProvidersFailedError{Errors: errs}
}

// RequestModel implements ModelRequester. It tries providers in priority
// order, skipping any that are in cooldown. The first provider to succeed
// wins; its health is reset and its index is recorded for subsequent
// BroadcastResponse delegation. If all available providers fail,
// AllProvidersFailedError is returned.
func (f *FailoverModelRequester) RequestModel(ctx context.Context, data *RequestData) (<-chan *StreamChunk, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	available := f.availableIndices()
	if len(available) == 0 {
		return nil, &AllProvidersFailedError{}
	}
	var errs []error
	for _, idx := range available {
		f.tryRecover(idx)
		stream, err := f.providers[idx].requester.RequestModel(ctx, data)
		if err != nil {
			f.recordFailure(idx)
			errs = append(errs, err)
			continue
		}
		f.recordSuccess(idx)
		f.setLastIndex(idx)
		return stream, nil
	}
	return nil, &AllProvidersFailedError{Errors: errs}
}

// BroadcastResponse delegates to the provider that was most recently used
// successfully (tracked via lastIndex). If no provider has succeeded yet,
// it delegates to the first available provider. If no provider is available
// it returns AllProvidersFailedError.
func (f *FailoverModelRequester) BroadcastResponse(ctx context.Context, stream <-chan *StreamChunk) (<-chan *ResultEvent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	idx := f.getLastIndex()
	if idx < 0 {
		available := f.availableIndices()
		if len(available) == 0 {
			return nil, &AllProvidersFailedError{}
		}
		idx = available[0]
	}
	return f.providers[idx].requester.BroadcastResponse(ctx, stream)
}

// === health inspection ===

// IsHealthy reports whether the named provider is not currently in cooldown.
// A provider whose cooldown has expired (even if not yet reset by a request)
// is considered healthy. Returns false for unknown providers.
func (f *FailoverModelRequester) IsHealthy(name string) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	now := time.Now()
	for i := range f.providers {
		if f.providers[i].name == name {
			cd := f.providers[i].cooldownEnd
			if cd.IsZero() || now.After(cd) {
				return true
			}
			return false
		}
	}
	return false
}

// GetProviderStatus returns a snapshot of every provider's health. The
// CooldownEnd field is zero for providers not currently in cooldown.
func (f *FailoverModelRequester) GetProviderStatus() map[string]ProviderStatus {
	f.mu.RLock()
	defer f.mu.RUnlock()
	now := time.Now()
	result := make(map[string]ProviderStatus, len(f.providers))
	for i := range f.providers {
		e := &f.providers[i]
		healthy := e.cooldownEnd.IsZero() || now.After(e.cooldownEnd)
		var cdEnd time.Time
		if !healthy {
			cdEnd = e.cooldownEnd
		}
		result[e.name] = ProviderStatus{
			Name:        e.name,
			Healthy:     healthy,
			FailCount:   e.failCount,
			CooldownEnd: cdEnd,
		}
	}
	return result
}

// === internal health tracking ===

// availableIndices returns indices of providers not currently in cooldown,
// in priority order. Providers whose cooldown has expired are included
// (they will be reset by tryRecover when the request loop reaches them).
func (f *FailoverModelRequester) availableIndices() []int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	now := time.Now()
	indices := make([]int, 0, len(f.providers))
	for i := range f.providers {
		e := &f.providers[i]
		if !e.cooldownEnd.IsZero() && now.Before(e.cooldownEnd) {
			continue // still in cooldown
		}
		indices = append(indices, i)
	}
	return indices
}

// tryRecover resets a provider's failure state if its cooldown has expired.
// This implements lazy auto-recovery: on the first request after cooldown
// expires, the provider's failCount is reset and it becomes healthy again.
// No-op if the provider was never in cooldown or is still in cooldown.
func (f *FailoverModelRequester) tryRecover(idx int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	e := &f.providers[idx]
	if !e.cooldownEnd.IsZero() && time.Now().After(e.cooldownEnd) {
		e.failCount = 0
		e.healthy = true
		e.cooldownEnd = time.Time{}
		e.lastFailAt = time.Time{}
	}
}

// recordFailure increments the failure count for the provider at idx and
// places it in cooldown if failCount reaches MaxFailures.
func (f *FailoverModelRequester) recordFailure(idx int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	e := &f.providers[idx]
	e.failCount++
	e.lastFailAt = time.Now()
	if e.failCount >= f.config.MaxFailures {
		e.healthy = false
		e.cooldownEnd = time.Now().Add(f.config.CooldownDuration)
	}
}

// recordSuccess resets the failure count and cooldown for the provider at idx.
func (f *FailoverModelRequester) recordSuccess(idx int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	e := &f.providers[idx]
	e.failCount = 0
	e.healthy = true
	e.cooldownEnd = time.Time{}
	e.lastFailAt = time.Time{}
}

// setLastIndex records the index of the most recently successful provider.
func (f *FailoverModelRequester) setLastIndex(idx int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastIndex = idx
}

// getLastIndex returns the index of the most recently successful provider,
// or -1 if none has succeeded yet.
func (f *FailoverModelRequester) getLastIndex() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.lastIndex
}
