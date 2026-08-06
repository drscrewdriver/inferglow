# Checklist — Responses API 支持

## 协议层
- [x] `/v1/responses` 端点（py + go）已注册并转发非流式请求
- [x] `/v1/responses` 流式事件流已实时转发，且以 response.completed/failed 判定完整性
- [x] Chat 既有路径（/v1/chat/completions 流式/非流式）行为保持不变（回归通过）

## 解析与审计
- [x] 从 input/output items 正确提取 model、usage（input/output tokens 映射）
- [x] 流式 output_text 通过聚合 output_text.delta 正确重建
- [x] 审计记录 protocol 字段正确标记（chat/responses）
- [x] 校验失败（缺 model / input 空）仍记录审计（审计先于校验）

## session 关联
- [x] session_id 优先级正确：conversation_id → previous_response_id → user message fallback

## 验证
- [x] Go 端到端测试通过（非流式 + 流式）
- [x] Python 端到端测试通过（非流式 + 流式）
- [x] DB 迁移后 protocol/response 字段可查询（Dashboard/API 可见）
