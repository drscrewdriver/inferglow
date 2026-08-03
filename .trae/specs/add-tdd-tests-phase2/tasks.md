# Tasks

- [x] Task 1: 为 `context/compress` 补全测试（20 测试，全部通过）
  - [x] SubTask 1.1: `mechanical_test.go` — MechanicalCompress L1/L2/L3 去噪、掩码生成、工具名提取
  - [x] SubTask 1.2: `prompts_test.go` — BuildPrompt L1/L2/L3 提示词组装、默认返回原内容
  - [x] SubTask 1.3: `levels_test.go` — LevelThresholds 阈值表、TypeConstraintMatrix 类型约束
  - [x] SubTask 1.4: `idle_test.go` — IdleConsolidator Tick/Reset/Enabled
  - [x] SubTask 1.5: `engine_test.go` — CompressModelChain 接口契约、maskHeaderRegex 正则校验
- [x] Task 2: 为 `context/store` 补全测试（5 单元测试 + 2 集成测试桩）
  - [x] SubTask 2.1: `store_test.go` — StepStore 接口 mock 实现，测试 AppendStep/GetStep/RangeSteps
  - [x] SubTask 2.2: `postgres/store_test.go` — 集成测试（`//go:build integration`），验证 Postgres 实现
  - [x] SubTask 2.3: `redis/cache_test.go` — 集成测试（`//go:build integration`），验证 Redis 缓存实现
- [x] Task 3: 为 `context/tools` 补全测试（30 测试，全部通过）
  - [x] SubTask 3.1: `tools_test.go` — 工具注册、context_search 逻辑
- [x] Task 4: 为 `flow/stage/builtin` 补全测试（14 测试，全部通过）
  - [x] SubTask 4.1: `stages_test.go` — 内置 Stage 注册（triage/plan/coder/reviewer）
- [x] Task 5: 为 `observability` 补全测试（5 测试，全部通过）
  - [x] SubTask 5.1: `collector_test.go` — SpanCollector 写入、读取、清除、溢出行为
- [x] Task 6: 为 `server/config` 补全测试（10 测试，全部通过）
  - [x] SubTask 6.1: `config_test.go` — Config 加载、解析、字段校验
- [x] Task 7: 为 `server/handler` 补全测试（4 测试，全部通过）
  - [x] SubTask 7.1: `handler_test.go` — Health/OpenAPISpec 端点（使用 httptest）
- [x] Task 8: 为 `server/middleware` 补全测试（14 测试，全部通过）
  - [x] SubTask 8.1: `middleware_test.go` — Logging/Recovery/CORS/Auth/RateLimit 中间件
- [x] Task 9: 验证所有测试通过
  - [x] SubTask 9.1: 运行 `go test.count=1 ./context/... ./flow/stage/builtin/... ./observability/... ./server/...`（全部通过）
  - [x] SubTask 9.2: 提交并推送至 GitHub 触发 CI（cbf7d78）

# Task Dependencies
- Tasks 1-8 无依赖，可并行执行
- Task 9 依赖 Tasks 1-8