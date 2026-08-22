# 需求族（docs/requirements/）

非 input 线需求按领域拆分为独立需求族文件，统一在本文件夹管理；
input 线需求见 [rich-input-composer-prd.md](../rich-input-composer-prd.md)（富输入 Composer PRD）。

| 需求族 | 文件 | 状态 | 领域 | 拆分自 |
|--------|------|------|------|--------|
| 媒体能力门控 | [media-capability-gating.md](media-capability-gating.md) | 已交付 ✅ | `model/` 模型层 | PRD §13.1 + §11.3 |
| provider 配置存储 | [provider-config-store.md](provider-config-store.md) | 已交付 ✅ | `cli/config/` 配置域 | PRD §13.2 |
| 严格 lint 范围 | [strict-lint-scope.md](strict-lint-scope.md) | 已交付 ✅ | 工程基础设施 | PRD §13.3 + §12.1 |
| TUI 能力补齐 backlog | [tui-capability-backlog.md](tui-capability-backlog.md) | 规划中 📋 | `orchestrator/` / `cli/` | PRD §12 #2/#3/#5 + §11.4 |

## 管理规则

- **一需求族一文件**：文件名 kebab-case，内容自包含（背景 / 决策 / 验收 / 状态 / 交付指针）。
- **状态标注**：已交付 ✅ / 待建 ⏳ / 规划中 📋，状态变更时同步本索引。
- **已交付的需求族保留设计记录与验收**（防回归），不删除。
- **与 PRD 的关系**：input 线 PRD（其 §13「关联需求族」）只保留指针，双向可追溯。
