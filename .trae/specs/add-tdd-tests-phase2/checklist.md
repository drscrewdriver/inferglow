# Checklist

- [x] `context/compress` 测试覆盖 MechanicalCompress L1/L2/L3、BuildPrompt 提示词、LevelThresholds 阈值、IdleConsolidator
- [x] `context/store` 测试覆盖 StepStore 接口 mock 实现
- [x] `context/store/postgres` 有 `//go:build integration` 集成测试
- [x] `context/store/redis` 有 `//go:build integration` 集成测试
- [x] `context/tools` 测试覆盖工具注册逻辑
- [x] `flow/stage/builtin` 测试覆盖内置 Stage 注册
- [x] `observability` 测试覆盖 SpanCollector 环形缓冲区
- [x] `server/config` 测试覆盖 YAML 配置加载
- [x] `server/handler` 测试覆盖 Health/OpenAPISpec HTTP 端点
- [x] `server/middleware` 测试覆盖中间件链
- [x] `go test -count=1 ./context/... ./flow/stage/builtin/... ./observability/... ./server/...` 全部通过
- [ ] GitHub CI 测试通过（等待 CI 完成）