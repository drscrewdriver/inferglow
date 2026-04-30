package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestToJSON(t *testing.T) {
	s := NewSession("test-1", 1000)
	s.AddMessage("user", "hello", "")
	s.AddMessage("assistant", "hi there", "")
	s.Memo["key"] = "value"

	jsonStr, err := s.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}
	if jsonStr == "" {
		t.Fatal("ToJSON returned empty string")
	}
}

func TestToYAML(t *testing.T) {
	s := NewSession("test-2", 1000)
	s.AddMessage("user", "hello", "")
	s.Memo["count"] = 42

	yamlStr, err := s.ToYAML()
	if err != nil {
		t.Fatalf("ToYAML failed: %v", err)
	}
	if yamlStr == "" {
		t.Fatal("ToYAML returned empty string")
	}
}

func TestSaveAndLoadJSON(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "session.json")

	s := NewSession("persist-test", 500)
	s.AddMessage("user", "first message", "")
	s.AddMessage("assistant", "second message", "")
	s.Memo["lang"] = "en"
	s.AutoResize = true

	err := s.SaveJSON(path)
	if err != nil {
		t.Fatalf("SaveJSON failed: %v", err)
	}

	// Verify file exists
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read saved file: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("Saved file is empty")
	}

	// Load into new session
	s2 := NewSession("loaded", 0)
	err = s2.LoadJSON(path)
	if err != nil {
		t.Fatalf("LoadJSON failed: %v", err)
	}

	// Verify data matches
	if s2.ID != "persist-test" {
		t.Errorf("ID = %q, want %q", s2.ID, "persist-test")
	}
	if len(s2.FullContext) != 2 {
		t.Fatalf("FullContext len = %d, want 2", len(s2.FullContext))
	}
	if s2.AutoResize != true {
		t.Error("AutoResize should be true")
	}
}

func TestSaveAndLoadYAML(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "session.yaml")

	s := NewSession("yaml-test", 500)
	s.AddMessage("user", "yaml msg", "")
	s.Memo["version"] = 1

	err := s.SaveYAML(path)
	if err != nil {
		t.Fatalf("SaveYAML failed: %v", err)
	}

	s2 := NewSession("yaml-loaded", 0)
	err = s2.LoadYAML(path)
	if err != nil {
		t.Fatalf("LoadYAML failed: %v", err)
	}

	if s2.ID != "yaml-test" {
		t.Errorf("ID = %q, want %q", s2.ID, "yaml-test")
	}
	if len(s2.FullContext) != 1 {
		t.Fatalf("FullContext len = %d, want 1", len(s2.FullContext))
	}
}

func TestRoundTripConsistency(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "roundtrip.json")

	s := NewSession("roundtrip", 1000)
	s.AddMessage("user", "msg1", "")
	s.AddMessage("assistant", "msg2", "")
	s.AddMessage("user", "msg3", "")
	s.Memo["a"] = "1"
	s.Memo["b"] = 2
	s.AutoResize = true
	s.MaxLength = 1000

	if err := s.SaveJSON(path); err != nil {
		t.Fatalf("SaveJSON failed: %v", err)
	}

	s2 := NewSession("", 0)
	if err := s2.LoadJSON(path); err != nil {
		t.Fatalf("LoadJSON failed: %v", err)
	}

	// Full consistency check
	if s2.ID != s.ID {
		t.Errorf("ID mismatch: got %q, want %q", s2.ID, s.ID)
	}
	if len(s2.FullContext) != len(s.FullContext) {
		t.Errorf("FullContext len mismatch: got %d, want %d", len(s2.FullContext), len(s.FullContext))
	}
	if len(s2.Memo) != len(s.Memo) {
		t.Errorf("Memo len mismatch: got %d, want %d", len(s2.Memo), len(s.Memo))
	}
	if s2.AutoResize != s.AutoResize {
		t.Errorf("AutoResize mismatch: got %v, want %v", s2.AutoResize, s.AutoResize)
	}
}
