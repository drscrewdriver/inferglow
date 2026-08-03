# Tasks

- [x] Task 1: 编写 TDD 工作流文档 `docs/guides/development-workflow.md`
- [x] Task 2: 为 `memory/` 模块补全测试（17 个测试，全部通过）
  - [x] SubTask 2.1: `memory_test.go` — Memory 创建、规范化、序列化
  - [x] SubTask 2.2: `store_test.go` — Store 文件操作（Save/Load/List/Archive/Index）
  - [x] SubTask 2.3: `graph_test.go` — JSONGraphStore 三元组操作
  - [x] SubTask 2.4: `memory_bridge_test.go` — MemoryBridge 提取逻辑
- [x] Task 3: 为 `skill/` 模块补全测试（20 个测试，全部通过）
  - [x] SubTask 3.1: `skill_test.go` — Skill 解析（ParseMD）、序列化（ToMD）、校验
  - [x] SubTask 3.2: `store_test.go` — Store 文件操作（List/Read/Save/Delete）
- [x] Task 4: 为 `imbridge/` 模块补全测试（8 个测试，全部通过）
  - [x] SubTask 4.1: `bridge_test.go` — Bridge 消息路由、ChatHandler 注册
  - [x] SubTask 4.2: `telegram_test.go` — TelegramAdapter 配置校验
- [x] Task 5: 为 `desktop/` 模块补全测试（3 个测试，全部通过）
  - [x] SubTask 5.1: `shell_test.go` — DesktopBridge 状态方法
- [x] Task 6: 验证所有测试通过
  - [x] SubTask 6.1: 运行 `go test ./memory/... ./skill/... ./imbridge/... ./desktop/...`（全部通过）
  - [x] SubTask 6.2: 提交并推送至 GitHub 触发 CI（ac9c0df）

# Task Dependencies
- Task 1 无依赖
- Tasks 2-5 无依赖，可并行执行
- Task 6 依赖 Tasks 2-5