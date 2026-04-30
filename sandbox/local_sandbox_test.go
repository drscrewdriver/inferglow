package sandbox

import (
	"errors"
	"strings"
	"testing"
)

func TestNewLocalSandboxProvider(t *testing.T) {
	p := NewLocalSandboxProvider()
	if p == nil {
		t.Fatal("NewLocalSandboxProvider returned nil")
	}
}

func TestLocalSandboxProviderImplementsProvider(t *testing.T) {
	var _ Provider = (*LocalSandboxProvider)(nil)
}

func TestLocalSandboxProviderNameKind(t *testing.T) {
	p := NewLocalSandboxProvider()
	if p.Name() != "local" {
		t.Errorf("Name() = %q, want %q", p.Name(), "local")
	}
	if p.Kind() != "local" {
		t.Errorf("Kind() = %q, want %q", p.Kind(), "local")
	}
}

func TestLocalSandboxProviderInspectAvailability(t *testing.T) {
	p := NewLocalSandboxProvider()
	avail, err := p.InspectAvailability()
	if err != nil {
		t.Fatalf("InspectAvailability returned error: %v", err)
	}
	if avail == nil {
		t.Fatal("InspectAvailability returned nil")
	}
	if avail.Available {
		t.Error("expected Available=false for local sandbox stub")
	}
	if !strings.Contains(avail.ErrorMessage, "not implemented in P2/P3 initial") {
		t.Errorf("ErrorMessage = %q, want contains %q", avail.ErrorMessage, "not implemented in P2/P3 initial")
	}
}

func TestLocalSandboxProviderCreateHandleStub(t *testing.T) {
	p := NewLocalSandboxProvider()
	policy := DefaultPolicy()
	_, err := p.CreateHandle(nil, &policy)
	if !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("expected ErrProviderUnavailable, got %v", err)
	}
}
