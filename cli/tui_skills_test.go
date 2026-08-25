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

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- parseSkillContent ---

func TestParseSkillContentQuoted(t *testing.T) {
	content := "---\nname: graphify\ndescription: \"Use for any question about a codebase\"\n---\n\n# /graphify\n\nUsage text.\n"
	name, desc, body := parseSkillContent(content)
	if name != "graphify" {
		t.Errorf("name = %q, want graphify", name)
	}
	if desc != "Use for any question about a codebase" {
		t.Errorf("description = %q, want unquoted value", desc)
	}
	if !strings.Contains(body, "# /graphify") {
		t.Errorf("body missing markdown content: %q", body)
	}
	if strings.Contains(body, "---") {
		t.Errorf("body must not contain the frontmatter fence: %q", body)
	}
}

func TestParseSkillContentFolded(t *testing.T) {
	content := "---\nname: agent-reach\ndescription: >\n  MUST USE when user wants to research\n  anything on the internet\n\n  Also MUST USE for platforms\n---\n\nBody here.\n"
	name, desc, body := parseSkillContent(content)
	if name != "agent-reach" {
		t.Errorf("name = %q, want agent-reach", name)
	}
	want := "MUST USE when user wants to research anything on the internet Also MUST USE for platforms"
	if desc != want {
		t.Errorf("folded description = %q, want %q", desc, want)
	}
	if !strings.Contains(body, "Body here.") {
		t.Errorf("body = %q, want body content", body)
	}
}

func TestParseSkillContentLiteral(t *testing.T) {
	content := "---\nname: literal-skill\ndescription: |\n  line one\n  line two\n---\nBody.\n"
	_, desc, _ := parseSkillContent(content)
	if desc != "line one\nline two" {
		t.Errorf("literal description = %q, want newline-joined", desc)
	}
}

func TestParseSkillContentNoFrontmatter(t *testing.T) {
	content := "# /plain\n\nNo frontmatter here.\n"
	name, desc, body := parseSkillContent(content)
	if name != "" || desc != "" {
		t.Errorf("no-frontmatter name/desc = %q/%q, want empty (caller falls back to dir name)", name, desc)
	}
	if body != content {
		t.Errorf("no-frontmatter body = %q, want whole content", body)
	}
}

func TestParseSkillContentUnterminatedFence(t *testing.T) {
	content := "---\nname: broken\nno closing fence\n"
	name, _, body := parseSkillContent(content)
	if name != "" || body != content {
		t.Errorf("unterminated fence: name=%q body=%q, want empty name + full body", name, body)
	}
}

func TestParseSkillContentEmpty(t *testing.T) {
	name, desc, body := parseSkillContent("")
	if name != "" || desc != "" || body != "" {
		t.Errorf("empty content → %q/%q/%q, want all empty", name, desc, body)
	}
}

// --- normalizeSkillName ---

func TestNormalizeSkillName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"AI-Research-SKILLs", "ai-research-skills"},
		{"graphify", "graphify"},
		{"My Skill!", "my-skill"},
		{"foo_bar", "foo-bar"},
		{"  spaced  ", "spaced"},
		{"中文名", ""},
		{"", ""},
		{"---", ""},
	}
	for _, c := range cases {
		if got := normalizeSkillName(c.in); got != c.want {
			t.Errorf("normalizeSkillName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// --- LoadSkills ---

func TestLoadSkills(t *testing.T) {
	root := t.TempDir()
	mk := func(rel, content string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("good-skill/SKILL.md", "---\nname: good-skill\ndescription: \"A good skill\"\n---\n# Good\n\nbody\n")
	mk("folded-skill/SKILL.md", "---\nname: folded-skill\ndescription: >\n  Folded first\n  Folded second\n---\nBody.\n")
	mk("no-frontmatter/SKILL.md", "# /no-frontmatter\n\nplain\n")
	mk("ai-research/SKILL.md", "---\nname: AI-Research-SKILLs\ndescription: Research\n---\nResearch body\n")
	mk("nested-skill/references/SKILL.md", "---\nname: nested-skill\n---\nNested must NOT load\n")
	mk("no-md/notes.txt", "not a skill\n")
	mk("root-note.txt", "file, not dir\n")

	skills := LoadSkills(root)
	byName := map[string]Skill{}
	for _, s := range skills {
		byName[s.Name] = s
	}

	for _, want := range []string{"good-skill", "folded-skill", "no-frontmatter", "ai-research-skills"} {
		if _, ok := byName[want]; !ok {
			t.Errorf("skill %q not loaded (got %v)", want, keysOf(byName))
		}
	}
	if _, ok := byName["nested-skill"]; ok {
		t.Error("nested references/SKILL.md must not be loaded as a skill")
	}
	if byName["good-skill"].Description != "A good skill" {
		t.Errorf("good-skill description = %q", byName["good-skill"].Description)
	}
	if byName["folded-skill"].Description != "Folded first Folded second" {
		t.Errorf("folded-skill description = %q", byName["folded-skill"].Description)
	}
	if byName["no-frontmatter"].Name != "no-frontmatter" {
		t.Errorf("no-frontmatter name should fall back to dir name: %q", byName["no-frontmatter"].Name)
	}
	if !strings.Contains(byName["no-frontmatter"].Body, "plain") {
		t.Errorf("no-frontmatter body = %q", byName["no-frontmatter"].Body)
	}
	if byName["ai-research-skills"].Name != "ai-research-skills" {
		t.Errorf("normalized name = %q", byName["ai-research-skills"].Name)
	}
}

func keysOf(m map[string]Skill) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestLoadSkillsMissingDir(t *testing.T) {
	if got := LoadSkills(filepath.Join(t.TempDir(), "does-not-exist")); got != nil {
		t.Errorf("missing dir → %v, want nil", got)
	}
}

func TestLoadSkillsEmptyDir(t *testing.T) {
	if got := LoadSkills(t.TempDir()); len(got) != 0 {
		t.Errorf("empty dir → %v, want none", got)
	}
}

// --- registerSkillsFromDir ---

func TestRegisterSkillsFromDir(t *testing.T) {
	root := t.TempDir()
	mk := func(rel, content string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("alpha/SKILL.md", "---\nname: alpha\ndescription: \"Alpha skill\"\n---\nAlpha body\n")
	mk("beta/SKILL.md", "# /beta\n\nNo frontmatter.\n")

	r := NewSlashRegistry()
	registerSkillsFromDir(r, root)

	if !r.IsImplemented("alpha") || r.SourceOf("alpha") != "skill" {
		t.Fatalf("alpha: implemented=%v source=%q, want implemented skill", r.IsImplemented("alpha"), r.SourceOf("alpha"))
	}
	if !r.IsImplemented("beta") || r.SourceOf("beta") != "skill" {
		t.Fatalf("beta: implemented=%v source=%q, want implemented skill", r.IsImplemented("beta"), r.SourceOf("beta"))
	}
	// Handlers must be attached (skills are summonable).
	if r.index["alpha"].Handler == nil {
		t.Error("alpha handler missing")
	}
	// Suggest surfaces skills.
	got := r.Suggest("al", 5)
	if len(got) != 1 || got[0].Name != "alpha" {
		t.Fatalf("Suggest(al) = %v, want [alpha]", names(got))
	}
}

func TestRegisterSkillsConflict(t *testing.T) {
	root := t.TempDir()
	mk := func(rel, content string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("tasks/SKILL.md", "---\nname: tasks\ndescription: \"User task skill\"\n---\nTasks body\n")
	mk("vim/SKILL.md", "---\nname: vim\ndescription: \"Vim skill from disk\"\n---\nVim body\n")

	r := NewSlashRegistry()
	// Native command: must never be overridden by a skill.
	r.Register(&SlashCommand{Name: "tasks", Description: "Toggle the task panel"})
	// Compat stub: a real skill with the same name replaces it.
	r.RegisterOverlay(&SlashCommand{Name: "vim", Aliases: []string{"vi"}, Description: "stub", Source: "codex", Implemented: false})

	registerSkillsFromDir(r, root)

	// Native wins over the "tasks" skill.
	if r.SourceOf("tasks") != "" || r.index["tasks"].Description != "Toggle the task panel" {
		t.Errorf("native /tasks must survive skill with same name: source=%q desc=%q",
			r.SourceOf("tasks"), r.index["tasks"].Description)
	}
	// Stub is replaced by the skill, stub alias repointed.
	if r.SourceOf("vim") != "skill" || !r.IsImplemented("vim") {
		t.Fatalf("/vim stub not replaced by skill: source=%q implemented=%v", r.SourceOf("vim"), r.IsImplemented("vim"))
	}
	if r.index["vim"].Description != "Vim skill from disk" {
		t.Errorf("/vim description = %q, want skill description", r.index["vim"].Description)
	}
	if r.SourceOf("vi") != "skill" {
		t.Errorf("stub alias /vi should resolve to the skill, got source=%q", r.SourceOf("vi"))
	}
	got := r.Suggest("vi", 5)
	if len(got) != 1 || got[0].Name != "vim" {
		t.Fatalf("Suggest(vi) = %v, want [vim]", names(got))
	}
}

func TestRegisterSkillCommandsGate(t *testing.T) {
	cfg := DefaultCLIConfig()
	cfg.Features.SkillLoader = false
	r := NewSlashRegistry()
	registerSkillCommands(r, cfg) // must return before touching the real home dir
	if len(r.All()) != 0 {
		t.Fatalf("skill_loader=false must register nothing, got %d", len(r.All()))
	}
}

// --- lazy activation ---

// TestSkillHandlerLazily verifies the activation-timing contract: the
// registered command only carries metadata, and the SKILL.md body is read
// from disk when the handler runs (i.e. when the user confirms the command
// with Enter). Fresh content is read on every activation, and a missing file
// produces a warning instead of stale content.
func TestSkillHandlerLazily(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "demo")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(body string) {
		content := "---\nname: demo\ndescription: \"Demo skill\"\n---\n" + body
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("version one body\n")

	// Register from dir: the registry entry must NOT retain the body.
	r := NewSlashRegistry()
	registerSkillsFromDir(r, root)
	cmd := r.index["demo"]
	if cmd == nil || cmd.Handler == nil {
		t.Fatal("demo skill not registered with handler")
	}
	joined := ""
	m := &chatTUI{}
	if _, quit := cmd.Handler(m, ""); quit {
		t.Fatal("handler must not quit")
	}
	for _, b := range m.transcript {
		joined += b.Source
	}
	if !strings.Contains(joined, "version one body") {
		t.Fatalf("first activation missing body: %q", joined)
	}

	// Change the file on disk: the next activation must read the fresh body
	// (proving the body is loaded at activation, not at registration).
	m = &chatTUI{}
	write("version two body\n")
	if _, quit := cmd.Handler(m, ""); quit {
		t.Fatal("handler must not quit")
	}
	joined = ""
	for _, b := range m.transcript {
		joined += b.Source
	}
	if !strings.Contains(joined, "version two body") {
		t.Fatalf("second activation must read the fresh body: %q", joined)
	}
	if strings.Contains(joined, "version one body") {
		t.Fatalf("stale body leaked into second activation: %q", joined)
	}

	// Remove the file: activation reports a warning, not stale content.
	m = &chatTUI{}
	if err := os.Remove(filepath.Join(skillDir, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if _, quit := cmd.Handler(m, ""); quit {
		t.Fatal("handler must not quit")
	}
	joined = ""
	for _, b := range m.transcript {
		joined += b.Source
	}
	if !strings.Contains(joined, "无法加载") {
		t.Fatalf("missing SKILL.md should warn, got: %q", joined)
	}
}
