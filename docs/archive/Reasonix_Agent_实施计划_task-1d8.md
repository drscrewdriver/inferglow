# Reasonix Agent on InferGlow — 全阶段实施计划

> 核心原则：**CLI Agent 是组合根（composition root），不是新引擎**。所有核心能力已存在，仅通过新增文件组装。
> 变更足迹：P0 新增 ~14 个文件，**修改 0 个现有文件**。
> 产品终态：全平台个人 AI 助理（对标 Reasonix 桌面端 + OpenHanako 多端接入）。

---

## 架构总览

```
用户输入
  ↓
┌─ cli/repl.go ─────────────────────────────────────────┐
│  REPL 循环 → dispatchCommand / chatOnce               │
└───────────────────────────────────────────────────────┘
  ↓ chatOnce
┌─ cli/memory_bridge.go ────────────────────────────────┐
│  Recall(query) → FusionRetriever.Search() top-K       │  ← 自动注入
│  → 格式化为 <relevant_memories> 拼入 systemPrompt     │
└───────────────────────────────────────────────────────┘
  ↓ agent.Run(ctx, msg, WithSystemPrompt(base+mem))
┌─ orchestrator/agent (已有) ───────────────────────────┐
│  Middleware chain → Engine.executeLoop → Tool calls    │
│  ↑ ingestExecutor decorator 在每个工具返回后:          │  ← 自动存储
│    ContextManager.Ingest() → L1→L2 → LongMemPromoter  │
└───────────────────────────────────────────────────────┘
  ↓ 响应
┌─ cli/memory_bridge.go ────────────────────────────────┐
│  IngestAssistant(response) → 存入记忆                  │
│  ValidateMemory(cited IDs) → confidence += 0.04       │
└───────────────────────────────────────────────────────┘
```

---

## P0：核心 CLI Agent + 记忆闭环（2 周）

### 0.1 新增 list_dir / grep builtin actions

- **新建** `builtins/actions/list_dir.go`
  - 仿照 `file_read.go` 模式：`ListDirConfig{AllowedDirs []string}`
  - `NewListDirAction(cfg) *action.Action`，返回 `[]DirEntry{name, type, size}`
  - AllowedDirs 沙箱限制（路径穿越防护）
- **新建** `builtins/actions/grep_executor.go`
  - 注入式 `GrepRunner` 接口（仿 `BashExecutor`）
  - `NewGrepAction(runner) *action.Action`，参数：pattern/path/recursive/max_results
- **新建** 对应 `_test.go`

### 0.2 CLI 模块骨架

- **新建** `cli/go.mod`（module `github.com/inferglow/cli`）
  - replace 指向 `../orchestrator`、`../action`、`../session`、`../context`、`../builtins`、`../model`
  - 参照 `orchestrator/go.mod` L67-84 的 replace 模式
<!-- REVIEW: orchestrator/go.mod 的 replace 列表不含 `context`（因为 orchestrator 不直接依赖 context）。cli/go.mod 需要额外加 `replace github.com/inferglow/context => ../context`——context 是 V6 独立 module。-->
- **新建** `cli/cmd/inferglow-cli/main.go`
  - flag 解析：`--workspace`、`--model`、`--config`、`--resume`、`--unsafe`
  - 信号处理（SIGINT/SIGTERM 优雅退出）
  - 调用 `cli.Run(ctx, cfg)`

### 0.3 配置

- **新建** `cli/config.go`
  ```go
  type CLIConfig struct {
      LLM            LLMConfig   // endpoint, model, api_key
      DataDir        string      // ~/.inferglow/
      WorkspaceDir   string      // 工作目录
      Constitutional string      // 宪法文件路径
      WindowTokens   int         // 上下文窗口 (default 32000)
      TopK           int         // 记忆检索 top-K (default 5)
      Features       FeatureFlags
  }
  type FeatureFlags struct {
      MemoryInjection bool  // 每轮自动检索注入
      MemoryStorage   bool  // 工具结果自动存储
      Constitutional  bool  // 宪法区加载
      Compression     bool  // 自动压缩
  }
  ```
  - `LoadConfig(path)` + `DefaultCLIConfig()`

### 0.4 模型构造

- **新建** `cli/model_factory.go`
  - `buildModelRequester(cfg) (model.StreamRequester, error)`
  - 照搬 `inferflow/runtime/integration/model_provider.go` 的 StaticConfigProvider 模式
  - 支持 OpenAI 兼容 endpoint（DeepSeek/MiMo/本地）
<!-- REVIEW: `agent.New` 接受 `model.ModelRequester`（非 `StreamRequester`）。`ModelRequester` 组合了 `StreamRequester` + `ResponseBroadcaster`。factory 应返回 `model.ModelRequester`，或接受返回 `StreamRequester` 但需要额外包装为 `ModelRequester`。-->

### 0.5 记忆桥（P0 核心）

- **新建** `cli/memory_bridge.go`
  ```go
  type MemoryBridge struct {
      mgr       contextmgr.ContextManager
      retriever *retrieval.FusionRetriever
      bm25      *retrieval.BM25Index
      promoter  *contextmgr.LongMemPromoter
      topK      int
  }
  ```
  - `NewMemoryBridge(cfg, sessionID)`:
    1. `jsonl.New(dataDir, sessionUUID)` → 持久化后端（返回 `(*Store, error)`，需处理 err）
    2. `contextmgr.NewHybridManager(ctxCfg, store)` → 压缩引擎
    3. `retrieval.NewBM25Index()` → 关键词索引
    4. `retrieval.NewFusionRetriever(nil, bm25, nil, &NoopEmbedder{})` → 退化为 BM25+recency
    5. `contextmgr.NewLongMemPromoter(store, longMemCfg, sessionID)` → 提升器
  - `Recall(ctx, query) (string, []string)`:
    - `retriever.Search(ctx, query, topK)` + `mgr.SearchLongMem(ctx, query, "", topK)`
    - 格式化为 `<relevant_memories>...</relevant_memories>` 文本块
    - 返回格式化文本 + 被引用的 memID 列表（用于后续 Validate）
<!-- REVIEW: `FusionRetriever.Search` 参数名为 `limit`（非 `topK`）。`ContextManager.SearchLongMem` 签名：`(ctx, query, category, limit)`。-->
  - `IngestTool(toolName, content)`:
    - 构造 `StepRecord{Type:"tool", Content: truncate(content, 8KB)}`
    - `mgr.Ingest(step)` → 自动 L1→L2 压缩
    - `bm25.Add(stepID, content)` → 同步索引
    - `promoter.EvaluateAndPromote(ctx)` → 满足条件则提升
  - `IngestUser(content)` / `IngestAssistant(content)`: 同理
  - `ValidateCited(memIDs)`: 对每个被引用记忆 `promoter.ValidateMemory(id, step)`
  - `Stats()` / `Compact(ctx)` / `SearchMemory(query)`
  - `OnSessionEnd(ctx)`: `promoter.OnSessionEnd(ctx)` + `mgr.Close()`

### 0.6 自动存储装饰器

- **新建** `cli/ingest_executor.go`
  ```go
  type ingestExecutor struct {
      inner action.ActionExecutor
      bridge *MemoryBridge
      name   string
  }
  func (e *ingestExecutor) Execute(ctx context.Context, input map[string]any) (*action.ActionResult, error) {
      result, err := e.inner.Execute(ctx, input)
      if err == nil {
          e.bridge.IngestTool(e.name, formatResult(result))
      }
      return result, err
  }
  ```
  - `wrapWithIngest(act *action.Action, bridge) *action.Action`：浅拷贝，仅替换 Executor
  - 组合优于继承，零引擎改动
<!-- REVIEW: Execute 返回类型修正为 `*action.ActionResult`（非 `map[string]any`）。真实接口签名：`Execute(ctx context.Context, input map[string]any) (*ActionResult, error)`。-->

### 0.7 Agent 工厂

- **新建** `cli/agent_factory.go`
  - `buildAgent(cfg, bridge) (*agent.Agent, error)`:
    1. `session.NewSession(id, maxLen)`
    2. `agent.NewActionExtension()` → 注册 6 个 builtins（bash/file_read/file_write/list_dir/grep/url_fetch）
    3. 对每个 action 调 `wrapWithIngest` 包装
    4. `agent.New(sess, actExt, modelReq, agent.WithCallbacks(...))`
  - Callbacks:
    - `OnRunEnd`: `bridge.IngestAssistant(response)` + `bridge.ValidateCited(lastCitedIDs)`

### 0.8 REPL + 命令

- **新建** `cli/repl.go`
  - `RunREPL(ctx, cfg)`: bufio.Scanner 循环
  - 非 `/` 开头 → `chatOnce(line)`
  - `/` 开头 → `dispatchCommand(line)`
- **新建** `cli/commands.go`
  - 命令表 `map[string]CommandFunc`
  - `/memory search <query>` — 检索记忆
  - `/memory stats` — 记忆统计
  - `/compact` — 手动压缩
  - `/quit` — OnSessionEnd + 退出
- `chatOnce` 核心流程:
  ```
  memText, memIDs := bridge.Recall(ctx, line)
  sysPrompt := basePrompt + persona + memText
  resp, err := ag.Run(ctx, line, agent.WithSystemPrompt(sysPrompt))
  bridge.ValidateCited(memIDs)
  print(resp)
  ```

### 0.9 宪法区加载

- **新建** `cli/constitutional.go`
  - `loadConstitutional(path) ([]string, error)`: 按行读取
  - 在 buildAgent 时 `hybridMgr.AppendConstitutional(entries)`

### 0.10 会话持久化

- 复用 `jsonl.Store`（0.5 已含）
- **新建** `cli/session_index.go`: `sessions/index.jsonl`（append-only 索引：uuid + 标题 + 时间）
- `/quit` 时 `session.SaveJSON(path)` + `LongMemPromoter.OnSessionEnd`
<!-- REVIEW: Session.SaveJSON 接受 `path string` 参数（session/session.go L114），不是无参调用。需在 cli 层持有 dataDir 拼接完整路径。-->

### 0.11 本地 Bash 执行器

- **新建** `cli/local_bash.go`
  - 实现 `builtins.BashExecutor` 接口
  - `os/exec.CommandContext` + 超时（30s）+ 工作目录限制
  - 受 `--unsafe` flag 守卫（默认需确认）

### 0.12 P0 冒烟测试

- **新建** `cli/repl_test.go`
  - 用 mock LLM（参照 `examples/example_orchestrator.go` L44-72）
  - 断言：输入 → 召回 → Run → 存储 → JSONL 有记录
  - 断言：第二轮能检索到第一轮的记忆

---

## P1：子 Agent + Skill（1.5 周）

### 1.1 TaskTool

- **新建** `cli/task_tool.go`
  - `NewTaskAction(agentFactory) *action.Action`
  - Execute: 创建独立 session + 独立 MemoryBridge → 子 Agent.Run → 返回结果
  - 子 agent 与父物理隔离（独立 JSONL 文件）

### 1.2 SkillLoader

- **新建** `cli/skill_loader.go`
  - `parseSkillMD(path) (*SkillDef, error)`: 解析 YAML front-matter + 正文
  - `registerSkill(actExt, def)`: 动态构造 Action
  - 复用 `orchestrator/skill/SkillLibrary` 做版本管理

### 1.3 Web 工具

- 在 `agent_factory.go` 中注册已有 `NewWebSearchAction(provider)` + `NewURLFetchAction(cfg)`
- **新建** `cli/web_provider.go`: 实现 `SearchProvider` 接口（调 SearXNG/Tavily）

### 1.4 会话恢复

- **新建** `cli/resume.go`
  - `ResumeSession(dataDir, sessionID)`: `Session.LoadJSON` + JSONL store `loadAll` 自动恢复
  - `--resume <id>` flag + `/chat list` + `/chat resume <id>`

---

## P2：个性化助手感（1.5 周）

### 2.1 Persona

- **新建** `cli/persona.go`
  ```go
  type Persona struct {
      Name        string   `yaml:"name"`
      Tone        string   `yaml:"tone"`
      Constraints []string `yaml:"constraints"`
      Language    string   `yaml:"language"`
  }
  func (p Persona) SystemPromptBlock() string
  ```
  - 从 `~/.inferglow/persona.yaml` 加载
  - 在 chatOnce 中拼到 basePrompt 最前

### 2.2 记忆编译

- **新建** `cli/memory_compile.go`
  - `/memory compile`: 收集碎片 longmem → 单次 LLM 摘要 → 写回合并记录
  - 批量处理，减少 LLM 调用次数

### 2.3 主动召回

- 在 REPL 启动时（或 `/chat resume` 后）自动 `bridge.Recall(最近话题)`
- 注入 "你之前提到过..." 到首条 system message
- 受 `FeatureFlags.ProactiveRecall` 守卫

### 2.4 衰减/遗忘

- **新建** `cli/decay_hook.go`
  - 每 N 轮（或 `/memory decay`）对 longmem 执行 confidence 衰减
  - 复用 `context/decay.go` 的 `EffectiveDecay` 公式
  - 低于阈值（0.1）标记为遗忘

### 2.5 用户画像

- **新建** `cli/user_profile.go`
  - `/profile extract`: LLM 从历史 facts 提取用户偏好
  - 存入 longmem（category="user_profile"）
  - 每轮注入到 systemPrompt 的 `<user_profile>` 区

---

## P3：Multi-Agent + 桌面端 + IM（远期）

> 对标：Reasonix（CLI + 桌面端）、OpenHanako（Electron 桌面端 + 多端接入）
> 产品形态演进：CLI → 桌面端 → 多端

### 3.1 AgentTool 机制

- **新建** `cli/agent_tool.go`
  - 把"委托给具名 Agent"做成 Action
  - 持久化子 agent 实例（跨调用复用）

### 3.2 频道路由（Host-Specialist）

- **新建** `cli/router.go`
  - `ChannelRouter{host, specialists map[string]*agent.Agent}`
  - 基于消息内容 + 意图分类路由
  - 复用 `orchestrator/team/messageBus`
  - 对标 OpenHanako：多 Agent 频道群聊 + 互相委派任务

### 3.3 桌面端（Tauri/Electron）

> 对标：Reasonix 桌面端（Wails）、OpenHanako（Electron + React）

- **技术选型**：Tauri 2.0（Rust 壳 + Go 后端 via sidecar）或 Wails（已有经验）
- **架构**：
  ```
  桌面壳 (Tauri/Wails)
    ├── 前端: React/Svelte 聊天 UI + 记忆面板 + Agent 管理
    ├── 后端: inferglow-cli 作为 sidecar 进程
    │         通过 stdin/stdout JSON-RPC 或 WebSocket 通信
    └── 系统托盘: 后台心跳 + 定时任务 + 主动通知
  ```
- **关键功能**：
  - 多 Agent 管理（创建/切换/删除，各有独立记忆+人格）
  - 记忆可视化（时间线 + 搜索 + 手动编辑/删除）
  - 书桌（Desk）：拖拽文件到 Agent 工作区，Agent 主动处理
  - 角色卡导入/导出（Persona + Memory + Skills 打包为 zip）
  - 系统托盘常驻 + 全局快捷键唤起
- **新建**：
  - `desktop/` 独立仓库（或 `cli/desktop/` 子目录）
  - `cli/rpc_server.go`: JSON-RPC over stdio，供桌面壳调用
  - `cli/tray.go`: 系统托盘 + 心跳循环

### 3.4 定时任务 + 心跳（自主行为）

> 对标：OpenHanako Cron + 心跳巡检

- 复用 V7 已实现的 `server/trigger/CronTrigger`
- 新增 Heartbeat 循环：定期巡检书桌文件变化 / 主动执行计划任务
- Agent 不在对话中时也能按计划自主工作
<!-- REVIEW: spec 原引用 "V7 已实现的 server/trigger/CronTrigger"——该类型在代码库中不存在（grep 全库无匹配）。P3.4 定时任务需从零构建，建议用 Go 标准库 time.Ticker + goroutine，或引用 robfig/cron。-->

### 3.5 IM Bridge（多平台接入）

> 对标：OpenHanako Telegram/飞书/QQ/微信 Bridge

- **新建** `cli/bridge/` 目录
  - `bridge.go`: `Bridge` 接口（Receive/Send/Start/Stop）
  - `telegram.go`: long-polling → 复用 `chatOnce` 内核
  - `websocket.go`: WebSocket 实时双向（供桌面端/移动端）
  - 后续：飞书/QQ/微信（按需）
- 每 chat 独立 session，共享模型连接
- 多端同时接入同一 Agent（桌面 + Telegram + 手机）

### 3.6 移动端 / LAN 接入

> 对标：OpenHanako Hana Server + PWA

- `cli/server_mode.go`: 将 CLI Agent 托管为 HTTP 服务
  - 复用 `server/` 模块的 REST API + SSE 流式
  - 托管 PWA 前端（移动端通过 LAN URL 接入）
  - access key 鉴权

### 3.7 插件系统（远期）

> 对标：OpenHanako 约定优先插件 + 两级权限

- 插件可贡献：工具/技能/命令/Agent 模板/HTTP 路由/事件钩子
- PluginContext 注入 + Session Bus 与 Agent 交互
- 两级权限（restricted / full-access）
- 复用 `orchestrator/skill/SkillLibrary` 做安装/版本管理

---

## 性能优化（随 P0 落地，独立于功能）

| 修复 | 文件 | 说明 |
|------|------|------|
| O(n²) 排序 | `context/retrieval/fusion.go` L169-176 | 冒泡 → `sort.Slice` |
| JSONL 全量重写 | `context/store/jsonl/jsonl_store.go` L250-283 | 脏标记 + 批量 coalesced flush |
| BuildContext 缓存 | `context/hybrid.go` L287-312 | 接入 `RenderStepWithCache` |
| SQLite WAL | `context/store/sqlite/sqlite_store.go` | `PRAGMA journal_mode=WAL` + 连接池 |

> 注：这些修复**修改现有文件**，但都是纯性能优化，不改接口语义，可独立 PR。

---

## 依赖关系

```
0.1 (list_dir/grep) ──┐
                      ├──→ 0.7 (agent_factory) ──→ 0.8 (REPL) ──→ 0.12 (测试)
0.2 (骨架) ─→ 0.3 (配置) ─→ 0.4 (模型) ──┘         ↑
                      └──→ 0.5 (记忆桥) ─→ 0.6 (装饰器) ─┘
                      └──→ 0.9 (宪法) ──────────────────┘
                      └──→ 0.10 (持久化) ─→ 0.11 (bash) ─┘

P1: 0.7 → 1.1 (TaskTool); 0.1 → 1.2 (Skill); 0.7 → 1.3 (Web); 0.10 → 1.4 (Resume)
P2: 0.5 → 2.2/2.3/2.4; 0.8 → 2.1 (Persona); 2.2 → 2.5
P3: 1.1 → 3.1 → 3.2; 0.8 → 3.3 (桌面端) → 3.4 (心跳); 3.2 → 3.5 (IM) → 3.6 (移动/LAN); 1.2 → 3.7 (插件)
```

**关键路径**: 0.2 → 0.5 → 0.6 → 0.7 → 0.8（记忆闭环骨架）

**产品形态演进**:
```
P0: CLI Agent（终端对话 + 记忆闭环）
P1: CLI + 子Agent + Skill + Web工具
P2: CLI + 人格 + 主动回忆 + 衰减遗忘（“有温度的助手”）
P3: 桌面端 + Multi-Agent + IM + 移动端（“全平台个人 AI 助理”）
```

---

## 风险与缓解

| 风险 | 缓解 |
|------|------|
| Agent 引擎与 ContextManager 未接线（最大缺口） | MemoryBridge 在 CLI 层显式桥接；P0 测试断言"召回出现在 prompt"且"Ingest 后 JSONL 有记录" |
| FusionRetriever 索引不自动填充 | MemoryBridge 自持 BM25Index，每次 Ingest 同步 Add；语义检索用 NoopEmbedder 占位 |
| AgentCallbacks 无 OnToolResult | 用 ingestExecutor decorator 包装每个 Action.Execute，零侵入捕获结果 |
| systemPrompt 单次 Run 内固定 | 记忆按"用户回合"粒度召回（每次 chatOnce 重建），符合 CLI 交互节奏 |
| JSONL rewriteRefs 长会话性能 | P0 可接受；性能修复阶段引入 coalesced flush 或切 SQLite |
| BashExecutor 需真实 shell | local_bash.go 最小实现 + --unsafe flag 守卫 |
| 工具结果过大 | ingestExecutor 截断 >8KB 仅存摘要 |
| 多模块 replace 链复杂 | 参照 orchestrator/go.mod 模板；可用 go.work 简化 |

---

## 被否决的方案

| 方案 | 否决原因 |
|------|----------|
| 修改 Engine.executeLoop 内部嵌入 memory hook | 侵入核心引擎，回归风险高，违反"零修改"原则 |
| 记忆注入用 BuildContext 替代 systemPrompt 拼接 | BuildContext 返回 []RenderedBlock 与 Agent.Run 的 systemPrompt string 不兼容，需改引擎 |
| 异步 channel + worker 做 Ingest（Plan B） | P0 过度设计；同步 Ingest 在 CLI 单 goroutine 下无锁竞争，延迟可接受 |
| 每轮 LLM call 前注入记忆（per-round） | 需改 Engine 内部；per-Run 粒度对 CLI 交互已足够 |
| 直接复用 session.SaveJSON 做会话持久化 | 全量 JSON 快照非 append-only；JSONL store 已免费获得 append-only + 自动恢复 |
| 在 builtins 包内新增 list_dir/grep 时修改 registry.go | 不需要——Action 注册在 CLI 层通过 `actExt.Register` 完成 |

---

## 文件清单（P0 新增）

| 文件 | 职责 | 行数估计 |
|------|------|---------|
| `cli/go.mod` | 模块定义 + replace | ~30 |
| `cli/cmd/inferglow-cli/main.go` | 入口 + flag + 信号 | ~60 |
| `cli/config.go` | CLIConfig + FeatureFlags | ~80 |
| `cli/model_factory.go` | 模型构造 | ~50 |
| `cli/memory_bridge.go` | **记忆闭环核心** | ~180 |
| `cli/ingest_executor.go` | 自动存储装饰器 | ~60 |
| `cli/agent_factory.go` | Agent 组装 | ~90 |
| `cli/repl.go` | REPL 循环 | ~80 |
| `cli/commands.go` | 斜杠命令 | ~70 |
| `cli/constitutional.go` | 宪法加载 | ~30 |
| `cli/session_index.go` | 会话索引 | ~50 |
| `cli/local_bash.go` | 本地 shell 执行器 | ~60 |
| `builtins/actions/list_dir.go` | 目录列表 Action | ~80 |
| `builtins/actions/grep_executor.go` | 搜索 Action | ~90 |
| **合计** | | **~1010** |

---

## P0 实施记录（2026-07-30 执行）

### 实施状态：✅ 全部完成，编译通过

### Spec vs 最新代码差异 & 修正

| # | Spec 假设 | 实际代码 | 修正方案 |
|---|-----------|----------|----------|
| 1 | 0.4 `buildModelRequester` 返回 `StreamRequester` | `agent.New` 需要 `ModelRequester`（= StreamRequester + ResponseBroadcaster） | 返回类型改为 `model.ModelRequester`；使用 `NewDeepSeekProviderFromConfig` 等直接返回满足完整接口的 Provider |
| 2 | 0.5/0.9 直接调 `hybridMgr.AppendConstitutional` | `AppendConstitutional` 仅在 `*HybridManager`，不在 `ContextManager` 接口 | `MemoryBridge` 保留 `*HybridManager` 具体类型引用，通过类型断言 `mgr.(*HybridManager)` 获取 |
| 3 | 0.10 `Session.SaveJSON` 无参 | 实际签名 `SaveJSON(path string)` | 在 cli 层持有 dataDir 拼接完整路径（`session_index.go` 已实现） |
| 4 | 无 `go.work` | 项目用 per-module `replace` 指令 | `cli/go.mod` 遵循同样模式，包含所有必要的 replace |
| 5 | `context/go.mod` 仅 2 行 | 确认无外部依赖，子包 `store/jsonl` 通过父模块获得类型 | 无需额外处理 |
| 6 | `NewHybridManager` 返回 `ContextManager` 接口 | 需要具体类型才能调 `AppendConstitutional` | `MemoryBridge.hybrid` 字段保留 `*HybridManager`，`AppendConstitutional` 通过 nil 检查保护 |

### 新增文件清单

| 文件 | 行数 | 状态 |
|------|------|------|
| `builtins/actions/list_dir.go` | 166 | ✅ |
| `builtins/actions/grep_executor.go` | 138 | ✅ |
| `cli/go.mod` | 38 | ✅ |
| `cli/cmd/inferglow-cli/main.go` | 77 | ✅ |
| `cli/config.go` | 86 | ✅ |
| `cli/model_factory.go` | 72 | ✅ |
| `cli/memory_bridge.go` | 236 | ✅ |
| `cli/ingest_executor.go` | 70 | ✅ |
| `cli/agent_factory.go` | 89 | ✅ |
| `cli/repl.go` | 146 | ✅ |
| `cli/commands.go` | 130 | ✅ |
| `cli/constitutional.go` | 49 | ✅ |
| `cli/session_index.go` | 140 | ✅ |
| `cli/local_bash.go` | 161 | ✅ |
| **合计** | **~1598** | `go build ./...` ✅ `go vet ./...` ✅ |

### 实施说明

1. **model_factory.go**：使用 `model.StaticConfigProvider` + provider-specific 构造函数（`NewDeepSeekProviderFromConfig` 等），直接返回满足 `ModelRequester`（= `StreamRequester` + `ResponseBroadcaster`）的 Provider
2. **memory_bridge.go**：`NewMemoryBridge` 内部创建 JSONL store → HybridManager → BM25Index → FusionRetriever → LongMemPromoter 完整链路；`Recall` 同时搜索 BM25 + LongMem 并格式化为 `<relevant_memories>` 块
3. **ingest_executor.go**：装饰器模式，`wrapWithIngest` 浅拷贝 Action 仅替换 Executor，零侵入
4. **agent_factory.go**：注册 6 个 builtins（file_read/file_write/list_dir/bash_executor/grep/url_fetch），全部经 ingest 包装
5. **local_bash.go**：同时实现 `BashExecutor` 和 `GrepRunner` 两个接口，复用 `os/exec.CommandContext`
