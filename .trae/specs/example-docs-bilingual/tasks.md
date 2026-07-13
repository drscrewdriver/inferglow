# Tasks

- [x] Task 1: 验证所有现有 example 文件的编译和运行有效性
  - [x] 1.1 编译检查所有 example 文件（go vet）
  - [x] 1.2 运行检查所有 example 文件（go run）
  - [x] 1.3 修复编译/运行错误（如果有）

- [x] Task 2: 创建双语描述文档 - 基础模块
  - [x] 2.1 创建 `example_quickstart.md` — 快速入门双语描述
  - [x] 2.2 创建 `example_action.md` — Action 模块双语描述
  - [x] 2.3 创建 `example_flow.md` — Flow 模块双语描述
  - [x] 2.4 创建 `example_schema.md` — Schema 模块双语描述
  - [x] 2.5 创建 `example_session.md` — Session 模块双语描述

- [x] Task 3: 创建双语描述文档 - 高级模块
  - [x] 3.1 创建 `example_audit.md` — Audit 模块双语描述
  - [x] 3.2 创建 `example_model.md` — Model 模块双语描述
  - [x] 3.3 创建 `example_orchestrator.md` — Orchestrator 模块双语描述
  - [x] 3.4 创建 `example_workspace.md` — Workspace 模块双语描述

- [x] Task 4: 创建双语描述文档 - 可插拔架构
  - [x] 4.1 创建 `example_pluggable.md` — 可插拔安全特性双语描述
  - [x] 4.2 创建 `example_sandbox_enabled.md` — 沙箱执行双语描述

- [x] Task 5: 创建综合 Server 示例
  - [x] 5.1 创建 `example_server_comprehensive.go` — 综合 server 端示例代码
  - [x] 5.2 创建 `example_server_comprehensive.md` — 综合 server 示例双语描述
  - [x] 5.3 验证 server 示例编译和运行

- [x] Task 6: 更新 examples/README.md
  - [x] 6.1 为每个示例添加双语描述文档链接
  - [x] 6.2 更新学习路径表

# Task Dependencies

- Task 2, 3, 4 depend on Task 1 (example validity must be confirmed first)
- Task 5 is independent of Task 2-4 (can run in parallel)
- Task 6 depends on Task 2-5 (all docs must exist before linking)