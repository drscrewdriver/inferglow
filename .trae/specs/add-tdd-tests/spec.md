# Add TDD Tests & Workflow Spec

## Why

4 个子模块（`memory`、`imbridge`、`desktop`、`skill`）完全没有测试覆盖，且项目缺少 TDD 开发流程说明。补全测试并引入 TDD 工作流文档，提升代码质量门槛。

## What Changes

### 类别 A：补全测试（4 个模块，按 TDD 流程）
- `memory/`：5 个源文件，覆盖 `Memory` CRUD、`Store` 文件操作、`JSONGraphStore` 图存储、`MemoryBridge` 提取
- `imbridge/`：3 个源文件，覆盖 `Bridge` 消息路由、`TelegramAdapter` 平台适配、`ChatHandler` 处理
- `desktop/`：1 个源文件，覆盖 `DesktopBridge` 的 `StartSession`、`SendChat`、`GetStatus` 方法
- `skill/`：2 个源文件，覆盖 `Skill` 解析/序列化、`Store` 文件操作

### 类别 B：新增 TDD 工作流文档
- `docs/guides/development-workflow.md`：描述项目的 TDD 流程（Red-Green-Refactor）、测试命名规范、运行方式、代码审查要求

## Impact
- Affected code: 11 个源文件（4 个模块）+ 1 个文档
- 新增 ~20 个测试文件，~30 个测试函数
- No functional changes to production code

## ADDED Requirements

### Requirement: Test Coverage
每个模块的测试 SHALL 覆盖其导出的所有公共类型和函数的关键路径。

#### Scenario: 核心功能测试
- **WHEN** 运行 `go test ./<module>/...`
- **THEN** 所有测试通过，主要功能路径被覆盖

#### Scenario: 边界与错误路径
- **WHEN** 输入非法参数或模拟 I/O 错误
- **THEN** 测试验证错误被正确处理

### Requirement: TDD 工作流
项目 SHALL 包含 TDD 开发流程说明文档。

#### Scenario: 贡献者阅读
- **WHEN** 新贡献者阅读 `docs/guides/development-workflow.md`
- **THEN** 能理解项目的测试流程、命名规范、运行方式