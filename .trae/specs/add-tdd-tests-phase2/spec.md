# Add TDD Tests Phase 2 Spec

## Why

Phase 1 补全了 4 个模块的测试，但仍剩余 10 个包缺少测试覆盖，其中 `context/compress`（5 个源文件）是核心压缩逻辑，最需要补全。其余 9 个包各 1-2 个源文件，为接口/工具/中间件层。

## What Changes

### 类别 A：核心压缩逻辑（context/compress，5 文件）
- `context/compress/engine.go`：压缩引擎核心逻辑
- `context/compress/levels.go`：压缩级别定义
- `context/compress/mechanical.go`：机械降级
- `context/compress/prompts.go`：压缩提示词
- `context/compress/idle.go`：空闲检测

### 类别 B：上下文存储接口与实现（context/store 系列，5 文件）
- `context/store/store.go`：StepStore 接口——用 mock 实现测试
- `context/store/postgres/`（2 文件）——需要 Postgres，加 `//go:build integration` 标签
- `context/store/redis/`（2 文件）——需要 Redis，加 `//go:build integration` 标签

### 类别 C：上下文工具与内置 Stage（2 文件）
- `context/tools/tools.go`：上下文管理工具（context_search, context_expand 等）
- `flow/stage/builtin/stages.go`：内置 Stage 注册

### 类别 D：可观测性与服务层（4 文件）
- `observability/collector.go`：SpanCollector 环形缓冲区
- `server/config/config.go`：YAML 配置加载与热重载
- `server/handler/handler.go`：HTTP 处理器（Health, Version）
- `server/middleware/middleware.go`：HTTP 中间件（Logging, Recovery, CORS, RateLimit）

## Impact
- Affected code: 10 个包，16 个源文件
- 预期新增 ~40 个测试函数
- `context/store/postgres` 和 `context/store/redis` 需要 `-tags integration` 运行
- No functional changes to production code

## ADDED Requirements

### Requirement: 核心路径测试
每个包的导出类型/函数关键路径 SHALL 有测试覆盖。

#### Scenario: 正常路径
- **WHEN** 调用导出的公共函数
- **THEN** 测试验证返回正确结果

#### Scenario: 边界与错误
- **WHEN** 输入非法参数或模拟错误
- **THEN** 测试验证错误被正确处理

### Requirement: 集成测试
需要外部依赖的包（postgres, redis）SHALL 使用 `//go:build integration` 标签隔离。

#### Scenario: 运行集成测试
- **WHEN** 运行 `go test -tags integration ./context/store/...`
- **THEN** 需要对应服务可用，否则跳过