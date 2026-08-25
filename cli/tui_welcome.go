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
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
// MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO
// EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES
// OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
// ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
// DEALINGS IN THE SOFTWARE.

package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// welcomeTip is one usage tip (RF-9). Groups mirror dsh-tui tips.ts:
// keys / commands / workflow / display / pitfalls. Each tip is ≤60 chars.
type welcomeTip struct {
	id    string
	group string
	text  string
}

// welcomeTips is the built-in tip pool. Extend with new features.
var welcomeTips = []welcomeTip{
	{"keys-history", "keys", "↑ 召回输入历史（重启后依然可用）"},
	{"keys-ctrl-o", "keys", "Ctrl+O 显示/隐藏思考过程"},
	{"keys-ctrl-r", "keys", "Ctrl+R 切换富文本/原文渲染"},
	{"cmd-model", "commands", "/model 切换模型 · /model status 查看路由"},
	{"cmd-effort", "commands", "/effort high 提高思考等级 · /effort auto 恢复默认"},
	{"cmd-workspace", "commands", "/workspace 切换目录 · /cd - 回到上一个"},
	{"cmd-theme", "commands", "/theme light|dark|auto 切换主题"},
	{"cmd-tips", "commands", "/tips 随时查看提示 · /tips display 过滤分组"},
	{"wf-receipt", "workflow", "/receipt 查看本轮耗时/工具/tps/缓存命中"},
	{"wf-tasks", "workflow", "/tasks 任务面板 · /mode 切换上下文管理模式"},
	{"disp-health", "display", "状态栏 ●=API在线 ○=离线 · /health 手动探活"},
	{"disp-tps", "display", "状态栏 tps 实时效率 · /tps 查看 avg60/p95"},
	{"pitfall-restart", "pitfalls", "模型切换立即生效；编辑 config 后需重启"},
	{"pitfall-config", "pitfalls", "providers.list 配置多路 API · llm 仍兼容单路"},
}

const (
	welcomeSeenFile = "welcome_seen.json"
	welcomePageSize = 5
)

// welcomeSeenFlag is the persisted first-run marker.
type welcomeSeenFlag struct {
	Seen bool `json:"seen"`
}

// tuiWelcome is the startup welcome page state (RF-9).
type tuiWelcome struct {
	visible bool
	page    int
	group   string // "" = all groups
}

// welcomeGroupOrder is the canonical display order for tip groups.
var welcomeGroupOrder = []string{"keys", "commands", "workflow", "display", "pitfalls"}

// welcomeGroupLabel returns the display label for a group.
func welcomeGroupLabel(group string) string {
	labels := map[string]string{
		"keys":     "快捷键",
		"commands": "命令",
		"workflow": "工作流",
		"display":  "界面",
		"pitfalls": "避坑",
	}
	if l, ok := labels[group]; ok {
		return l
	}
	return group
}

// tipsForGroup filters the tip pool by group ("" = all, grouped order).
func tipsForGroup(group string) []welcomeTip {
	if group == "" {
		return welcomeTips
	}
	var out []welcomeTip
	for _, t := range welcomeTips {
		if t.group == group {
			out = append(out, t)
		}
	}
	return out
}

// welcomeGroups returns the distinct groups present in the filtered tips.
func welcomeGroups(tips []welcomeTip) []string {
	seen := map[string]bool{}
	var out []string
	for _, g := range welcomeGroupOrder {
		for _, t := range tips {
			if t.group == g && !seen[g] {
				seen[g] = true
				out = append(out, g)
				break
			}
		}
	}
	return out
}

// fireflyGlow 是 logo 上方的萤火虫光点装饰行（暖光色）。
const fireflyGlow = "      ✦        ✧          ✦         ✧        ✦"

// inferglowLogo 是欢迎页的 ASCII 大字 logo（ANSI Shadow 风格 "INFERGLOW"，
// 由生成脚本校验行宽 ≤81；渲染时按终端宽度紧凑截断兜底）。
var inferglowLogo = []string{
	"██╗ ███╗   ██╗ ███████╗ ███████╗ ██████╗   ██████╗ ██╗       ██████╗ ██╗    ██╗",
	"██║ ████╗  ██║ ██╔════╝ ██╔════╝ ██╔══██╗ ██╔════╝ ██║      ██╔═══██╗ ██║    ██║",
	"██║ ██╔██╗ ██║ █████╗   █████╗   ██████╔╝ ██║  ███╗ ██║      ██║   ██║ ██║ █╗ ██║",
	"██║ ██║╚██╗██║ ██╔══╝   ██╔══╝   ██╔══██╗ ██║   ██║ ██║      ██║   ██║ ██║███╗██║",
	"██║ ██║ ╚████║ ██║      ███████╗ ██║  ██║ ╚██████╔╝ ███████╗ ╚██████╔╝ ╚███╔███╔╝",
	"╚═╝ ╚═╝  ╚═══╝ ╚═╝      ╚══════╝ ╚═╝  ╚═╝  ╚═════╝ ╚══════╝  ╚═════╝  ╚══╝╚══╝",
}

// renderWelcome renders the welcome/tips panel. Returns "" when not visible
// or when the terminal width is not yet known (Bubble Tea renders the first
// frame with width=0 before the first WindowSizeMsg arrives — rendering a
// panel there would panic on negative Repeat counts and draw garbage).
func (m *chatTUI) renderWelcome(width int) string {
	if !m.welcome.visible {
		return ""
	}
	if width < 10 {
		return "" // wait for a real terminal size
	}
	if width > 88 {
		width = 88
	}
	tips := tipsForGroup(m.welcome.group)
	pages := (len(tips) + welcomePageSize - 1) / welcomePageSize
	if pages == 0 {
		pages = 1
	}
	if m.welcome.page >= pages {
		m.welcome.page = pages - 1
	}
	start := m.welcome.page * welcomePageSize
	end := start + welcomePageSize
	if end > len(tips) {
		end = len(tips)
	}

	var sb strings.Builder
	// 萤火虫大字 logo（仅当终端宽度足够；窄终端跳过，提示框照常显示）。
	if width >= 84 {
		sb.WriteString(warnText(fireflyGlow) + "\n")
		for _, ln := range inferglowLogo {
			sb.WriteString(accent(compactMiddle(ln, width-2)) + "\n")
		}
		sb.WriteString("\n")
	}
	title := "InferGlow 快速上手"
	if m.welcome.group != "" {
		title = "Tips · " + welcomeGroupLabel(m.welcome.group)
	}
	sb.WriteString(accent("┌─ " + title + " "))
	sb.WriteString(accent(strings.Repeat("─", max(width-visibleWidth(title)-6, 1))))
	sb.WriteString(accent("┐\n"))
	shown := tips[start:end]
	if len(shown) == 0 {
		shown = welcomeTips
		start = 0
		end = len(welcomeTips)
	}
	// Group headers in canonical order within the page.
	groupIdx := map[string]bool{}
	for _, g := range welcomeGroups(shown) {
		groupIdx[g] = true
	}
	for _, t := range shown {
		if groupIdx[t.group] {
			sb.WriteString(dim(fmt.Sprintf("  [%s]\n", welcomeGroupLabel(t.group))))
			groupIdx[t.group] = false
		}
		sb.WriteString(compactMiddle("  · "+t.text, width-2) + "\n")
	}
	sb.WriteString(accent("└" + strings.Repeat("─", max(width-2, 1)) + "┘\n"))
	if pages > 1 {
		sb.WriteString(dim(fmt.Sprintf("  %d/%d 页 · Tab 翻页 · Esc 关闭\n", m.welcome.page+1, pages)))
	} else {
		sb.WriteString(dim("  Esc 关闭 · /tips 随时查看\n"))
	}
	return sb.String()
}

// welcomeSeenPath returns the first-run marker path.
func welcomeSeenPath() string {
	return filepath.Join(prefsDir(), welcomeSeenFile)
}

// markWelcomeSeen writes the first-run marker (best-effort).
func markWelcomeSeen() {
	data, err := json.Marshal(welcomeSeenFlag{Seen: true})
	if err != nil {
		return
	}
	if dir := filepath.Dir(welcomeSeenPath()); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return
		}
	}
	_ = os.WriteFile(welcomeSeenPath(), data, 0o644)
}

// welcomeSeenFrom reports whether the marker file exists (corrupt = seen,
// to avoid nagging).
func welcomeSeenFrom(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var f welcomeSeenFlag
	if err := json.Unmarshal(data, &f); err != nil {
		return true
	}
	return f.Seen
}

// tuiHandleWelcome re-shows the welcome page (/welcome).
func tuiHandleWelcome(m *chatTUI, args string) (tea.Cmd, bool) {
	m.welcome.visible = true
	m.welcome.page = 0
	m.welcome.group = ""
	m.transcriptDirty = true
	return nil, false
}

// tuiHandleTips handles /tips (RF-9):
//
//	/tips          → show all groups (paginated)
//	/tips <group>  → filter to one group
func tuiHandleTips(m *chatTUI, args string) (tea.Cmd, bool) {
	args = strings.TrimSpace(args)
	if args != "" {
		valid := false
		for _, g := range welcomeGroupOrder {
			if args == g {
				valid = true
				break
			}
		}
		if !valid {
			m.commitLine("")
			m.commitLine(errorText("  Unknown tip group: " + args))
			m.commitLine(dim("  Groups: keys | commands | workflow | display | pitfalls"))
			return nil, false
		}
	}
	m.welcome.group = args
	m.welcome.page = 0
	m.welcome.visible = true
	m.transcriptDirty = true
	return nil, false
}
