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

package compress

import (
	"testing"

	"github.com/inferglow/context"
)

// mockStepStore implements the minimal methods needed for tests.
type mockStepStore struct{}

func (m *mockStepStore) AppendStep(step contextmgr.StepRecord) error { return nil }
func (m *mockStepStore) GetStep(stepID int) (*contextmgr.StepRecord, error) {
	return &contextmgr.StepRecord{}, nil
}
func (m *mockStepStore) RangeSteps(from, to int) ([]contextmgr.StepRecord, error) { return nil, nil }
func (m *mockStepStore) UpsertRef(ref contextmgr.RefRecord) error                 { return nil }
func (m *mockStepStore) GetRef(stepID int) (*contextmgr.RefRecord, error) {
	return &contextmgr.RefRecord{}, nil
}
func (m *mockStepStore) AllActiveStepIDs() ([]int, error)               { return nil, nil }
func (m *mockStepStore) RemoveRef(stepID int) error                     { return nil }
func (m *mockStepStore) AppendL1(rec contextmgr.L1Record) error         { return nil }
func (m *mockStepStore) GetL1(stepID int) (*contextmgr.L1Record, error) { return nil, nil }
func (m *mockStepStore) AppendL2(rec contextmgr.L2Record) error         { return nil }
func (m *mockStepStore) GetL2(stepID int) (*contextmgr.L2Record, error) { return nil, nil }
func (m *mockStepStore) HotFacts(minRefCount int, minStrength float64) ([]contextmgr.L2Record, error) {
	return nil, nil
}
func (m *mockStepStore) AppendL3(rec contextmgr.L3Record) error                     { return nil }
func (m *mockStepStore) GetL3(stepID int) (*contextmgr.L3Record, error)             { return nil, nil }
func (m *mockStepStore) UpsertLongMem(mem contextmgr.LongMemRecord) error           { return nil }
func (m *mockStepStore) GetLongMem(memID string) (*contextmgr.LongMemRecord, error) { return nil, nil }
func (m *mockStepStore) SearchLongMem(query string, category string, limit int) ([]contextmgr.LongMemRecord, error) {
	return nil, nil
}
func (m *mockStepStore) RemoveLongMem(memID string) error             { return nil }
func (m *mockStepStore) AppendAudit(rec contextmgr.AuditRecord) error { return nil }
func (m *mockStepStore) Close() error                                 { return nil }

func TestIdleConsolidator_New(t *testing.T) {
	store := &mockStepStore{}
	cfg := contextmgr.DefaultConfig()
	ic := NewIdleConsolidator(store, cfg)
	if ic == nil {
		t.Fatal("NewIdleConsolidator returned nil")
	}
}

func TestIdleConsolidator_Tick(t *testing.T) {
	store := &mockStepStore{}
	cfg := contextmgr.DefaultConfig()
	cfg.IdleConsolidation.Enabled = true
	cfg.IdleConsolidation.IdleSteps = 3
	ic := NewIdleConsolidator(store, cfg)

	// First two ticks should return false
	if ic.Tick() {
		t.Errorf("tick 1: expected false, got true")
	}
	if ic.Tick() {
		t.Errorf("tick 2: expected false, got true")
	}
	// Third tick should return true (hits IdleSteps=3)
	if !ic.Tick() {
		t.Errorf("tick 3: expected true (hit IdleSteps=3), got false")
	}
	// Fourth tick should also return true (counter keeps incrementing)
	if !ic.Tick() {
		t.Errorf("tick 4: expected true (counter > IdleSteps), got false")
	}
}

func TestIdleConsolidator_Tick_Reset(t *testing.T) {
	store := &mockStepStore{}
	cfg := contextmgr.DefaultConfig()
	cfg.IdleConsolidation.Enabled = true
	cfg.IdleConsolidation.IdleSteps = 5
	ic := NewIdleConsolidator(store, cfg)

	// Tick 3 times
	ic.Tick()
	ic.Tick()
	ic.Tick()

	// Reset
	ic.Reset()

	// After reset, should need 5 more ticks
	for i := 0; i < 4; i++ {
		if ic.Tick() {
			t.Errorf("after reset, tick %d: expected false, got true", i+1)
		}
	}
	// 5th tick after reset should trigger
	if !ic.Tick() {
		t.Errorf("after reset, tick 5: expected true (hit IdleSteps=5), got false")
	}
}

func TestIdleConsolidator_Disabled(t *testing.T) {
	store := &mockStepStore{}
	cfg := contextmgr.DefaultConfig()
	cfg.IdleConsolidation.Enabled = false
	cfg.IdleConsolidation.IdleSteps = 3
	ic := NewIdleConsolidator(store, cfg)

	// Tick should always return false when disabled
	for i := 0; i < 10; i++ {
		if ic.Tick() {
			t.Errorf("tick %d: expected false when disabled, got true", i+1)
		}
	}
}
