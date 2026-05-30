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

package skill

import (
	"errors"
	"testing"
)

func TestSkillLibraryInstallAndGet(t *testing.T) {
	lib := NewSkillLibrary("/tmp/skills")
	rev, err := lib.Install("my-skill", "global")
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if rev.Revision != "r1" {
		t.Errorf("expected r1, got %s", rev.Revision)
	}
	if rev.Scope != "global" {
		t.Errorf("expected scope=global, got %s", rev.Scope)
	}

	got, err := lib.GetRevision("my-skill", "")
	if err != nil {
		t.Fatalf("GetRevision: %v", err)
	}
	if got.Revision != "r1" {
		t.Errorf("expected r1, got %s", got.Revision)
	}
}

func TestSkillLibraryMultipleRevisions(t *testing.T) {
	lib := NewSkillLibrary("/tmp/skills")
	lib.Install("pkg", "global")
	rev2, _ := lib.Install("pkg", "global")
	if rev2.Revision != "r2" {
		t.Errorf("expected r2, got %s", rev2.Revision)
	}

	// Latest.
	got, _ := lib.GetRevision("pkg", "")
	if got.Revision != "r2" {
		t.Errorf("latest should be r2, got %s", got.Revision)
	}

	// Specific.
	got, err := lib.GetRevision("pkg", "r1")
	if err != nil {
		t.Fatalf("GetRevision(r1): %v", err)
	}
	if got.Revision != "r1" {
		t.Errorf("expected r1, got %s", got.Revision)
	}
}

func TestSkillLibraryNotFound(t *testing.T) {
	lib := NewSkillLibrary("/tmp/skills")
	_, err := lib.GetRevision("nonexistent", "")
	if !errors.Is(err, ErrSkillNotFound) {
		t.Fatalf("expected ErrSkillNotFound, got %v", err)
	}
}

func TestSkillLibraryInvalidSource(t *testing.T) {
	lib := NewSkillLibrary("/tmp/skills")
	_, err := lib.Install("", "global")
	if !errors.Is(err, ErrInvalidSource) {
		t.Fatalf("expected ErrInvalidSource, got %v", err)
	}
}

func TestSkillLibraryListInstalled(t *testing.T) {
	lib := NewSkillLibrary("/tmp/skills")
	lib.Install("a", "global")
	lib.Install("b", "agent")

	all := lib.ListInstalled()
	if len(all) != 2 {
		t.Fatalf("expected 2, got %d", len(all))
	}
}

func TestSkillLibraryUninstall(t *testing.T) {
	lib := NewSkillLibrary("/tmp/skills")
	lib.Install("pkg", "global")
	if !lib.Uninstall("pkg") {
		t.Error("expected true")
	}
	if lib.Uninstall("pkg") {
		t.Error("expected false after uninstall")
	}
}

func TestSkillBinding(t *testing.T) {
	b := SkillBinding{
		Skills: []SkillRef{
			{Source: "code-review", Revision: "r1"},
			{Source: "summarizer"},
		},
		Mode: ModelDecision,
	}
	if len(b.Skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(b.Skills))
	}
	if b.Mode != ModelDecision {
		t.Errorf("expected model_decision, got %s", b.Mode)
	}
}
