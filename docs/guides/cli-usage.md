# CLI 使用指南

> 面向**用户**的 CLI 操作指南：告诉你如何配置、启动、使用 InferGlow CLI 的全部能力。

## 目录

- [快速开始](#快速开始)
- [配置](#配置)
- [启动模式](#启动模式)
- [Slash 命令](#slash-命令)
- [审计链路](#审计链路)
- [复盘与报告](#复盘与报告)

---

## 快速开始

### 首次运行

```bash
# 运行初始化向导，配置 LLM 和审计选项
inferglow init
```

向导会依次提示：

1. **LLM Endpoint** — API 端点地址（必填）
2. **Model name** — 模型名称（默认 `gpt-4o`）
3. **API Key** — API 密钥（可选，可用 `OPENAI_API_KEY` 环境变量）
4. **Provider** — 提供商类型（默认 `openai`）
5. **Enable audit trail?** — 是否启用审计追踪 `(y/N)`
6. **Audit storage path** — 审计日志存储路径（启用审计时提示）
7. **Enable audit signature?** — 是否启用审计签名 `(y/N)`
8. **Signature key** — HMAC-SHA256 签名密钥（启用签名时提示）

### 环境变量覆盖

| 变量 | 作用 |
|------|------|
| `LLM_ENDPOINT` | API 端点 |
| `LLM_MODEL` | 模型名称 |
| `LLM_API_KEY` | API 密钥 |
| `LLM_PROVIDER` | 提供商类型 |
| `COMPRESS_MODEL` | 专用压缩模型 |

环境变量优先级高于配置文件，可用于 CI/CD 场景。

### 配置文件

配置文件位于 `~/.inferglow/config.json`，完整示例：

```json
{
  "llm": {
    "endpoint": "https://api.openai.com/v1",
    "model": "gpt-4o",
    "api_key": "sk-...",
    "provider": "openai"
  },
  "data_dir": "~/.inferglow",
  "workspace_dir": ".",
  "window_tokens": 32000,
  "top_k": 5,
  "sandbox_mode": "trusted_local",
  "context_mode": "hybrid",
  "audit": {
    "enabled": true,
    "storage_path": "~/.inferglow/audit/",
    "signature_key": "optional-hmac-key"
  },
  "features": {
    "memory_injection": true,
    "memory_storage": true,
    "compression": true,
    "constitutional": true,
    "meta_instructions": true,
    "runtime_mode_switch": true,
    "tui_mode": true
  }
}
```

---

## 启动模式

### 交互式 TUI（默认）

```bash
inferglow
```

全屏终端界面，支持实时 token 流式显示、工具执行预览和审批弹窗。

### REPL 模式

```bash
inferglow --cli
```

行式交互循环，输入 `>>>` 提示符后输入消息，支持 `/` 开头的 slash 命令。

### OneShot 模式

```bash
inferglow -z "你的 prompt"
inferglow --oneshot "你的 prompt"
```

单次执行模式：发送 prompt，打印最终响应到 stdout 后退出。适合脚本和管道使用。

### 会话恢复

```bash
inferglow --resume <session-id>
```

恢复指定 session 的上下文，继续之前的对话。

### 子命令

```bash
# 运行初始化向导
inferglow init

# 团队模式（多 agent 协作）
inferglow team --help

# 内存管理
inferglow memory search <query>
inferglow memory stats

# 查看帮助
inferglow --help
```

---

## Slash 命令

在 REPL 或 TUI 模式下，输入 `/` 开头的命令：

| 命令 | 功能 | 说明 |
|------|------|------|
| `/help` | 显示帮助信息 | 列出所有可用命令 |
| `/memory search <q>` | 搜索记忆 | 按关键词搜索长期记忆 |
| `/memory stats` | 记忆统计 | 显示记忆存储的统计信息 |
| `/compact` | 手动压缩 | 触发上下文压缩 |
| `/audit query [flags]` | 查询审计 | 按条件过滤审计条目 |
| `/audit stats` | 审计统计 | 按 source/action 统计审计条目 |
| `/cost` | 用量与成本 | 当前 session 的 token 用量和估算成本 |
| `/cache-stats` | 缓存统计 | 当前 session 的缓存命中率分析 |
| `/cache-report [flags]` | 缓存报告 | 跨 session 的缓存效率复盘报告 |
| `/quit` | 退出会话 | 结束当前会话并退出 |

### 审计命令示例

```bash
# 查询所有 agent 决策审计条目
/audit query --source=agent --action=decision

# 查询指定时间范围内的工具执行记录
/audit query --from=2026-08-01T00:00:00Z --to=2026-08-03T00:00:00Z --action=execute

# 查看审计统计摘要
/audit stats
```

### 缓存报告示例

```bash
# 查看跨 session 缓存效率报告
/cache-report

# 指定时间范围
/cache-report --from=2026-08-01T00:00:00Z --to=2026-08-03T00:00:00Z

# 按模型过滤
/cache-report --model=gpt-4o
```

---

## 审计链路

### 概述

审计链路提供**防篡改的哈希链式日志**，记录所有 agent 决策和工具执行。每条审计条目包含：

- **Source** — 来源（`agent` / `action` / `flow`）
- **Action** — 操作类型（`decision` / `execute` / `request`）
- **Input/Output** — 请求和响应的完整内容
- **Metadata** — 元数据（round、model、token 用量等）
- **Hash** — 基于前一条哈希的链式校验
- **Signature** — 可选的 HMAC-SHA256 签名

### 启用审计

```json
{
  "audit": {
    "enabled": true,
    "storage_path": "~/.inferglow/audit/",
    "signature_key": "your-hmac-key"
  }
}
```

### 存储格式

审计日志按日轮转，存储为 `audit-YYYYMMDD.jsonl` 文件：

```
~/.inferglow/audit/
├── audit-20260801.jsonl
├── audit-20260802.jsonl
└── audit-20260803.jsonl
```

每行是一个 JSON 对象，可通过链式校验验证完整性。

### 安全性

- 所有审计条目通过 `PrevHash` 链接，形成防篡改链
- 可选 HMAC-SHA256 签名，防止未授权修改
- 可通过 `audit.VerifyChain()` 验证完整链的完整性
- 支持 JSON/CSV/Text 多种导出格式

---

## 复盘与报告

### Session 用量统计

每个 session 的 LLM 调用用量自动记录在 `sessions/{uuid}.usage.jsonl` 中。

`/cost` 命令输出示例：

```
Session cost summary:
  Total prompt tokens:      12500
  Total completion tokens:  3400
  Total cached tokens:      8000
  Total reasoning tokens:   500
  Total tokens:             15900
  Total cost:               0.002150 USD
  Record count:             12
```

### 缓存效率分析

`/cache-stats` 命令输出示例：

```
Cache statistics:
  Total prompt tokens:  12500
  Total cached tokens:  8000
  Cache hit rate:       64.00%
  Total cost (w/o cache): 0.003750 USD
  Actual cost:             0.002150 USD
  Savings:                 0.001600 USD
  By model:
    gpt-4o:
      prompt tokens:   12500
      cached tokens:   8000
      cache hit rate:  64.00%
      savings:         0.001600 USD
```

### 跨 Session 报告

`/cache-report` 命令输出示例：

```
Cross-session cache efficiency report:
  Period:                2026-08-01T00:00:00Z – 2026-08-03T00:00:00Z
  Sessions:              5
  Total prompt tokens:   45000
  Total cached tokens:   28000
  Cache hit rate:        62.22%
  Total cost (w/o cache): 0.013500 USD
  Actual cost:              0.007720 USD
  Savings:                  0.005780 USD
  By model:
    gpt-4o:
      prompt tokens:   30000
      cached tokens:   20000
      cache hit rate:  66.67%
      savings:         0.004000 USD
    claude-3-opus:
      prompt tokens:   15000
      cached tokens:   8000
      cache hit rate:  53.33%
      savings:         0.001780 USD
```

### 数据目录结构

```
~/.inferglow/
├── config.json                    # 配置文件
├── constitutional/
│   └── rules.md                   # 宪法规则
├── sessions/
│   ├── index.jsonl                # 会话索引
│   ├── {uuid}.refs.jsonl          # 会话引用追踪
│   └── {uuid}.usage.jsonl         # 会话用量记录
├── audit/
│   ├── audit-20260801.jsonl       # 审计日志（按日轮转）
│   └── audit-20260802.jsonl
├── memory/                        # 长期记忆存储
├── skills/
│   └── global/
└── projects/
    └── default/
        └── skills/
```

---

## 常见问题

### Q: 审计日志文件过大怎么办？

审计日志按日自动轮转，每天一个文件。可以通过工具的 `logrotate` 或定期清理策略管理旧日志。

### Q: 审计禁用后是否有性能开销？

零开销。禁用时（`audit.enabled = false`），审计钩子使用 `NoOpHook`，所有 `Append` 调用立即返回，不产生任何 I/O 或内存分配。

### Q: 缓存命中率低怎么办？

通过 `/cache-report` 分析不同模型的缓存效率，考虑：
- 调整 `sweet_spot_tokens` 配置
- 使用 ThreeZone 或 Hybrid 上下文模式
- 检查系统提示词是否频繁变化导致前缀失效