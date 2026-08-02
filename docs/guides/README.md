# 能力使用指南（Guides）

> 面向**用户**的能力使用指南：告诉你「怎么用 inferglow 的某项能力」。
> 与源码分析（`system-analysis/`）互补——本目录聚焦**使用方式**，`system-analysis/` 聚焦**源码原理**。

## 文档目录

| 指南 | 能力 | 适用场景 |
|------|------|---------|
| [CLI 使用指南](./cli-usage.md) | 配置、启动、Slash 命令、审计链路、复盘报告 | 如何使用 InferGlow CLI 的全部能力 |
| [工具组织与调度](./tool-organization.md) | `Action` / `ActionRegistry` / `ToolGroup` / `GroupRegistry` / `ToolFilter` / `ActionDispatcher` | 如何注册、分组、过滤、调度工具 |
| [上下文管理](./context-management.md) | `ContextManager` / `HybridManager` / 5 种 Mode / 后备存储 | 如何管理对话上下文、选择压缩模式、接驳存储 |

## 与各层文档的关系

inferglow 的文档按「视角」分层，相互补充、不重复：

| 层 | 目录 | 视角 | 回答的问题 |
|----|------|------|-----------|
| 概览 | [README.md](../../README.md) | 用户/心态 | 项目是什么、能做什么、怎么跑起来 |
| **使用指南** | **`docs/guides/`** | **用户/操作** | **某项能力怎么用（API、代码片段、模式）** |
| 源码分析 | [`docs/system-analysis/`](../system-analysis/README.md) | 开发者/原理 | 内部如何实现、模块如何组织、为何这样设计 |
| 可运行示例 | [`examples/`](../../examples/README.md) | 用户/验证 | 直接可跑的完整程序 |

**建议阅读路径**：先读 `README.md` 了解全貌 → 需要动手用某能力时查阅本目录 → 想深入理解内部实现时转向 `system-analysis/` → 复现并验证时运行 `examples/` 中的对应示例。

## 与示例的对应关系

| 指南 | 对应示例 | 说明 |
|------|---------|------|
| 工具组织与调度 | [`examples/example_toolgroup.go`](../../examples/example_toolgroup.go) | 按组注册/列举/过滤工具 |
| 上下文管理 | [`examples/example_context.go`](../../examples/example_context.go) | 初始化上下文管理器、选 Mode、记录与渲染 |

## 约定

- 所有代码片段均可在对应 `examples/` 示例中实际运行验证（`go run`）。
- 术语遵循 `docs/system-analysis/README.md` 的「术语约定」一节。