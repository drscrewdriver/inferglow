# Example Bilingual Docs & Server Integration Spec

## Why

当前的 examples 目录缺少配套描述文档，开发者无法快速理解每个示例的用途、模块概念和运行方式。同时缺少一个综合性的 server 端示例来展示完整系统能力（model、action、sandbox、session、audit、flow、schema 等所有基础模块的并联使用）。

## What Changes

1. 验证所有现有 example 文件的可编译性和可运行性
2. 对每个 example 文件创建 1:1 双语描述文档（中英文对照），放置在 `examples/` 目录下
3. 创建综合 server 示例，展示所有基础模块能力的串联使用
4. 更新 examples/README.md 增加双语描述文档的导航

## Impact

- Affected examples: `examples/` 目录下的所有 .go 文件
- Affected code: `examples/` 目录新增 .md 描述文件 + 新增 server 综合示例
- No code changes to existing modules

## ADDED Requirements

### Requirement: Example Validity Check

**All existing examples MUST compile and run without errors**

#### Scenario: Compile check
- **WHEN** running `go vet` on each example file
- **THEN** it MUST succeed with zero errors

#### Scenario: Runtime check
- **WHEN** running each example with `go run`
- **THEN** it MUST exit with code 0

### Requirement: Bilingual Description Documents

**For each example file, create a matching bilingual description document**

#### Scenario: 1:1 correspondence
- **WHEN** an example file `example_<name>.go` exists
- **THEN** a description file `example_<name>.md` MUST exist in the same directory

#### Scenario: Bilingual content structure
- **WHEN** reading the description document
- **THEN** it MUST contain both Chinese (zh) and English (en) sections
- **AND** it MUST include:
  - 概述 / Overview
  - 核心概念 / Core Concepts
  - 前置条件 / Prerequisites
  - 使用示例 / Usage Example
  - 运行验证 / Running the Example
  - 预期输出 / Expected Output

#### Scenario: Module coverage
- **WHEN** reviewing all description documents collectively
- **THEN** they MUST cover: action, flow, schema, session, audit, model, orchestrator, workspace, pluggable security, sandbox

### Requirement: Server Comprehensive Example

**Create a server-based example that demonstrates the full system capabilities**

#### Scenario: Server initialization
- **WHEN** creating the server example
- **THEN** it MUST demonstrate:
  - Server startup with config
  - Agent creation via REST API
  - Chat execution via REST API

#### Scenario: Module penetration
- **WHEN** demonstrating system capabilities
- **THEN** it MUST cover:
  - Model layer: LLM Provider setup and configuration
  - Action layer: Tool registration and execution
  - Session layer: Conversation memory management
  - Audit layer: Enable/disable audit chain, verification
  - Flow layer: Step orchestration
  - Schema layer: Output validation
  - Sandbox layer: Sandbox execution (when enabled)
  - Workspace layer: File operations

#### Scenario: Audit switch
- **WHEN** demonstrating the audit module
- **THEN** it MUST show both:
  - Audit enabled mode (with verification)
  - Audit disabled mode (zero overhead)

### Requirement: README Update

**Update examples/README.md to include navigation for bilingual description documents**

#### Scenario: Navigation update
- **WHEN** reading the examples README
- **THEN** each example entry MUST link to its bilingual description document

## MODIFIED Requirements

(No existing requirements are modified)

## REMOVED Requirements

(No requirements are removed)