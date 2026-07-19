# Checklist

## model ↔ action 操作关系一致性
- [x] 报告包含 ToolCall 从 Provider SSE 到 action 执行的完整数据流图，标注每个环节的代码文件和行号
- [x] 报告包含三种 Provider（OpenAI Chat / Responses API / Anthropic）function call 实现差异对比表
- [x] 报告分析了 native tool calls 与 legacy JSON decision 双路径并存的维护风险
- [x] 报告评估了 components/tool/adapt_action.go 的 schemaToParams 转换完整性
- [x] 报告检查了 SessionExtension 对三种 Provider ToolCall ID 格式的兼容性

## usage 处理链路完整性
- [x] 报告包含 UsageInfo 从 Provider 到 agent 层的完整传递路径，标注每环节代码位置
- [x] 报告指出 executeLoop 跳过 BroadcastResponse 导致 usage 丢失的具体影响
- [x] 报告分析了 failover/attempt 重试场景的 usage 累加逻辑
- [x] 报告评估了 OpenAI Responses API 未解析 usage 字段的影响
- [x] 报告对比了 eino TokenUsage 与 InferGlow UsageInfo 的标准化设计差异

## session 与回溯日志一致性
- [x] 报告分析了 ThreeZoneSession 三区架构的缓存友好性
- [x] 报告评估了 SmartCompressResizeHandler 压缩策略的推理链保留能力
- [x] 报告指出了 session 持久化机制（SaveJSON 只写不读、无自动恢复）的缺陷
- [x] 报告评估了 AuditChain 哈希链的完整性（HMAC 签名、MaxEntries 软上限风险）
- [x] 报告分析了 session 与审计日志无事务性保证的一致性风险
- [x] 报告对比了 eino 中间件机制与 InferGlow resize 策略链

## 沙箱安全边界
- [x] 报告包含 SandboxExecutor.Execute 的完整执行链路追踪
- [x] 报告指出 buildPolicyFromInput 策略可被 LLM 参数操控的安全风险
- [x] 报告评估了多后端隔离能力的策略覆盖差异
- [x] 报告分析了间接注入（tool 返回内容未检测）的风险
- [x] 报告评估了 approval 机制可被 LLM 绕过的风险

## 防注入与 PII 脱敏覆盖度
- [x] 报告评估了 prompt injection detector 的规则覆盖面，列出未覆盖的注入变体
- [x] 报告指出了 PII masker 对非 string content 的处理盲区
- [x] 报告评估了 security_hook 集成的完整性（检查时机和范围）
- [x] 报告分析了 RBAC 与 approval 模块的集成完整性
- [x] 报告对比了参考框架的安全集成模式

## 模块边界与依赖方向
- [x] 报告追踪了 ActionToTool 的所有调用点，评估是否与 buildToolDefinitions 重复
- [x] 报告列出了 model/action/orchestrator/session/security/components 包间的 import 关系
- [x] 报告识别了跨层调用和潜在循环依赖
- [x] 报告评估了 orchestrator/agent 对 model/action 内部细节的耦合程度
- [x] 报告评估了 security 子包间的依赖方向和职责重叠
- [x] 报告产出了可消除的无用代码/桥接层清单，每项标注"删除/合并/保留"

## 代码重复与接口抽象
- [x] 报告量化了三个 Provider SSE 解析的重复代码行数，给出提取方案
- [x] 报告评估了 resize.go 多个 ResizeHandler 的可合并性
- [x] 报告分析了 ModelRequester 接口职责宽度，给出拆分建议
- [x] 报告分析了 ActionExecutor 接口对三种执行器的统一性
- [x] 报告分析了 SessionBackend 接口的抽象层次合理性

## 包结构与目录组织
- [x] 报告统计了各包文件数和导出符号数，识别 god package 风险
- [x] 报告检查了 internal/ 目录使用情况，列出应移入 internal/ 的包
- [x] 报告评估了目录结构是否反映职责边界，给出拆分建议
- [x] 报告对比了 eino 的包组织方式与 InferGlow 的扁平结构

## 扩展性评估
- [x] 报告量化了新增 Provider 需要改动的文件数和接触点
- [x] 报告量化了新增 action 执行器需要改动的文件数和接触点
- [x] 报告量化了新增安全检查规则需要改动的文件数和接触点
- [x] 报告量化了新增沙箱后端需要改动的文件数和接触点
- [x] 报告给出了降低扩展成本的架构建议（插件化等）
- [x] 报告包含扩展性评分矩阵（四类扩展点，InferGlow vs eino）

## 横向对比分析
- [x] 报告包含接口设计对比（泛型 vs 非泛型、不可变 vs 每轮重建）
- [x] 报告包含 tool 集成对比（多模态结果、编排节点 vs 直接调度）
- [x] 报告包含 session/memory 管理对比（中间件机制 vs resize 策略链）
- [x] 报告包含 usage 标准化对比（TokenUsage vs UsageInfo）
- [x] 报告包含安全集成对比（panic 恢复、全栈安全模块）
- [x] 报告包含包结构对比（eino 分层 vs InferGlow 扁平）
- [x] 报告通过 WebSearch 调研了 langchaingo 和 agently 的设计模式

## 问题矩阵与架构调整建议总表
- [x] 报告包含完整的问题矩阵，按 P0/P1/P2 分级
- [x] 每个问题包含具体改进路径和影响的文件列表
- [x] 报告包含差距评分矩阵（InferGlow vs eino vs langchaingo vs agently，10 维度 1-5 分）
- [x] P0 问题包括：executeLoop 跳过 BroadcastResponse 导致 usage 丢失、沙箱策略可被 LLM 参数操控
- [x] 报告包含架构调整建议总表，每个冗余/问题代码标注"删除/合并/拆分/保留"
- [x] "删除"项包含替代路径说明
- [x] "合并"项包含合并目标
- [x] "拆分"项包含拆分方案

## 报告质量
- [x] 报告文件位于 `docs/architecture-consistency-audit.md`
- [x] 报告结构清晰，10 个分析维度 + 问题矩阵 + 架构调整建议总表
- [x] 所有问题包含代码引用（文件名+行号）
- [x] 所有改进建议可执行，不包含模糊表述
