# Tasks

- [ ] Task 1: 为 `context/compress` 补全测试（5 文件）
  - [ ] SubTask 1.1: `mechanical_test.go` — MechanicalCompress L1/L2/L3 去噪、掩码生成、工具名提取
  - [ ] SubTask 1.2: `prompts_test.go` — BuildPrompt L1/L2/L3 提示词组装、默认返回原内容
  - [ ] SubTask 1.3: `levels_test.go` — LevelThresholds 阈值表、TypeConstraintMatrix 类型约束
  - [ ] SubTask 1.4: `idle_test.go` — IdleConsolidator Tick/Reset/Enabled
  - [ ] SubTask 1.5: `engine_test.go` — CompressModelChain 接口契约、maskHeaderRegex 正则校验
- [ ] Task 2: 为 `context/store` 补全测试
  - [ ] SubTask 2.1: `store_test.go` — StepStore 接口 mock 实现，测试 AppendStep/GetStep/RangeSteps
  - [ ] SubTask 2.2: `postgres/store_test.go` — 集成测试（`//go:build integration`），验证 Postgres 实现
  - [ ] SubTask 2.3: `redis/cache_test.go` — 集成测试（`//go:build integration`），验证 Redis 缓存实现
- [ ] Task 3: 为 `context/tools` 补全测试
  - [ ] SubTask 3.1: `tools_test.go` — 工具注册、context_search 逻辑
- [ ] Task 4: 为 `flow/stage/builtin` 补全测试
  - [ ] SubTask 4.1: `stages_test.go` — 内置 Stage 注册（ListBuiltinStages、RegisterBuiltins）
- [ ] Task 5: 为 `observability` 补全测试
  - [ ] SubTask 5.1: `collector_test.go` — SpanCollector 写入、读取、清除、溢出行为
- [ ] Task 6: 为 `server/config` 补全测试
  - [ ] SubTask 6.1: `config_test.go` — Config 加载、解析、字段校验
- [ ] Task 7: 为 `server/handler` 补全测试
  - [ ] SubTask 7.1: `handler_test.go` — Health/Version 端点（使用 httptest）
- [ ] Task 8: 为 `server/middleware` 补全测试
  - [ ] SubTask 8.1: `middleware_test.go` — Logging/Recovery/CORS/RateLimit 中间件（使用 httptest）
- [ ] Task 9: 验证所有测试通过
  - [ ] SubTask 9.1: 运行 `go test.count=1 ./context/... ./flow/stage/builtin/... ./observability/... ./server/...`
  - [ ] SubTask 9.2: 提交并推送至 GitHub 触发 CI

# Task Dependencies
- Tasks 1-8 无依赖，可并行执行
- Task 9 依赖 Tasks 1-8