# Checklist

## 审计链路
- [x] CLIConfig 增加 Audit 配置块（Enabled/StoragePath/SignatureKey），单一开关无冗余
- [x] EnsureDataDirs 创建 audit/ 目录
- [x] buildAgent 在 AuditChain 启用时使用 NewEngineWithAudit
- [x] AgentRuntime 正确持有和关闭 AuditChain 引用
- [x] CLI audit enabled 时审计条目写入 audit-YYYYMMDD.jsonl
- [x] CLI audit disabled 时零额外开销（NoOpHook）

## 审计条目 Token 用量
- [x] engine.go decision 审计条目 metadata 包含 model, provider, input_tokens, output_tokens, cached_tokens, reasoning_tokens
- [x] flow_context_impl.go action 审计条目同样包含 token 用量
- [x] 单元测试验证 metadata 字段正确

## Session 用量聚合
- [x] SessionUsageStats 结构体定义完整
- [x] UsageRecorder 支持 Record/Summary 方法
- [x] 持久化到 sessions/{uuid}.usage.jsonl
- [x] 多轮调用后聚合数据正确
- [x] 持久化数据可重新加载

## CLI 复盘命令
- [x] /audit query 命令可按 source/action/时间范围过滤
- [x] /audit stats 命令显示审计条目统计摘要
- [x] /cost 命令显示当前 session 用量和成本
- [x] /cache-stats 命令显示缓存命中率和节省成本
- [x] 命令输出格式正确，无 panic

## 缓存率复盘报告
- [x] CacheReport 生成器按时间/模型维度聚合
- [x] CLI 表格输出和 JSON 导出
- [x] /cache-report 命令支持 --from/--to/--model 过滤
- [x] 边界情况处理（空数据、单 session）