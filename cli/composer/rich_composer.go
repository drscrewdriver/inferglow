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

// Package composer 实现确定性的富输入状态机。
//
// 本包是纯函数式裁决层：不直接修改 UI、不持有全局状态、不产生副作用，
// 只把输入事件翻译成离散的动作（Action），由调用方决定如何应用到界面。
// 唯一的外部依赖是标准库 time。
package composer

import "time"

// Mode 描述输入状态机当前所处的模式（对应 PRD §3 M0-M4）。
type Mode int

const (
	ModeIdle          Mode = iota // M0 空输入区，等待首字符
	ModePasteHold                 // M2 暂存首字符，防闪烁
	ModeTyping                    // M1 判定非突发，逐字符 Typed
	ModePasteBurst                // M3 突发缓冲中，整体将作为一次 Paste
	ModeEnterSuppress             // M4 突发窗口内，Enter 视为换行
)

// Config 可配置时间参量（含默认值）。
type Config struct {
	// BurstCharInterval 相邻字符间隔判突发的上界，默认 2ms。
	// 判定语义固定：interval > BurstCharInterval → 非突发（严格大于）。
	BurstCharInterval time.Duration
	// BurstActiveIdleTimeout 停止收集后冲刷为 Paste 的等待，默认 10ms。
	BurstActiveIdleTimeout time.Duration
	// EnterSuppressWindow 突发冲刷后 Enter 抑制窗口，默认 10ms。
	EnterSuppressWindow time.Duration
}

// DefaultConfig 返回默认时间参量。
func DefaultConfig() Config {
	return Config{
		BurstCharInterval:    2 * time.Millisecond,
		BurstActiveIdleTimeout: 10 * time.Millisecond,
		EnterSuppressWindow:  10 * time.Millisecond,
	}
}

// EventKind 输入事件类型（对应 PRD §4）。
type EventKind int

const (
	EventPlainChar      EventKind = iota // IN-1 ASCII 文本键
	EventPlainCharNoHold                 // IN-2 非 ASCII / IME（不暂存、不参与突发）
	EventEnter                           // IN-3 换行/提交键
	EventBracketedPaste                  // IN-4 终端 bracketed-paste 整体到达
	EventModifiedInput                   // IN-5 方向键 / Ctrl+Alt / Super/Hyper/Meta
	EventTick                            // IN-6 时序 flush
)

// Event 一次输入事件。Text 仅对 PlainChar/PlainCharNoHold/BracketedPaste 有意义。
type Event struct {
	Kind EventKind
	Text string
	Now  time.Time
}

// ActionKind 输出动作（对应 PRD §5）。
type ActionKind int

const (
	ActionTyped          ActionKind = iota // OUT-1 普通键入，逐字符插入
	ActionPaste                            // OUT-2 整体作为一段粘贴注入（含内部换行）
	ActionInsertNewline                    // OUT-3 粘贴内 Enter → 换行
	ActionSubmit                           // OUT-4 非突发上下文的显式 Enter 提交
	ActionBufferDiscard                    // OUT-5 非文本输入时丢弃缓存
)

// Action 一次裁决动作。
type Action struct {
	Kind ActionKind
	Text string // Typed/Paste 的文本载荷
}

// Composer 是确定性的富输入状态机。纯函数：不直接修改 UI，仅产出 Action 供调用方应用。
type Composer struct {
	mode             Mode
	buffer           []rune
	pendingFirstChar rune   // 0 表示无暂存
	lastCharAt       time.Time
	burstWindowUntil time.Time
	cfg              Config
}

// New 构造一个 Composer。cfg 中任何 <=0 的字段回退到 DefaultConfig 对应默认值。
func New(cfg Config) *Composer {
	def := DefaultConfig()
	if cfg.BurstCharInterval <= 0 {
		cfg.BurstCharInterval = def.BurstCharInterval
	}
	if cfg.BurstActiveIdleTimeout <= 0 {
		cfg.BurstActiveIdleTimeout = def.BurstActiveIdleTimeout
	}
	if cfg.EnterSuppressWindow <= 0 {
		cfg.EnterSuppressWindow = def.EnterSuppressWindow
	}
	return &Composer{cfg: cfg}
}

// Mode 返回当前模式。
func (c *Composer) Mode() Mode { return c.mode }

// Pending 返回当前突发缓冲文本（空串表示无）。
func (c *Composer) Pending() string { return string(c.buffer) }

// Reset 清空到 idle（会话恢复等）。
func (c *Composer) Reset() {
	c.mode = ModeIdle
	c.buffer = nil
	c.pendingFirstChar = 0
	c.burstWindowUntil = time.Time{}
}

// TickInterval 返回突发活动空闲超时（供 TUI 编排 flush tick）。
func (c *Composer) TickInterval() time.Duration { return c.cfg.BurstActiveIdleTimeout }

// firstRune 返回字符串的首个 rune；空串返回 0。
func firstRune(s string) rune {
	for _, r := range s {
		return r
	}
	return 0
}

// Feed 喂入一个输入事件，返回零个或多个裁决动作。
func (c *Composer) Feed(ev Event) []Action {
	switch ev.Kind {
	case EventBracketedPaste:
		// 整体清空突发/暂存态，直接作为一次 Paste。
		c.mode = ModeIdle
		c.buffer = nil
		c.pendingFirstChar = 0
		c.burstWindowUntil = time.Time{}
		return []Action{{ActionPaste, ev.Text}}

	case EventPlainCharNoHold:
		// 非 ASCII / IME：不暂存、不参与突发，一次 Typed 携带全部 rune。
		var acts []Action
		if c.pendingFirstChar != 0 {
			acts = append(acts, Action{ActionTyped, string(c.pendingFirstChar)})
			c.pendingFirstChar = 0
		}
		if c.mode != ModePasteBurst {
			c.mode = ModeIdle
		}
		return append(acts, Action{ActionTyped, ev.Text})

	case EventPlainChar:
		ch := firstRune(ev.Text)
		switch c.mode {
		case ModeIdle, ModeEnterSuppress:
			c.pendingFirstChar = ch
			c.lastCharAt = ev.Now
			c.mode = ModePasteHold
			return nil

		case ModePasteHold:
			interval := ev.Now.Sub(c.lastCharAt)
			if interval <= c.cfg.BurstCharInterval {
				c.buffer = []rune{c.pendingFirstChar, ch}
				c.pendingFirstChar = 0
				c.lastCharAt = ev.Now
				c.burstWindowUntil = ev.Now.Add(c.cfg.EnterSuppressWindow)
				c.mode = ModePasteBurst
				return nil
			}
			prev := c.pendingFirstChar
			c.pendingFirstChar = ch
			c.lastCharAt = ev.Now
			c.mode = ModeTyping
			return []Action{{ActionTyped, string(prev)}}

		case ModeTyping:
			interval := ev.Now.Sub(c.lastCharAt)
			if interval <= c.cfg.BurstCharInterval {
				c.buffer = []rune{c.pendingFirstChar, ch}
				c.pendingFirstChar = 0
				c.lastCharAt = ev.Now
				c.burstWindowUntil = ev.Now.Add(c.cfg.EnterSuppressWindow)
				c.mode = ModePasteBurst
				return nil
			}
			prev := c.pendingFirstChar
			c.pendingFirstChar = ch
			c.lastCharAt = ev.Now
			c.mode = ModeTyping
			return []Action{{ActionTyped, string(prev)}}

		case ModePasteBurst:
			interval := ev.Now.Sub(c.lastCharAt)
			if interval <= c.cfg.BurstCharInterval {
				c.buffer = append(c.buffer, ch)
				c.lastCharAt = ev.Now
				c.burstWindowUntil = ev.Now.Add(c.cfg.EnterSuppressWindow)
				return nil
			}
			txt := string(c.buffer)
			c.buffer = nil
			c.pendingFirstChar = ch
			c.lastCharAt = ev.Now
			c.mode = ModeTyping
			return []Action{{ActionPaste, txt}}
		}
		return nil

	case EventEnter:
		if len(c.buffer) > 0 {
			// 突发进行中：Enter 变成粘贴内的换行，绝不 Submit。
			c.buffer = append(c.buffer, '\n')
			return nil
		}
		var acts []Action
		if c.pendingFirstChar != 0 {
			// 先冲刷暂存字符，防丢失。
			acts = append(acts, Action{ActionTyped, string(c.pendingFirstChar)})
			c.pendingFirstChar = 0
		}
		if !c.burstWindowUntil.IsZero() && !ev.Now.After(c.burstWindowUntil) {
			// 突发窗口内：Enter 视为换行（不改 mode/不清窗口）。
			return append(acts, Action{Kind: ActionInsertNewline})
		}
		// 窗口外：Submit 并复位到 idle。
		c.mode = ModeIdle
		c.burstWindowUntil = time.Time{}
		return append(acts, Action{Kind: ActionSubmit})

	case EventModifiedInput:
		// 先冲刷，再重置；调用方随后执行原动作。
		var acts []Action
		if len(c.buffer) > 0 {
			acts = append(acts, Action{ActionPaste, string(c.buffer)})
		}
		if c.pendingFirstChar != 0 {
			acts = append(acts, Action{ActionTyped, string(c.pendingFirstChar)})
		}
		c.buffer = nil
		c.pendingFirstChar = 0
		c.burstWindowUntil = time.Time{}
		c.mode = ModeIdle
		return acts

	case EventTick:
		// 按超时冲刷。
		switch c.mode {
		case ModePasteBurst:
			if ev.Now.Sub(c.lastCharAt) >= c.cfg.BurstActiveIdleTimeout {
				txt := string(c.buffer)
				c.buffer = nil
				c.burstWindowUntil = ev.Now.Add(c.cfg.EnterSuppressWindow)
				c.mode = ModeEnterSuppress
				return []Action{{ActionPaste, txt}}
			}
		case ModePasteHold, ModeTyping:
			if ev.Now.Sub(c.lastCharAt) >= c.cfg.BurstActiveIdleTimeout {
				ch := c.pendingFirstChar
				c.pendingFirstChar = 0
				c.mode = ModeIdle
				return []Action{{ActionTyped, string(ch)}}
			}
		case ModeEnterSuppress:
			if !c.burstWindowUntil.IsZero() && ev.Now.After(c.burstWindowUntil) {
				c.burstWindowUntil = time.Time{}
				c.mode = ModeIdle
				return nil
			}
		}
		return nil
	}
	return nil
}
