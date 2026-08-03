//go:build desktop

package desktop

import (
	"testing"
)

func TestDesktopBridge_New(t *testing.T) {
	bridge := NewDesktopBridge()
	if bridge == nil {
		t.Fatal("NewDesktopBridge() returned nil")
	}
}

func TestDesktopBridge_GetStatus(t *testing.T) {
	bridge := NewDesktopBridge()
	status := bridge.GetStatus()
	if status == nil {
		t.Fatal("GetStatus() returned nil")
	}
	if status["status"] == "" {
		t.Error("GetStatus() returned empty status field")
	}
}

func TestDesktopBridge_GetDashboardURL(t *testing.T) {
	bridge := NewDesktopBridge()
	url := bridge.GetDashboardURL()
	// A new bridge has no server address, so the dashboard URL should be empty.
	if url != "" {
		t.Errorf("GetDashboardURL() expected empty string, got %q", url)
	}
}
