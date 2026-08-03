# Checklist

- [ ] `context/compress` 测试覆盖 MechanicalCompress L1/L2/L3、BuildPrompt 提示词、LevelThresholds 阈值、IdleConsolidator
- [ ] `context/store` 测试覆盖 StepStore 接口 mock 实现
- [ ] `context/store/postgres` 有 `//go:build integration` 集成测试
- [ ] `context/store/redis` 有 `//go:build integration` 集成测试
- [ ] `context/tools` 测试覆盖工具注册逻辑
- [ ] `flow/stage/builtin` 测试覆盖内置 Stage 注册
- [ ] `observability` 测试覆盖 SpanCollector 环形缓冲区
- [ ] `server/config` 测试覆盖 YAML 配置加载
- [ ] `server/handler` 测试覆盖 Health/Version HTTP 端点
- [ ] `server/middleware` 测试覆盖中间件链
- [ ] `go test -count=1 ./context/... ./flow/stage/builtin/... ./observability/... ./server/...` 全部通过
- [ ] GitHub CI 测试通过