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
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// failoverMockProvider is a configurable ModelRequester used by failover
// tests. The name failoverMockProvider (rather than mockProvider) avoids a
// collision with the existing mockProvider defined in pool_test.go.
type failoverMockProvider struct {
	name        string
	genReqErr   error        // error to return from GenerateRequestData
	reqModErr   error        // error to return from RequestModel
	genReqData  *RequestData // pre-built request data (nil => default)
	stream      chan *StreamChunk
	genReqCalls int32
	reqModCalls int32
	bcastCalls  int32
}

func (m *failoverMockProvider) Name() string { return m.name }

func (m *failoverMockProvider) GenerateRequestData(ctx context.Context, req *ModelRequest) (*RequestData, error) {
	atomic.AddInt32(&m.genReqCalls, 1)
	if m.genReqErr != nil {
		return nil, m.genReqErr
	}
	if m.genReqData != nil {
		return m.genReqData, nil
	}
	return &RequestData{Model: m.name}, nil
}

func (m *failoverMockProvider) RequestModel(ctx context.Context, data *RequestData) (<-chan *StreamChunk, error) {
	atomic.AddInt32(&m.reqModCalls, 1)
	if m.reqModErr != nil {
		return nil, m.reqModErr
	}
	if m.stream != nil {
		return m.stream, nil
	}
	ch := make(chan *StreamChunk, 1)
	ch <- &StreamChunk{Delta: "ok-" + m.name, IsDone: true}
	close(ch)
	return ch, nil
}

func (m *failoverMockProvider) BroadcastResponse(ctx context.Context, stream <-chan *StreamChunk) (<-chan *ResultEvent, error) {
	atomic.AddInt32(&m.bcastCalls, 1)
	events := make(chan *ResultEvent, 1)
	events <- &ResultEvent{EventType: EventDone, Payload: m.name}
	close(events)
	return events, nil
}

func (m *failoverMockProvider) GenReqCalls() int32 { return atomic.LoadInt32(&m.genReqCalls) }
func (m *failoverMockProvider) ReqModCalls() int32 { return atomic.LoadInt32(&m.reqModCalls) }
func (m *failoverMockProvider) BcastCalls() int32  { return atomic.LoadInt32(&m.bcastCalls) }

// drain reads and discards all values from a channel until it is closed.
func drain[T any](ch <-chan T) {
	for range ch {
	}
}

// === Tests ===

// TestFailover_PrimarySuccess verifies that when the primary provider is
// healthy, it handles the request directly and the secondary is never tried.
func TestFailover_PrimarySuccess(t *testing.T) {
	primary := &failoverMockProvider{name: "primary"}
	secondary := &failoverMockProvider{name: "secondary"}
	f := NewFailoverModelRequester([]ModelRequester{primary, secondary}, FailoverConfig{})

	stream, err := f.RequestModel(context.Background(), &RequestData{})
	if err != nil {
		t.Fatalf("RequestModel unexpected error: %v", err)
	}
	if primary.ReqModCalls() != 1 {
		t.Errorf("primary calls = %d, want 1", primary.ReqModCalls())
	}
	if secondary.ReqModCalls() != 0 {
		t.Errorf("secondary calls = %d, want 0 (no failover expected)", secondary.ReqModCalls())
	}

	// Verify the stream comes from the primary provider.
	var delta string
	for chunk := range stream {
		if chunk.Delta != "" {
			delta = chunk.Delta
		}
	}
	if delta != "ok-primary" {
		t.Errorf("stream delta = %q, want %q", delta, "ok-primary")
	}

	// BroadcastResponse should delegate to the primary (last successful).
	events, err := f.BroadcastResponse(context.Background(), nil)
	if err != nil {
		t.Fatalf("BroadcastResponse unexpected error: %v", err)
	}
	if primary.BcastCalls() != 1 {
		t.Errorf("primary bcast calls = %d, want 1", primary.BcastCalls())
	}
	if secondary.BcastCalls() != 0 {
		t.Errorf("secondary bcast calls = %d, want 0", secondary.BcastCalls())
	}
	drain(events)

	// Name should be "failover".
	if f.Name() != "failover" {
		t.Errorf("Name() = %q, want %q", f.Name(), "failover")
	}
}

// TestFailover_PrimaryFails_SecondarySucceeds verifies that when the primary
// provider fails, the request automatically fails over to the secondary.
func TestFailover_PrimaryFails_SecondarySucceeds(t *testing.T) {
	primary := &failoverMockProvider{name: "primary", reqModErr: errors.New("primary down")}
	secondary := &failoverMockProvider{name: "secondary"}
	f := NewFailoverModelRequester([]ModelRequester{primary, secondary}, FailoverConfig{})

	stream, err := f.RequestModel(context.Background(), &RequestData{})
	if err != nil {
		t.Fatalf("RequestModel unexpected error: %v", err)
	}
	if primary.ReqModCalls() != 1 {
		t.Errorf("primary calls = %d, want 1", primary.ReqModCalls())
	}
	if secondary.ReqModCalls() != 1 {
		t.Errorf("secondary calls = %d, want 1 (failover expected)", secondary.ReqModCalls())
	}

	// Stream should come from the secondary provider.
	var delta string
	for chunk := range stream {
		if chunk.Delta != "" {
			delta = chunk.Delta
		}
	}
	if delta != "ok-secondary" {
		t.Errorf("stream delta = %q, want %q", delta, "ok-secondary")
	}

	// BroadcastResponse should delegate to secondary (last successful).
	events, err := f.BroadcastResponse(context.Background(), nil)
	if err != nil {
		t.Fatalf("BroadcastResponse unexpected error: %v", err)
	}
	if secondary.BcastCalls() != 1 {
		t.Errorf("secondary bcast calls = %d, want 1", secondary.BcastCalls())
	}
	if primary.BcastCalls() != 0 {
		t.Errorf("primary bcast calls = %d, want 0", primary.BcastCalls())
	}
	drain(events)
}

// TestFailover_AllProvidersFail verifies that when every provider fails, an
// AllProvidersFailedError is returned containing all the underlying errors.
func TestFailover_AllProvidersFail(t *testing.T) {
	primary := &failoverMockProvider{name: "primary", reqModErr: errors.New("primary error")}
	secondary := &failoverMockProvider{name: "secondary", reqModErr: errors.New("secondary error")}
	f := NewFailoverModelRequester([]ModelRequester{primary, secondary}, FailoverConfig{})

	_, err := f.RequestModel(context.Background(), &RequestData{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var allErr *AllProvidersFailedError
	if !errors.As(err, &allErr) {
		t.Fatalf("expected *AllProvidersFailedError, got %T: %v", err, err)
	}
	if len(allErr.Errors) != 2 {
		t.Fatalf("expected 2 underlying errors, got %d", len(allErr.Errors))
	}

	// errors.Is should find each underlying error via Unwrap() []error.
	if !errors.Is(err, primary.reqModErr) {
		t.Errorf("errors.Is(err, primary error) = false, want true")
	}
	if !errors.Is(err, secondary.reqModErr) {
		t.Errorf("errors.Is(err, secondary error) = false, want true")
	}

	// Error message should mention "all providers failed" and both messages.
	msg := err.Error()
	if !strings.Contains(msg, "all providers failed") {
		t.Errorf("error message missing prefix: %s", msg)
	}
	if !strings.Contains(msg, "primary error") {
		t.Errorf("error message missing 'primary error': %s", msg)
	}
	if !strings.Contains(msg, "secondary error") {
		t.Errorf("error message missing 'secondary error': %s", msg)
	}
}

// TestFailover_CooldownRecovery verifies that after a provider enters cooldown,
// it is skipped until the cooldown expires, after which it is automatically
// recovered and retried.
func TestFailover_CooldownRecovery(t *testing.T) {
	primary := &failoverMockProvider{name: "primary", reqModErr: errors.New("primary down")}
	secondary := &failoverMockProvider{name: "secondary"}
	f := NewFailoverModelRequester(
		[]ModelRequester{primary, secondary},
		FailoverConfig{MaxFailures: 1, CooldownDuration: 100 * time.Millisecond},
	)

	// Call 1: primary fails (failCount=1 >= 1 => cooldown), secondary succeeds.
	stream1, err := f.RequestModel(context.Background(), &RequestData{})
	if err != nil {
		t.Fatalf("call 1 unexpected error: %v", err)
	}
	drain(stream1)
	if primary.ReqModCalls() != 1 {
		t.Errorf("after call 1: primary calls = %d, want 1", primary.ReqModCalls())
	}
	if secondary.ReqModCalls() != 1 {
		t.Errorf("after call 1: secondary calls = %d, want 1", secondary.ReqModCalls())
	}

	// Call 2 immediately: primary is in cooldown and should be skipped.
	stream2, err := f.RequestModel(context.Background(), &RequestData{})
	if err != nil {
		t.Fatalf("call 2 unexpected error: %v", err)
	}
	drain(stream2)
	if primary.ReqModCalls() != 1 {
		t.Errorf("after call 2: primary calls = %d, want 1 (should be in cooldown)", primary.ReqModCalls())
	}
	if secondary.ReqModCalls() != 2 {
		t.Errorf("after call 2: secondary calls = %d, want 2", secondary.ReqModCalls())
	}

	// Wait for cooldown to expire, then primary should be retried.
	time.Sleep(120 * time.Millisecond)

	stream3, err := f.RequestModel(context.Background(), &RequestData{})
	if err != nil {
		t.Fatalf("call 3 unexpected error: %v", err)
	}
	drain(stream3)
	if primary.ReqModCalls() != 2 {
		t.Errorf("after call 3: primary calls = %d, want 2 (cooldown should have expired)", primary.ReqModCalls())
	}
	if secondary.ReqModCalls() != 3 {
		t.Errorf("after call 3: secondary calls = %d, want 3", secondary.ReqModCalls())
	}
}

// TestFailover_PriorityOrder verifies that providers are tried in the order
// they were configured: [A, B, C] → A first, then B, then C.
func TestFailover_PriorityOrder(t *testing.T) {
	a := &failoverMockProvider{name: "A", reqModErr: errors.New("A down")}
	b := &failoverMockProvider{name: "B"}
	c := &failoverMockProvider{name: "C"}
	f := NewFailoverModelRequester(
		[]ModelRequester{a, b, c},
		FailoverConfig{MaxFailures: 1}, // A enters cooldown after a single failure
	)

	// Call 1: A fails (enters cooldown), B succeeds. C should not be tried.
	stream1, err := f.RequestModel(context.Background(), &RequestData{})
	if err != nil {
		t.Fatalf("call 1 unexpected error: %v", err)
	}
	drain(stream1)
	if a.ReqModCalls() != 1 {
		t.Errorf("after call 1: A calls = %d, want 1", a.ReqModCalls())
	}
	if b.ReqModCalls() != 1 {
		t.Errorf("after call 1: B calls = %d, want 1", b.ReqModCalls())
	}
	if c.ReqModCalls() != 0 {
		t.Errorf("after call 1: C calls = %d, want 0 (B succeeded first)", c.ReqModCalls())
	}

	// Call 2: A is in cooldown (skipped). B is tried first and succeeds.
	stream2, err := f.RequestModel(context.Background(), &RequestData{})
	if err != nil {
		t.Fatalf("call 2 unexpected error: %v", err)
	}
	drain(stream2)
	if a.ReqModCalls() != 1 {
		t.Errorf("after call 2: A calls = %d, want 1 (in cooldown)", a.ReqModCalls())
	}
	if b.ReqModCalls() != 2 {
		t.Errorf("after call 2: B calls = %d, want 2", b.ReqModCalls())
	}
	if c.ReqModCalls() != 0 {
		t.Errorf("after call 2: C calls = %d, want 0", c.ReqModCalls())
	}

	// Separate scenario: when A and B both fail, C is reached.
	a2 := &failoverMockProvider{name: "A2", reqModErr: errors.New("A2 down")}
	b2 := &failoverMockProvider{name: "B2", reqModErr: errors.New("B2 down")}
	c2 := &failoverMockProvider{name: "C2"}
	f2 := NewFailoverModelRequester([]ModelRequester{a2, b2, c2}, FailoverConfig{MaxFailures: 1})

	stream3, err := f2.RequestModel(context.Background(), &RequestData{})
	if err != nil {
		t.Fatalf("call 3 unexpected error: %v", err)
	}
	drain(stream3)
	if a2.ReqModCalls() != 1 {
		t.Errorf("A2 calls = %d, want 1", a2.ReqModCalls())
	}
	if b2.ReqModCalls() != 1 {
		t.Errorf("B2 calls = %d, want 1", b2.ReqModCalls())
	}
	if c2.ReqModCalls() != 1 {
		t.Errorf("C2 calls = %d, want 1 (failover reached C2)", c2.ReqModCalls())
	}
}

// TestFailover_SkipCooldownProvider verifies the "A fails → B → C → A (in
// cooldown, skipped)" ordering. Within a single request, providers are tried
// in priority order and each is tried at most once. After A enters cooldown
// it is skipped on subsequent requests while B and C remain available.
func TestFailover_SkipCooldownProvider(t *testing.T) {
	a := &failoverMockProvider{name: "A", reqModErr: errors.New("A down")}
	b := &failoverMockProvider{name: "B", reqModErr: errors.New("B down")}
	c := &failoverMockProvider{name: "C"}
	f := NewFailoverModelRequester(
		[]ModelRequester{a, b, c},
		FailoverConfig{MaxFailures: 1, CooldownDuration: 5 * time.Second},
	)

	// Call 1: A fails (cooldown), B fails (cooldown), C succeeds.
	// Within this single request the order is A → B → C and none are retried.
	stream1, err := f.RequestModel(context.Background(), &RequestData{})
	if err != nil {
		t.Fatalf("call 1 unexpected error: %v", err)
	}
	drain(stream1)
	if a.ReqModCalls() != 1 {
		t.Errorf("after call 1: A calls = %d, want 1 (tried once, not retried)", a.ReqModCalls())
	}
	if b.ReqModCalls() != 1 {
		t.Errorf("after call 1: B calls = %d, want 1", b.ReqModCalls())
	}
	if c.ReqModCalls() != 1 {
		t.Errorf("after call 1: C calls = %d, want 1", c.ReqModCalls())
	}

	// Call 2: A and B are in cooldown (skipped). Only C is tried.
	stream2, err := f.RequestModel(context.Background(), &RequestData{})
	if err != nil {
		t.Fatalf("call 2 unexpected error: %v", err)
	}
	drain(stream2)
	if a.ReqModCalls() != 1 {
		t.Errorf("after call 2: A calls = %d, want 1 (in cooldown, skipped)", a.ReqModCalls())
	}
	if b.ReqModCalls() != 1 {
		t.Errorf("after call 2: B calls = %d, want 1 (in cooldown, skipped)", b.ReqModCalls())
	}
	if c.ReqModCalls() != 2 {
		t.Errorf("after call 2: C calls = %d, want 2", c.ReqModCalls())
	}
}

// TestFailover_MaxFailuresBoundary tests the exact boundary at which
// MaxFailures triggers a cooldown. With MaxFailures=3 the provider should
// still be available after 2 failures (failCount < 3) and enter cooldown on
// the 3rd failure (failCount >= 3).
func TestFailover_MaxFailuresBoundary(t *testing.T) {
	primary := &failoverMockProvider{name: "primary", reqModErr: errors.New("fail")}
	secondary := &failoverMockProvider{name: "secondary"}
	f := NewFailoverModelRequester(
		[]ModelRequester{primary, secondary},
		FailoverConfig{MaxFailures: 3, CooldownDuration: 5 * time.Second},
	)

	// Calls 1 and 2: primary fails but failCount (1, 2) < 3, no cooldown.
	for i := 0; i < 2; i++ {
		stream, err := f.RequestModel(context.Background(), &RequestData{})
		if err != nil {
			t.Fatalf("call %d unexpected error: %v", i+1, err)
		}
		drain(stream)
	}
	if got := primary.ReqModCalls(); got != 2 {
		t.Fatalf("after 2 failures: primary calls = %d, want 2", got)
	}
	// Primary should still be healthy (failCount < MaxFailures).
	if !f.IsHealthy("primary") {
		t.Error("primary should be healthy after 2 failures (failCount < MaxFailures=3)")
	}

	// Call 3: primary fails again (failCount=3 >= 3, enters cooldown).
	stream, err := f.RequestModel(context.Background(), &RequestData{})
	if err != nil {
		t.Fatalf("call 3 unexpected error: %v", err)
	}
	drain(stream)
	if got := primary.ReqModCalls(); got != 3 {
		t.Errorf("after 3 failures: primary calls = %d, want 3", got)
	}
	// Primary should now be in cooldown.
	if f.IsHealthy("primary") {
		t.Error("primary should be in cooldown after 3 failures (failCount >= MaxFailures=3)")
	}

	// Call 4: primary is in cooldown, skipped. Secondary handles the request.
	stream, err = f.RequestModel(context.Background(), &RequestData{})
	if err != nil {
		t.Fatalf("call 4 unexpected error: %v", err)
	}
	drain(stream)
	if got := primary.ReqModCalls(); got != 3 {
		t.Errorf("after call 4: primary calls = %d, want 3 (in cooldown, skipped)", got)
	}
	if got := secondary.ReqModCalls(); got != 4 {
		t.Errorf("after call 4: secondary calls = %d, want 4", got)
	}
}

// TestFailover_GetProviderStatus verifies that GetProviderStatus returns the
// correct health snapshot for each provider, including after cooldown is
// triggered and after it expires.
func TestFailover_GetProviderStatus(t *testing.T) {
	primary := &failoverMockProvider{name: "primary", reqModErr: errors.New("fail")}
	secondary := &failoverMockProvider{name: "secondary"}
	f := NewFailoverModelRequester(
		[]ModelRequester{primary, secondary},
		FailoverConfig{MaxFailures: 2, CooldownDuration: 100 * time.Millisecond},
	)

	// Initially both providers are healthy.
	status := f.GetProviderStatus()
	if len(status) != 2 {
		t.Fatalf("expected 2 providers in status, got %d", len(status))
	}
	ps, ok := status["primary"]
	if !ok {
		t.Fatal("primary not in status")
	}
	if !ps.Healthy {
		t.Error("primary should be healthy initially")
	}
	if ps.FailCount != 0 {
		t.Errorf("primary failCount = %d, want 0", ps.FailCount)
	}
	if !ps.CooldownEnd.IsZero() {
		t.Error("primary CooldownEnd should be zero initially")
	}

	// Fail primary twice to trigger cooldown (MaxFailures=2).
	for i := 0; i < 2; i++ {
		stream, err := f.RequestModel(context.Background(), &RequestData{})
		if err != nil {
			t.Fatalf("call %d unexpected error: %v", i+1, err)
		}
		drain(stream)
	}

	// After 2 failures primary should be in cooldown.
	status = f.GetProviderStatus()
	if status["primary"].Healthy {
		t.Error("primary should be unhealthy after 2 failures")
	}
	if status["primary"].FailCount != 2 {
		t.Errorf("primary failCount = %d, want 2", status["primary"].FailCount)
	}
	if status["primary"].CooldownEnd.IsZero() {
		t.Error("primary CooldownEnd should be non-zero when in cooldown")
	}
	if !status["secondary"].Healthy {
		t.Error("secondary should be healthy")
	}

	// Wait for cooldown to expire.
	time.Sleep(120 * time.Millisecond)

	// After cooldown expires, GetProviderStatus should reflect recovery
	// (healthy=true) even before the next request resets failCount.
	status = f.GetProviderStatus()
	if !status["primary"].Healthy {
		t.Error("primary should be healthy after cooldown expired")
	}
	// failCount is still 2 (lazy reset happens on next request).
	if status["primary"].FailCount != 2 {
		t.Errorf("primary failCount = %d, want 2 (lazy reset)", status["primary"].FailCount)
	}
	// CooldownEnd should be zero when healthy (not in cooldown).
	if !status["primary"].CooldownEnd.IsZero() {
		t.Error("primary CooldownEnd should be zero when healthy")
	}

	// After a request, primary's failCount is reset (lazy recovery).
	// Primary still fails, so failCount goes back to 1 (no cooldown since 1 < 2).
	stream, err := f.RequestModel(context.Background(), &RequestData{})
	if err != nil {
		t.Fatalf("post-cooldown request unexpected error: %v", err)
	}
	drain(stream)
	status = f.GetProviderStatus()
	if status["primary"].FailCount != 1 {
		t.Errorf("after recovery + 1 failure: primary failCount = %d, want 1", status["primary"].FailCount)
	}
	if !status["primary"].Healthy {
		t.Error("primary should be healthy after recovery + 1 failure (failCount < MaxFailures)")
	}
}

// TestFailover_IsHealthy verifies that IsHealthy correctly reports whether a
// named provider is in cooldown.
func TestFailover_IsHealthy(t *testing.T) {
	primary := &failoverMockProvider{name: "primary", reqModErr: errors.New("fail")}
	secondary := &failoverMockProvider{name: "secondary"}
	f := NewFailoverModelRequester(
		[]ModelRequester{primary, secondary},
		FailoverConfig{MaxFailures: 1, CooldownDuration: 100 * time.Millisecond},
	)

	// Initially all known providers are healthy; unknown returns false.
	if !f.IsHealthy("primary") {
		t.Error("primary should be healthy initially")
	}
	if !f.IsHealthy("secondary") {
		t.Error("secondary should be healthy initially")
	}
	if f.IsHealthy("unknown") {
		t.Error("unknown provider should report as not healthy")
	}

	// Fail primary once → cooldown (MaxFailures=1).
	stream, err := f.RequestModel(context.Background(), &RequestData{})
	if err != nil {
		t.Fatalf("call 1 unexpected error: %v", err)
	}
	drain(stream)

	if f.IsHealthy("primary") {
		t.Error("primary should be unhealthy after entering cooldown")
	}
	if !f.IsHealthy("secondary") {
		t.Error("secondary should still be healthy")
	}

	// Wait for cooldown to expire.
	time.Sleep(120 * time.Millisecond)

	// After cooldown, primary is considered healthy again.
	if !f.IsHealthy("primary") {
		t.Error("primary should be healthy after cooldown expired")
	}
}

// TestFailover_NamedConstructor verifies that NewFailoverModelRequesterNamed
// orders providers alphabetically by name and uses the map keys as the
// authoritative provider names.
func TestFailover_NamedConstructor(t *testing.T) {
	a := &failoverMockProvider{name: "ignored-A", reqModErr: errors.New("A down")}
	b := &failoverMockProvider{name: "ignored-B"}
	c := &failoverMockProvider{name: "ignored-C"}

	// Map keys are the authoritative names; they also determine priority
	// order (alphabetical). Insert in non-alphabetical order to verify
	// sorting: the priority should be A → B → C.
	f := NewFailoverModelRequesterNamed(
		map[string]ModelRequester{"C": c, "A": a, "B": b},
		FailoverConfig{MaxFailures: 1},
	)

	// A fails (enters cooldown), B succeeds, C should not be tried.
	stream, err := f.RequestModel(context.Background(), &RequestData{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	drain(stream)

	if a.ReqModCalls() != 1 {
		t.Errorf("A calls = %d, want 1", a.ReqModCalls())
	}
	if b.ReqModCalls() != 1 {
		t.Errorf("B calls = %d, want 1 (tried after A failed)", b.ReqModCalls())
	}
	if c.ReqModCalls() != 0 {
		t.Errorf("C calls = %d, want 0 (B succeeded)", c.ReqModCalls())
	}

	// Verify names in GetProviderStatus come from the map keys, not Name().
	status := f.GetProviderStatus()
	for _, name := range []string{"A", "B", "C"} {
		if _, ok := status[name]; !ok {
			t.Errorf("expected provider %q in status", name)
		}
	}
}

// TestFailover_GenerateRequestDataFailover verifies that GenerateRequestData
// also performs failover, not just RequestModel.
func TestFailover_GenerateRequestDataFailover(t *testing.T) {
	primary := &failoverMockProvider{name: "primary", genReqErr: errors.New("primary gen error")}
	secondary := &failoverMockProvider{name: "secondary"}
	f := NewFailoverModelRequester([]ModelRequester{primary, secondary}, FailoverConfig{})

	data, err := f.GenerateRequestData(context.Background(), &ModelRequest{})
	if err != nil {
		t.Fatalf("GenerateRequestData unexpected error: %v", err)
	}
	if primary.GenReqCalls() != 1 {
		t.Errorf("primary gen calls = %d, want 1", primary.GenReqCalls())
	}
	if secondary.GenReqCalls() != 1 {
		t.Errorf("secondary gen calls = %d, want 1 (failover expected)", secondary.GenReqCalls())
	}
	if data == nil {
		t.Fatal("expected non-nil RequestData")
	}
	if data.Model != "secondary" {
		t.Errorf("data.Model = %q, want %q", data.Model, "secondary")
	}
}

// TestFailover_Defaults verifies that a zero-value FailoverConfig uses the
// defaults (MaxFailures=3, CooldownDuration=30s).
func TestFailover_Defaults(t *testing.T) {
	primary := &failoverMockProvider{name: "primary", reqModErr: errors.New("fail")}
	secondary := &failoverMockProvider{name: "secondary"}
	// Zero-value config → defaults (MaxFailures=3, CooldownDuration=30s).
	f := NewFailoverModelRequester([]ModelRequester{primary, secondary}, FailoverConfig{})

	// With default MaxFailures=3, primary enters cooldown after 3 failures.
	for i := 0; i < 3; i++ {
		stream, _ := f.RequestModel(context.Background(), &RequestData{})
		drain(stream)
	}
	if got := primary.ReqModCalls(); got != 3 {
		t.Fatalf("primary calls = %d, want 3 (default MaxFailures=3)", got)
	}

	// 4th call: primary in cooldown, skipped.
	stream, _ := f.RequestModel(context.Background(), &RequestData{})
	drain(stream)
	if got := primary.ReqModCalls(); got != 3 {
		t.Errorf("primary calls after cooldown = %d, want 3 (should be skipped)", got)
	}
	if got := secondary.ReqModCalls(); got != 4 {
		t.Errorf("secondary calls = %d, want 4", got)
	}
}

// TestAllProvidersFailedError tests the AllProvidersFailedError type
// directly, including Error(), Unwrap(), and nil-receiver handling.
func TestAllProvidersFailedError(t *testing.T) {
	err1 := errors.New("error one")
	err2 := errors.New("error two")

	e := &AllProvidersFailedError{Errors: []error{err1, err2}}

	// Error() should contain the prefix and both messages.
	msg := e.Error()
	if !strings.Contains(msg, "all providers failed") {
		t.Errorf("Error() missing prefix: %s", msg)
	}
	if !strings.Contains(msg, "error one") {
		t.Errorf("Error() missing 'error one': %s", msg)
	}
	if !strings.Contains(msg, "error two") {
		t.Errorf("Error() missing 'error two': %s", msg)
	}

	// Unwrap() returns the slice for errors.Is / errors.As traversal.
	unwrapped := e.Unwrap()
	if len(unwrapped) != 2 {
		t.Fatalf("Unwrap() returned %d errors, want 2", len(unwrapped))
	}
	if !errors.Is(e, err1) {
		t.Errorf("errors.Is(e, err1) = false, want true")
	}
	if !errors.Is(e, err2) {
		t.Errorf("errors.Is(e, err2) = false, want true")
	}

	// Empty error slice => bare prefix.
	empty := &AllProvidersFailedError{}
	if empty.Error() != "all providers failed" {
		t.Errorf("empty Error() = %q, want %q", empty.Error(), "all providers failed")
	}

	// Nil receiver should not panic.
	var nilErr *AllProvidersFailedError
	if nilErr.Error() != "all providers failed" {
		t.Errorf("nil Error() = %q, want %q", nilErr.Error(), "all providers failed")
	}
	if nilErr.Unwrap() != nil {
		t.Errorf("nil Unwrap() = %v, want nil", nilErr.Unwrap())
	}
}

// TestFailover_AllInCooldown verifies that when all providers are in cooldown,
// an AllProvidersFailedError with no errors is returned (since no provider
// was attempted).
func TestFailover_AllInCooldown(t *testing.T) {
	primary := &failoverMockProvider{name: "primary", reqModErr: errors.New("fail")}
	secondary := &failoverMockProvider{name: "secondary", reqModErr: errors.New("fail")}
	f := NewFailoverModelRequester(
		[]ModelRequester{primary, secondary},
		FailoverConfig{MaxFailures: 1, CooldownDuration: 5 * time.Second},
	)

	// First request: both fail, both enter cooldown.
	_, err := f.RequestModel(context.Background(), &RequestData{})
	if err == nil {
		t.Fatal("expected error on first call, got nil")
	}

	// Second request: both in cooldown, no providers available.
	_, err = f.RequestModel(context.Background(), &RequestData{})
	if err == nil {
		t.Fatal("expected error on second call, got nil")
	}
	var allErr *AllProvidersFailedError
	if !errors.As(err, &allErr) {
		t.Fatalf("expected *AllProvidersFailedError, got %T: %v", err, err)
	}
	if len(allErr.Errors) != 0 {
		t.Errorf("expected 0 errors (no providers tried), got %d", len(allErr.Errors))
	}
}
