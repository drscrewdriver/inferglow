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

## 三、待增强方向

| 方向 | 优先级 | 说明 |
|------|--------|------|
| Multi-Agent 协作 | P1 | Host-Specialist 路由 + 任务委派 |
| 向量检索 | P1 | Embedding-based 语义检索 |
| IM Bridge | P2 | Telegram/飞书/QQ/微信 |
| 桌面端 | P2 | Tauri/Wails 桌面壳 |
| 插件系统 | P3 | 约定优先插件 + 两级权限 |

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