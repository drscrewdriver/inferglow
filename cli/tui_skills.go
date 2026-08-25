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
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// skillBodyMaxLines caps how many SKILL.md body lines /skill renders in the
// transcript; the remainder is summarized with the full file path.
const skillBodyMaxLines = 100

// skillBodyWidth word-wraps long body lines so they never overflow the
// viewport horizontally.
const skillBodyWidth = 100

// Skill is a skill discovered under ~/.agents/skills following the SKILL.md
// convention (a directory containing a SKILL.md file with YAML frontmatter).
type Skill struct {
	Name        string // normalized slash-command name
	Description string // frontmatter description ("" when absent)
	Body        string // markdown body after the frontmatter fence
	Dir         string // absolute path of the skill directory
}

// LoadSkills scans dir's top-level subdirectories for SKILL.md files and
// returns one Skill per valid skill, in directory order. Missing or
// unreadable roots yield nil; subdirectories without SKILL.md are skipped.
// Nested directories (e.g. a skill's references/) are never scanned.
func LoadSkills(dir string) []Skill {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []Skill
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillDir := filepath.Join(dir, e.Name())
		content, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
		if err != nil {
			continue // no SKILL.md → not a skill
		}
		name, desc, body := parseSkillContent(string(content))
		if name == "" {
			name = e.Name()
		}
		name = normalizeSkillName(name)
		if name == "" {
			continue // nothing usable as a slash-command name
		}
		out = append(out, Skill{Name: name, Description: desc, Body: body, Dir: skillDir})
	}
	return out
}

// parseSkillContent extracts name/description from a "---" fenced YAML
// frontmatter block and returns the markdown body. The parser is deliberately
// minimal and dependency-free: it understands name:/description: keys with
// plain, single/double-quoted, folded (>) and literal (|) values. Content
// without a frontmatter block yields an empty name (the caller falls back to
// the directory name), an empty description and the whole content as body.
func parseSkillContent(content string) (name, description, body string) {
	lines := strings.Split(content, "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "---" {
		return "", "", content
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return "", "", content // unterminated fence → plain body
	}
	fm := lines[1:end]
	for i := 0; i < len(fm); i++ {
		key, raw, ok := strings.Cut(fm[i], ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val := strings.TrimSpace(raw)
		switch key {
		case "name":
			name = unquoteScalar(val)
		case "description":
			switch {
			case val == ">" || val == ">+":
				var parts []string
				i, parts = foldBlock(fm, i, true)
				description = strings.Join(parts, " ")
			case val == "|" || val == "|-":
				var parts []string
				i, parts = foldBlock(fm, i, false)
				description = strings.Join(parts, "\n")
			default:
				description = unquoteScalar(val)
			}
		}
	}
	body = strings.TrimLeft(strings.Join(lines[end+1:], "\n"), "\n")
	return name, description, body
}

// unquoteScalar strips a single pair of matching quotes around a scalar.
func unquoteScalar(v string) string {
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
		return v[1 : len(v)-1]
	}
	return v
}

// foldBlock consumes the indented continuation lines of a block scalar whose
// marker sits at fm[i]. It returns the index of the last consumed line and
// the trimmed continuation lines. Blank lines between indented runs continue
// the block (kept as "" for literal scalars, dropped for folded scalars).
func foldBlock(fm []string, i int, folded bool) (int, []string) {
	var out []string
	j := i + 1
	for j < len(fm) {
		line := fm[j]
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			out = append(out, strings.TrimSpace(line))
			j++
			continue
		}
		if strings.TrimSpace(line) == "" {
			k := j
			for k < len(fm) && strings.TrimSpace(fm[k]) == "" {
				k++
			}
			if k < len(fm) && (strings.HasPrefix(fm[k], " ") || strings.HasPrefix(fm[k], "\t")) {
				if !folded {
					out = append(out, "")
				}
				j = k
				continue
			}
		}
		break
	}
	return j - 1, out
}

// normalizeSkillName converts a skill name into a valid slash-command name:
// lowercase, every run of characters outside [a-z0-9] becomes "-", leading
// and trailing "-" are dropped. Returns "" when nothing usable remains
// (e.g. a CJK-only name).
func normalizeSkillName(name string) string {
	var b strings.Builder
	pendingDash := false
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if pendingDash && b.Len() > 0 {
				b.WriteByte('-')
			}
			pendingDash = false
			b.WriteRune(r)
		} else {
			pendingDash = true
		}
	}
	return b.String()
}

// userSkillsDir returns the user-level skills directory:
// C:\Users\<user>\.agents\skills (or ~/.agents/skills on unix).
func userSkillsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".agents", "skills"), nil
}

// registerSkillCommands registers every skill under ~/.agents/skills as a
// slash command (SC-6). Disabled by features.skill_loader=false; a missing
// or unreadable skills directory is silently ignored (like workspace
// history persistence).
func registerSkillCommands(r *SlashRegistry, cfg CLIConfig) {
	if !cfg.Features.SkillLoader {
		return
	}
	dir, err := userSkillsDir()
	if err != nil {
		return
	}
	registerSkillsFromDir(r, dir)
}

// registerSkillsFromDir registers all skills under dir into r (the testable
// core of registerSkillCommands). Skills are registered through
// RegisterOverlay: implemented built-in commands always win over skill names,
// while unimplemented compat stubs are replaced by the skill (e.g. a ~/vim
// skill takes over the codex stub).
//
// Only metadata (name/description/dir) is registered — the SKILL.md body is
// NOT loaded here. The body is activated lazily when the user confirms the
// command with Enter (see tuiHandleSkillFor), so startup never parses or
// retains skill content that the user never summons.
func registerSkillsFromDir(r *SlashRegistry, dir string) {
	for _, s := range LoadSkills(dir) {
		meta := Skill{Name: s.Name, Description: s.Description, Dir: s.Dir} // Body deliberately dropped
		r.RegisterOverlay(&SlashCommand{
			Name:        meta.Name,
			Description: meta.Description,
			Source:      "skill",
			Implemented: true,
			Handler:     tuiHandleSkillFor(meta),
		})
	}
}

// tuiHandleSkillFor returns the slash handler that activates a skill: it
// reads the skill's SKILL.md from disk at dispatch time (lazy activation —
// nothing is loaded until the user confirms the command with Enter) and
// renders the name, description, source path and body in the transcript
// (body capped at skillBodyMaxLines).
func tuiHandleSkillFor(s Skill) func(*chatTUI, string) (tea.Cmd, bool) {
	return func(m *chatTUI, args string) (tea.Cmd, bool) {
		path := filepath.Join(s.Dir, "SKILL.md")
		content, err := os.ReadFile(path)
		if err != nil {
			m.commitLine("")
			m.commitLine(warnText("  Skill /" + s.Name + " 无法加载: " + err.Error()))
			m.transcriptDirty = true
			return nil, false
		}
		name, desc, body := parseSkillContent(string(content))
		if name == "" {
			name = s.Name
		}
		if desc == "" {
			desc = s.Description
		}
		m.commitLine("")
		m.commitLine(accent("Skill /" + name + " 已加载"))
		if desc != "" {
			for _, l := range wrapSkillBody(desc) {
				m.commitLine(dim("  " + l))
			}
		}
		m.commitLine(dim("  来源: " + path))
		lines := strings.Split(body, "\n")
		total := len(lines)
		if total > skillBodyMaxLines {
			lines = lines[:skillBodyMaxLines]
		}
		m.commitLine("")
		for _, ln := range lines {
			for _, wrapped := range wrapSkillBody(ln) {
				m.commitLine(wrapped)
			}
		}
		if total > skillBodyMaxLines {
			m.commitLine(dim(fmt.Sprintf("  …（共 %d 行，已省略 %d 行；完整内容见上方 SKILL.md）",
				total, total-skillBodyMaxLines)))
		}
		m.transcriptDirty = true
		return nil, false
	}
}

// wrapSkillBody word-wraps a single line at skillBodyWidth runes so long
// markdown lines (code blocks, URLs) never overflow the viewport. Empty
// lines pass through untouched.
func wrapSkillBody(line string) []string {
	if line == "" {
		return []string{line}
	}
	runes := []rune(line)
	if len(runes) <= skillBodyWidth {
		return []string{line}
	}
	var out []string
	for len(runes) > skillBodyWidth {
		out = append(out, string(runes[:skillBodyWidth]))
		runes = runes[skillBodyWidth:]
	}
	if len(runes) > 0 {
		out = append(out, string(runes))
	}
	return out
}
