# InferGlow TUI 富输入 Composer — 需求与设计文档 (PRD)

> 版本: v0.4
> 状态: 评审修订版；非 input 线需求已拆分至 docs/requirements/（需求族文件夹）
> 范围: inferglow `cli/` TUI 层（Go，Charm bubbletea/v2 + bubbles/v2 + lipgloss/v2）
> 参考实现: OpenAI Codex TUI ChatComposer（`codex/codex-rs/tui/src/bottom_pane/chat_composer.rs` + `paste_burst.rs`，ratatui/crossterm）

## 0. TL;DR

> **v0.4 变更记录**（2026-08-22，需求族拆分）：非 input 线需求整体迁出至
> [docs/requirements/](requirements/) 文件夹按需求族管理——媒体能力门控（原 §13.1 + §11.3 缺口 1）、
> provider 配置存储（原 §13.2）、严格 lint 范围（原 §13.3 + §12.1）、TUI 能力 backlog
> （原 §12 #2/#3/#5 + §11.4 展望）。本 PRD 自此只承载 input 线需求，§13 改为关联需求族指针表。
>
> **v0.3 变更记录**（2026-08-22，对照代码库逐条核查后修订）：
> - **状态对齐**：§13.1 媒体能力门控与 §13.2 config 长期存储域**已实现交付**（`model/content_block.go` 的 `gateMultimodal` + provider 穿透；`cli/config/` + `cli/configure.go`），相关章节由「建议方案」改为「已交付记录」；§11.3 缺口 1 同步关闭。
> - **矛盾修正**：① M1 触发条件原「< 突发阈值」与 M3「≤ 突发阈值」同侧冲突，改为「> 阈值」；② P2 里程碑原声称覆盖验收 8/9，实归 P0；③ §13.2 优先级原文「env 仍最高」与实现（providers.yaml 激活 provider 优先，回退 env/config.json/flags）相反，按实现修正；④ §13.3 严格 lint 原方案覆盖不到 composer 代码（golangci-lint 按包粒度启用 linter，cli 根包几十个存量文件必被连带），改为 composer 独立子包 `cli/composer/`。
> - **决策缺口补齐**：IN-7/IN-8 补用户触发入口与剪贴板双向裁决；时间参量补默认值建议；图片 vision 门控约束正式并入 §7；P0 覆盖率门槛（≥90%）入 §8 验收 9。
> - **范围澄清**：§12 盘点的 ask 建议 / 对话恢复 / TDD·证据门·视觉桥明确为**后续阶段**，不占本 PRD 里程碑。

给 inferglow 的 REPL 输入区引入「富输入 Composer」：一套**确定性的输入状态机**，统一处理
普通键入、多行粘贴（含无 bracketed-paste 终端的突发流）、剪贴板复制（文本）、图片粘贴转附件、
以及 Rich/Raw 渲染切换。核心诉求是**让“输入”可预测**：粘贴是粘贴，输入是输入，绝不互相污染。

文档以「**模式（Mode）× 输入事件（Input）→ 输出行为（Output）**」为骨架。

---

## 1. 背景与问题

现状：inferglow TUI 是内联派（Bubble Tea 直接消费 `orchestrator/agent` 事件），输入区只有
基础的 `textarea`。存在以下问题：

1. **无 bracketed-paste 兜底**：在 Windows / 部分终端上，粘贴多行会变成一串连发的
   `Char`+`Enter` 按键流。当前实现会在每次 `Enter` 处触发提交 → 一次粘贴多次发送 / 内容被拆碎。
2. **粘贴内容误触快捷键**：粘贴文本里的 `?` 等字符会意外触发面板/帮助切换。
3. **无“复制”通道**：无法把代码块/文本写回系统剪贴板。
4. **无图片通道**：无法把剪贴板图片作为附件喂给 `ask`/视觉类能力。
5. **渲染只有一种**：长 Set 转 Diff、Markdown 没有 rich/raw 切换。

本 PRD 用单一状态机一次性覆盖 1–4，用独立渲染维度覆盖 5。

---

## 2. 总体架构

```
终端事件流 ──► Input 状态机 (RichComposer)
                 │  mode 字段: idle / typing / paste_hold / paste_burst / enter_suppress
                 │  buffer / pending_first_char / timestamps
                 ▼
           Bubble Tea update() 分派
                 ├─► 文本插入 textarea
                 ├─► 作为独立 Paste 注入
                 └─► 剪贴板 / 图片 / 渲染副作用
```

- **状态机是纯函数**：Composer 不改 textarea，只做“裁决”，由调用方应用动作（同 Codex
  paste_burst 的“pure state machine”原则）。
- **渲染维度与输入状态解耦**：`render mode ∈ {rich, raw}` 是独立显示状态，不影响输入裁决。

---

## 3. 模式定义（状态）

| ID | 模式 | 触发 | 说明 | 关键字段 |
|----|------|------|------|---------|
| M0 | `idle` | 初始 / 缓冲清空后 | 空输入区，等待首字符 | `active=false, buffer=""` |
| M1 | `typing` | M2 暂存字符的次字符间隔 **>** 突发阈值（判定非突发） | 正常键入：暂存字符 flush 为 `Typed`，后续字符直接 `Typed` | `pending_first_char=""` |
| M2 | `paste_hold` | 收到首 ASCII 字符 | **防闪烁**：暂存首个字符（若立即上屏、稍后判定为突发又需撤销重排，会闪烁），等后续字符判定是突发还是单键 | `pending_first_char=ch` |
| M3 | `paste_burst` | 连续字符间隔 ≤ 突发阈值 | 突发缓冲，整体作为一个 Paste | `buffer+=ch, active=true` |
| M4 | `enter_suppress` | 突发缓存非空 / 突发窗口内 | **Enter 抑制**：视为换行而非提交 | `burst_window_until` |
| M5 | `image_paste` | 剪贴板含图片 | 取图 → 编码 PNG → 临时文件 → 附件 | 临时 PNG 路径 |

> 注：M5 是事件型脉冲（不持续），M2–M4 是时序型持续状态。M0/M1 是常规态。

---

## 4. 输入事件清单（Input）

| 编号 | 事件 | 类型 | 说明 |
|------|------|------|------|
| IN-1 | `plain_char` | ASCII 文本键 | 可参与突发判定的普通字符 |
| IN-2 | `plain_char_no_hold` | 非 ASCII / IME | 不暂存，直接可判定（中文输入法） |
| IN-3 | `enter` | 换行/提交键 | 需裁决“换行 or 提交” |
| IN-4 | `bracketed_paste(text)` | 粘贴事件 | 终端支持 bracketed-paste 时整体到达 |
| IN-5 | `modified_input` | 方向键 / Ctrl+Alt / Super/Hyper/Meta | 非文本输入，需先冲刷缓冲 |
| IN-6 | `tick` | 时序（flush timeout） | 触发突发刷新 `flush_if_due` |
| IN-7 | `clipboard_text` | 剪贴板 | 读系统剪贴板文本 |
| IN-8 | `clipboard_image` | 剪贴板 | 读系统剪贴板图片 |
| IN-9 | `render_toggle` | 用户命令 | Rich/Raw 切换 |

时间参量（可配置，含默认值建议）：
- `PASTE_BURST_CHAR_INTERVAL`：相邻字符间隔判突发的上界，默认 **2ms**。依据：人类连续键入相邻间隔通常 > 20ms，终端粘贴连发相邻间隔 < 1ms，2ms 有充分分离度。
- `PASTE_BURST_ACTIVE_IDLE_TIMEOUT`：停止收集后冲刷为 Paste 的等待，默认 **10ms**。
- 判定语义固定：`interval > 阈值` → 非突发（严格大于），便于测试用「阈值+1ms」构造跨边界用例。

---

## 5. 输出行为清单（Output）

| 编号 | 事件 | 目标 | 说明 |
|------|------|------|------|
| OUT-1 | `Typed(ch)` | textarea | 判定为普通键入，立即插入 |
| OUT-2 | `Paste(text)` | textarea | 整体作为一段粘贴注入（含内部换行） |
| OUT-3 | `InsertNewline` | textarea | 粘贴内 Enter → 换行（M4 抑制） |
| OUT-4 | `Submit(message)` | 会话 | **仅**在非突发上下文的显式 Enter 时提交 |
| OUT-5 | `BufferDiscard` | — | 出现非文本输入时丢弃缓存，避免“卡住” |
| OUT-6 | `ClipboardWrite(text)` | 系统剪贴板 | 文本/代码块复制（含 WSL、macOS NSPasteboard 处理） |
| OUT-7 | `AttachImage(pngPath)` | 会话 | 图片转 PNG 临时文件并作为附件 |
| OUT-8 | `RenderMode(mode)` | 视图 | rich/raw 切换 |

### 核心派生（决策表）

- 突发上下文内 `Enter` → **OUT-3**（换行），绝不触发 OUT-4。
- 突发冲刷后 → **OUT-2**（完整 Paste），并进入 `enter_suppress` 窗口。
- 非突发、空缓冲时 `Enter` → **OUT-4**（提交）。
- 任何 `modified_input`（IN-5）→ 先 **OUT-5**（冲刷并清窗口），再执行原动作。
- **剪贴板双向，触发入口必须分开**：
  - 用户按「粘贴」键（建议 `Ctrl+V`，兜底 `Ctrl+Y`——部分 Windows 终端拦截 Ctrl+V）→ 主动读剪贴板：含图片 → IN-8 → **OUT-7**；纯文本 → 作为一次显式 Paste 注入 textarea（走 OUT-2）。
  - 用户按「复制」键（建议 `Ctrl+C`，**仅当存在选区时**劫持；无选区不改变 Ctrl+C 原语义）→ **OUT-6**，把选区写回系统剪贴板。
  - 裁决规则：IN-7/IN-8 由**用户显式按键**触发读取，与粘贴突发状态机（M0–M4）正交，不参与突发判定；突发进行中的剪贴板读取先冲刷缓冲（同 IN-5 语义）。

---

## 6. 与现有 inferglow 代码的映射

| 现有文件 | 承接职责 |
|----------|---------|
| `cli/tui_model.go`（`tuiState` idle/running） | 仅保留“会话是否在等 agent 回复”的高层态；输入态迁移到新 Composer 状态机 |
| `cli/repl.go` | 事件入口改为先喂 RichComposer，再走 Bubble Tea update |
| `cli/tui_view.go` / `tui_footer.go` | 渲染层，挂 OUT-8 rich/raw |
| `cli/tui_approval.go` | 与 OUT-4 Submit 串联（提交带审批） |
| `cli/tui_theme.go` | 复用主题 token 渲染粘贴卡片 / 附件提示 |

新增：
- `cli/composer/` 子包 —— 状态机独立成包：`rich_composer.go`（纯函数，含字段
  `mode/buffer/pending_first_char/burst_window_until`）+ `rich_composer_test.go`（覆盖第 8 节全部验收用例）。
  **放子包而非 cli 根文件的原因**：golangci-lint 以包为粒度启用 linter，composer 代码必须进
  errcheck 严格域（§12.1 + 需求族 [严格 lint 范围](requirements/strict-lint-scope.md)），放 cli 根包会连带几十个存量文件一起被严格扫描、撑爆 CI；
  独立子包与 `cli/config` 同模式。
- `cli/clipboard.go` —— 文本/图片剪贴板桥（Go 侧 `golang.design/x/clipboard` 或平台胶水）。

---

## 7. 边界与异常

- **Android / Termux**：图片剪贴板不可用 → 返回明确 `ClipboardUnavailable` 错误，不静默失败。
- **WSL**：图片粘贴走 `/mnt/c` 回退路径。
- **IME/中文输入**：非 ASCII 走 `no_hold` 路径，不参与突发判定，保证中文输入流畅、不误判为粘贴。
- **粘贴中出现快捷键字符（如 `?`）**：在突发上下文中一律按文本处理，不触发面板副作用。
- **粘贴被方向键/快捷键打断**：先冲刷并清窗口，避免把后续键入并入上一个突发。
- **终端 resize**：不影响 Composer 状态机（纯文本裁决），仅触发重绘。
- **图片附件 × 非 vision 模型（双层门控）**：TUI 层在 M5 挂附件后、OUT-4 提交前用
  `model.SupportsVision(modelName)` 做提示性预检——已知非 vision → 明确提示并要求确认或移除；
  模型层 `gateMultimodal` 兜底拒绝（已实现，见需求族 [媒体能力门控](requirements/media-capability-gating.md)），保证任何上层路径都无法绕过。
  未知/自定义模型放行，交由上游 provider 裁决。

---

## 8. 验收标准

1. 无 bracketed-paste 终端粘贴多行代码：`Enter` 全部为换行，消息仅提交一次（UI 快照 + 断言）。
2. 粘贴内含 `?`：不触发帮助/面板切换。
3. 单字符慢速键入：与突发间隔 > 阈值时，逐字符 `Typed`，无闪烁。
4. 突发后紧跟 Enter：若窗口内为换行，超窗口为提交。
5. 复制代码块 → 系统剪贴板可粘贴到外部应用。
6. 剪贴板放图 → 会话产生一个附件，且不产生文本粘贴。
7. `render_toggle` 在 rich/raw 间切换且输入状态不受影响。
8. IME 中文输入不触发突发/被误判。
9. 状态机纯函数：相同事件序列 → 相同裁决（快照测试）；`cli/composer/rich_composer_test.go` 行覆盖率 ≥ 90%（P0 硬门槛，纯状态机易测）。
10. 已知非 vision 模型 + 图片附件：提交前 TUI 出现可读预检提示，不静默发送 base64；即便绕过提示，模型层 `gateMultimodal` 也返回 `ErrUnsupportedContent`（已实现，见需求族 [媒体能力门控](requirements/media-capability-gating.md)）。

---

## 9. 里程碑

- P0（核心）：M0–M4 + IN-1..6/OUT-1..5 —— 粘贴突发与 Enter 抑制，覆盖验收 1/2/3/4/8/9（IME 判定与纯函数快照均为状态机自身职责）。
- P1（剪贴板）：IN-7/8 + OUT-6/7 —— 文本/图片剪贴板，覆盖验收 5/6/10（含 vision 预检提示）。
- P2（渲染）：IN-9 + OUT-8 —— rich/raw 切换，覆盖验收 7。
- **后续阶段（不在本 PRD 里程碑，另行立项）**：ask 建议工具、对话恢复的 agent 重建、TDD 工作流/证据门/视觉桥——已移交需求族 [TUI 能力 backlog](requirements/tui-capability-backlog.md)，本 PRD 只交付其依赖的输入底座。

---

## 10. 参考实现对照

| 本 PRD 概念 | Codex 实现参照 |
|-------------|---------------|
| 突发状态机 / Enter 抑制 | `codex/codex-rs/tui/src/bottom_pane/paste_burst.rs` |
| ChatComposer 编排 | `codex/codex-rs/tui/src/bottom_pane/chat_composer.rs` |
| 剪贴板复制（WSL/macOS） | `codex/codex-rs/tui/src/clipboard_copy.rs` |
| 图片粘贴转 PNG | `codex/codex-rs/tui/src/clipboard_paste.rs` |
| Rich/Raw 渲染 | `codex/codex-rs/tui/src/chatwidget.rs`（`HistoryRenderMode::Rich`） |

---

## 11. 同项目依赖模块审计：LLM provider 处理层与图片支持

本节审计富输入 Composer 的「图片附件 → 模型请求」链路在现有 `model/` **模型层**的支撑程度。
结论先行：**模型层已经完整支持多模态图片，但缺「能力门控」与「上游输入通道」** —— PRD 的
M5/视觉桥只需补齐 TUI 侧输入，不需改模型层。

### 11.1 统一多模态内容模型（已具备 ✅）

- 载体：[content_block.go](file:///e:/test/rewrite-agently/inferglow/model/content_block.go) 的
  `ContentBlock{Type, MIMEType, Data, URL, Meta}`，覆盖 `text / image / audio / video / file`。
- 提供 inline 字节与 remote URL 两种形态，且二者互斥（`IsInline()` / `IsRemote()`）。
- 便捷构造器：`ImageBlock(mime, data)`（内联字节）、`ImageURLBlock(url)`（远端引用）。
- `ChatMessage.ContentBlocks []ContentBlock` 支持一次携带多块多模态内容（多图）。

### 11.2 按 provider 的图片编码（三类已实现 ✅）

图片在**模型层**已被各 provider 序列化为其原生格式：

| Provider | 序列化位置 | 图片编码 |
|----------|-----------|---------|
| OpenAI / OpenRouter 兼容 | [chat.go](file:///e:/test/rewrite-agently/inferglow/model/chat.go) `ChatMessage.MarshalJSON` | 内联→`{"type":"image_url","image_url":{"url":"data:<mime>;base64,..."}}`；远端→直接 URL。缺省 `mime` 回退 `image/png` |
| Anthropic 兼容 | [anthropic.go](file:///e:/test/rewrite-agently/inferglow/model/anthropic.go) `anthropicMessages` | 内联→`{"type":"image","source":{"type":"base64","media_type":...,"data":...}}`；远端→`source.type=url` |
| 音频（OpenAI GPT-4o-audio） | 同 `chat.go` | `{"type":"input_audio","input_audio":{"data":...,"format":...}}` |

> 说明：openai.go 本体不含图片逻辑，图片统一收口在 `ChatMessage.MarshalJSON`（通用 JSON 序列化层），
> 因此所有走 OpenAI wire 协议的 provider（OpenAI/Ollama/OpenRouter）自动共享该编码。

### 11.3 模型能力注册表（已具备，但**未强制门控** ⚠️）

- [capability.go](file:///e:/test/rewrite-agently/inferglow/model/capability.go) 内置
  `ModelCapabilityRegistry`（含 `Vision/Audio/Video/ToolCalling/JSONMode/Streaming/MaxContext`），
  并暴露 `SupportsVision(name)`、`LookupModelCapability(name)`。
- 覆盖 OpenAI/Anthropic/Gemini/DeepSeek/Qwen/GLM/Kimi/Cohere 全系，含 vision 标记
  （`gpt-4o/o1/o3/o4-mini`、`claude-3-*`、`gemini-2.x`、`deepseek-vl2`、`qwen-vl-*`、`glm-4v`、`kimi-latest`）。

**缺口（v0.3 更新：缺口 1 已关闭 ✅，缺口 2/3 仍待 P1/P2）**：
1. ~~图片发送前无能力校验~~ → **已实现**：门控收口在模型层 `gateMultimodal`
   （[content_block.go](file:///e:/test/rewrite-agently/inferglow/model/content_block.go)），OpenAI 兼容
   （[openai.go](file:///e:/test/rewrite-agently/inferglow/model/openai.go)）与 Anthropic 兼容
   （[anthropic.go](file:///e:/test/rewrite-agently/inferglow/model/anthropic.go)）的 `GenerateRequestData`
   在拼接 `ContentBlocks` 前调用，已知非 vision 模型直接返回 `ErrUnsupportedContent`。
   设计与验收见需求族 **[媒体能力门控](requirements/media-capability-gating.md)**（已交付）。
   TUI 层的提交前**预检提示**（体验优化）仍待 P1：M5 确认图片后、提交前用 `SupportsVision(modelName)`
   判定，已知非 vision → 提示用户，避免发出去才被兜底拦截（见 §7 双层门控、验收 10）。
2. **UI 无图片输入通道**：目前没有任何路径把 `ImageBlock` 灌入 `ChatMessage`（无 `ask`/视觉桥 CLI 入口）。
   → PRD 的 M5 `image_paste` 正是补这一环：剪贴板图 → PNG 临时文件 → `ImageBlock`（内联）或
   `ImageURLBlock`（本地路径经 provider 参考）→ 挂到 `ContentBlocks`。模型层无需改动。
3. **图片预览**：TUI 当前仅有 `streamAnswer`/`flushableMarkdownPrefix` 文本流渲染，
   `ContentImage` 块无任何可视化呈现。→ PRD 渲染维度可加「附件回显行」（文件名 + MIME + 尺寸 meta），
   至少让用户确认图片确实挂上。

### 11.4 对 PRD 的直接落点

- 验收 6「剪贴板放图 → 会话产生一个附件」在模型层**已可满足**（`ImageBlock` 构造 + provider 编码齐备），
  剩余工作全部在 `cli/` TUI 侧：读剪贴板 → PNG 落盘 → 组装 `ContentBlocks` → `render` 回显。
- PRD 约束已正式并入 §7 边界：**图片仅路由给 vision 模型，否则阻断并提示**（对标 capability.go 的
  `SupportsVision`）——TUI 提交前预检提示（P1 待做）+ 模型层 `gateMultimodal` 兜底（已实现，
  见需求族 [媒体能力门控](requirements/media-capability-gating.md)）双层把关。
- 若未来要支持「读屏/看图 agent」，只需把 `ContentImage` 挂到用户消息并依赖既有 provider 编码，模型层
  0 改动（已列入 [TUI 能力 backlog](requirements/tui-capability-backlog.md)）。

---

## 12. 能力对齐审计（6 项 + 周边）

对照 Codex 上游能力逐项盘点 inferglow 现状，标注 PRD 的接入点。

| # | 能力 | inferglow 现状 | 结论 | PRD 接入点 |
|---|------|---------------|------|-----------|
| 1 | Markdown 流渲染 | [tui_model.go](file:///e:/test/rewrite-agently/inferglow/cli/tui_model.go) `streamAnswer` + `flushableMarkdownPrefix`，把**已闭合**的 markdown 块（含代码块）刷进 transcript，避免半截 code block 上屏 | ✅ 已具备 | 富渲染（OUT-8）直接复用该「可冲刷前缀」机制 |
| 2 | ask/user 问题与建议工具 | 存在 `orchestrator/hitl`（人工介入桥）与 `approval/` 审批流，但**无对话内 ask 建议工具**（未发现 tool 让模型在运行中回溯提问建议） | ⚠️ 需补 | **后续阶段**，移交需求族 [TUI 能力 backlog](requirements/tui-capability-backlog.md)：新 `ask_suggestion` 工具，串到 hitl 桥 |
| 3 | 对话恢复 | [tui_session.go](file:///e:/test/rewrite-agently/inferglow/cli/tui_session.go) `/resume [id]` 能列出历史会话并显示目标，但**实际切换需 agent 重建，已延期** | ⚠️ 部分 | 富 Composer 不阻塞；对「恢复后输入态」无影响（Composer 状态与会话无关，恢复后从 M0 重新开始）；能力本体移交需求族 [TUI 能力 backlog](requirements/tui-capability-backlog.md) |
| 4 | 主体主题能力 | [tui_theme.go](file:///e:/test/rewrite-agently/inferglow/cli/tui_theme.go) 提供主题 token，`tui_receipt.go`/`tui_steer` 已用 | ✅ 已具备 | 粘贴卡片/附件回显用现有 token |
| 5 | TDD 工作流 / 证据门 / 视觉桥(ask/vision) | 未接入。无 TDD 编排、无证据门强制、"看读"能力待补（依赖第 11 章图片通道） | ❌ 需补 | **后续阶段**，移交需求族 [TUI 能力 backlog](requirements/tui-capability-backlog.md)：富 Composer 提供图片附件底座，视觉桥落地后再挂 |
| 6 | 代码检索 / 记忆跨会话召回 | `context/retrieval`（bm25/fusion/recency/semantic）+ `memory/` 均已实现 | ✅ 已具备 | 富输入支持 `@file/关键词` 触发检索，无需新后端 |

项目内还有更丰富的既有件可平替上游实现：`builtins/actions/memory_recall.go` + `context/`（召回/压缩）、
`orchestrator/hitl`（人机协同）、`model/` 多模态（第 11 章）。因此富输入 Composer 的**新增面收敛到
「输入状态机 + 剪贴板/图片通道 + 附件回显」**，其余靠搭既有模块。

### 12.1 测试与 lint 完备性（现状）

- 测试覆盖偏低：TUI 侧仅 `tui_steer_test.go`（含 `TestSteerModeForKey`/`TestSteerPendingSummary`/
  `TestSteerLabel`/`TestSteerTintFor` 等）；整体 CLAIM「48 用例 / 3 文件」为 TUI 单元层，且
  `Makefile` 的 `test`/`lint` 目标会遍历所有含 `go.mod` 的子模块。
- lint 漏检关键项：[.golangci.yaml](file:///e:/test/rewrite-agently/inferglow/.golangci.yaml) 禁用了
  `errcheck / staticcheck / unused / ineffassign`，仅启 `govet + revive`，**错误处理/未用变量不会在 CI 拦截**。
  → PRD 要求：`cli/composer/` 代码落地时**必须**进入严格 lint 域（`errcheck` 等），且 P0 强制
  `rich_composer_test.go` 行覆盖率 ≥ 90%（纯状态机易测，已入 §8 验收 9）。
  → 具体「如何扩 lint 而不撑爆 CI」见需求族 **[严格 lint 范围](requirements/strict-lint-scope.md)**。

---

## 13. 关联需求族（已拆分至 docs/requirements/）

> v0.4 起本 PRD 只保留 input 线需求；非 input 线需求按领域拆为需求族文件，
> 统一由 [docs/requirements/](requirements/) 文件夹管理（索引见其 [README.md](requirements/README.md)）。

| 需求族 | 状态 | 拆分自 | 说明 |
|--------|------|--------|------|
| [媒体能力门控](requirements/media-capability-gating.md) | 已交付 ✅ | 原 §13.1 + §11.3 缺口 1 | `gateMultimodal` + provider 穿透 + `ErrUnsupportedContent` |
| [provider 配置存储](requirements/provider-config-store.md) | 已交付 ✅ | 原 §13.2 | providers.yaml 单一事实源 + `configure` 向导 |
| [严格 lint 范围](requirements/strict-lint-scope.md) | 待建 ⏳ | 原 §13.3 + §12.1 | 新子包（`cli/config`、`cli/composer`）严格 lint 域 |
| [TUI 能力补齐 backlog](requirements/tui-capability-backlog.md) | 规划中 📋 | 原 §12 #2/#3/#5 + §11.4 展望 | ask 建议 / 对话恢复 / TDD·证据门 / 视觉桥 / 看图 agent |

---