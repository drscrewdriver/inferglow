# 12 · 质量属性与演进路线

## 一、质量属性

| 属性 | 度量 | 当前状态 |
|------|------|---------|
| 模块化 | 独立 Go module 数 | 23 |
| 可测试性 | 测试文件数 | 200+ |
| 可扩展性 | 接口扩展点 | 7 种（Provider/Executor/ResizeHandler 等） |
| 安全性 | 安全层 | 4 层（PII/注入/限流/RBAC） |
| 性能 | 并发模型 | goroutine + channel |
| 可观测性 | SpanKind 数 | 6 种 |
| 可维护性 | 循环依赖 | 无 |

## 二、演进路线

```
Phase 1 (V1–V3): 基础设施零件
    model/schema/flow/action/session/sandbox 独立模块

Phase 2 (V4–V5): 编排层
    orchestrator + security + context 管理

Phase 3 (V6): 6-Wave 优化
    Middleware/Callbacks/Memory/解耦/RateLimit/并行

Phase 4 (V7): 能力补齐
    触发器/LCEL/Memory/状态检查/流式

Phase 5 (V8+): 上层产品化
    CLI Agent → 桌面端 → 全平台 AI 助理
```

## 三、统一待增强方向（合并规划 2026-08-01）

> 合并自 roadmap / context-mode-switch-spec / Zone 结构设计文档，统一编号管理

### 3.1 上下文管理（Context Management）

| # | 方向 | 优先级 | 说明 | 状态 |
|---|------|--------|------|------|
| CM-1 | Zone 1 Background 填充 | **P1** | `/rebackground` 改为写入 Zone 1（`RewriteHeadBuffer`）而非 Zone 0.5；Zone 1 = Background + Skill 轻量索引 | ✅ 已完成 |
| CM-2 | 自动触发背景总结 | **P1** | agent loop 检测：session 启动 N 步后 Zone 1 为空 → 自动触发 rebackground | ✅ 已完成 |
| CM-3 | Zone 0.5 元操作指令注入 | P2 | 宪法区注入上下文工具/记忆工具使用指引、Background 自驱动规则、压缩状态感知提示 | ✅ 已完成 |
| CM-4 | 步骤驱动的背景更新 | P2 | 每 M 步检查语义漂移（当前 step 与 Zone 1 关键词重叠度），低于阈值则 HintBlock 提醒更新 | ✅ 已完成 |
| CM-5 | Plan 模式继承 | P2 | Plan 模式完成时，将 plan 摘要自动写入 Zone 1 | ✅ 已完成 |
| CM-6 | Zone 0.5 + Zone 1 token 预算 | P3 | 总上限 = 窗口 5%，避免挤占工作上下文 | 待实施 |
| CM-7 | 背景版本管理 | P3 | `/rebackground` 对接 `RewriteHeadBuffer` 实现替换语义 + 历史版本链 | 待实施 |

### 3.2 上下文管理模式与压缩（Mode & Compression）

| # | 方向 | 优先级 | 说明 | 状态 |
|---|------|--------|------|------|
| MC-1 | 上下文管理模式配置 | **P1** | `CLIConfig` 新增 `context_mode` 字段，`NewMemoryBridge` 按配置创建对应 Manager（passthrough/three_zone/summary/hybrid） | ✅ 已完成 |
| MC-2 | 压缩模型独立配置 | **P1** | `CLIConfig` 新增 `compress_model` 字段（独立 `LLMConfig`），传入 `CompressModelChain.small`；为空时 fallback 主模型。底层 `CompressModelChain` 已实现 small→main→mechanical 三级降级 | ✅ 已完成 |
| MC-3 | `/mode` TUI 命令 | P2 | TUI 中 `/mode hybrid`、`/mode summary` 等动态切换上下文管理模式 | ✅ 已完成 |
| MC-4 | `/async-compress` 命令 | ✅ 已完成 | 手动触发强制压缩，绕过甜点区阈值检查 | ✅ |

### 3.3 用户数据目录与配置（Data Directory & Config）

| # | 方向 | 优先级 | 说明 | 状态 |
|---|------|--------|------|------|
| DC-1 | 目录结构自动初始化 | ✅ 已完成 | `EnsureDataDirs()` 启动时创建完整目录结构 | ✅ |
| DC-2 | 宪法区默认路径 | ✅ 已完成 | `~/.inferglow/constitutional/rules.md` | ✅ |
| DC-3 | TUI 配置持久化 | ✅ 已完成 | `TUIConfig` 字段写入 config.json | ✅ |
| DC-4 | Session Log 格式规范 | ✅ 已完成 | L0 jsonl + refs jsonl 格式已定型 | ✅ |
| DC-5 | 首次运行引导 | P2 | 检测空 endpoint 时提示用户输入；支持 `init` 子命令交互式配置 | ✅ 已完成 |
| DC-6 | 配置热重载 | P3 | TUI 中 `/config reload` 命令，无需重启即可更新配置 | 待实施 |

### 3.4 其他待增强方向

| # | 方向 | 优先级 | 说明 | 状态 |
|---|------|--------|------|------|
| OT-1 | Streaming/SSE 输出 | P0 | 实时流式聊天输出，支持 SSE 和 WebSocket | 待实施 |
| OT-2 | Multi-Agent 协作 | P0 | Host-Specialist 路由 + 任务委派 | 待实施 |
| OT-3 | A2A Protocol | P1 | Agent-to-Agent 跨进程/跨网络通信协议 | 待实施 |
| OT-4 | 向量检索 | P1 | Embedding-based 语义检索 | 待实施 |
| OT-5 | Prompt 管理 | P1 | Prompt 版本控制、模板仓库、动态组合 | 待实施 |
| OT-6 | Eval 框架 | P1 | Agent 离线评估自动化（已实现 ~750 LOC，需示例和文档） | 待实施 |
| OT-7 | Task Tracker | **P1** | LLM 可操作的待办清单：TaskStore + 4 个 action（task_add/update/list/delete）+ 上下文注入 + TUI 进度显示 | ✅ 已完成 |
| OT-8 | BM25 语言搜索增强 | **P1** | CJK bigram 分词 + 倒排索引 + 索引持久化，解决中文搜索完全失效问题 | ✅ 已完成 |
| OT-9 | IM Bridge | P2 | Telegram/飞书/QQ/微信 | ✅ 已完成 |
| OT-10 | 桌面端 | P2 | Tauri/Wails 桌面壳 | ✅ 已完成 |
| OT-11 | Tool Result Cache | P2 | 工具调用结果缓存，支持 TTL 和 LRU 淘汰 | ✅ 已完成 |
| OT-12 | 知识图谱记忆 | P2 | 结构化长期记忆 | ✅ 已完成 |
| OT-13 | 可观测性面板 | P2 | 内置 Agent 运行仪表盘 | ✅ 已完成 |
| OT-14 | TUI Slash 命令系统 | P2 | `/` 命令菜单增强：自动补全、命令发现 | ✅ 已完成 |
| OT-15 | 插件系统 | P3 | 约定优先插件 + 两级权限 | 待实施 |

## 四、附录：模块 LOC 统计

| 模块 | 路径 | 代码量 |
|------|------|--------|
| model | `github.com/inferglow/model` | ~8000 |
| sandbox | `github.com/inferglow/sandbox` | ~6300 |
| context | `github.com/inferglow/context` | ~6300 |
| orchestrator | `github.com/inferglow/orchestrator` | ~7700 |
| flow | `github.com/inferglow/flow` | ~7400 |
| server | `github.com/inferglow/server` | ~3100 |
| action | `github.com/inferglow/action` | ~2900 |
| schema | `github.com/inferglow/schema` | ~2800 |
| examples | `github.com/inferglow/examples` | ~2800 |
| builtins | `github.com/inferglow/builtins` | ~2200 |
| security | `github.com/inferglow/security` | ~2000 |
| session | `github.com/inferglow/session` | ~1800 |
| rag | `github.com/inferglow/rag` | ~1500 |
| workspace | `github.com/inferglow/workspace` | ~1200 |
| cli | `github.com/inferglow/cli` | ~1200 |
| audit | `github.com/inferglow/audit` | ~1100 |
| mcpserver | `github.com/inferglow/mcpserver` | ~850 |
| resource | `github.com/inferglow/resource` | ~750 |
| eval | `github.com/inferglow/eval` | ~750 |
| observability | `github.com/inferglow/observability` | ~700 |
| approval | `github.com/inferglow/approval` | ~700 |
| rerank | `github.com/inferglow/rerank` | ~500 |
| components | `github.com/inferglow/components` | ~400 |
| **总计** | | **~62,000** |

## 五、附录：Graphify 命令参考

```bash
# 构建知识图谱（仅代码）
graphify <path> --code-only

# 查询架构
graphify query <path> "问题" --graph graphify-out/graph.json

# 查看 God Nodes
graphify god-nodes --top 20 --graph graphify-out/graph.json

# 最短路径分析
graphify path <path> "NodeA" "NodeB" --graph graphify-out/graph.json

# 节点解释
graphify explain <path> "NodeName" --graph graphify-out/graph.json

# 更新图谱
graphify update <path>
```

## 六、附录：文档索引

| 文档 | 位置 | 内容 |
|------|------|------|
| 架构深度分析 | `ARCHITECTURE.md` | 12 章完整架构分析 |
| 扩展机制 | `docs/EXTENDING.md` | 7 种扩展机制 |
| 上游缺口 | `docs/upstream-gaps.md` | inferglow 待完善能力清单 |
| 系统分析 | `docs/system-analysis/README.md` | 系统分析入口 |