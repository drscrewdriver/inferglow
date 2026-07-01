# InferGlow 能力成熟度分析 v4 — 四框架横向对比

> 对标基准：LangChain 1.x + LangGraph + LangSmith 能力规格（87 项）
> 分析对象：**InferGlow**（Go）· **AgentScope**（Python，阿里巴巴）· **Agently**（Python）· **LangChainGo**（Go）
> 分析日期：2026-07-29
> v3→v4 变化：6-Wave Optimization（RateLimitHook / Parallel / Middleware / Callbacks+OTel / Memory / Prompt 组件）

---

## 一、项目概况

| 维度 | InferGlow | AgentScope | Agently | LangChainGo |
|------|-----------|------------|---------|-------------|
| **语言** | Go | Python 3.11+ | Python 3.10+ | Go |
| **定位** | Go Agent 基础设施框架 | 生产级多 Agent 平台 | 工程化 AI 应用框架 | Go LangChain 替代 |
| **架构** | 分层模块化 + orchestrator | Agent + Middleware + App 服务层 | 四层架构 + TriggerFlow | 分层模块化 |
| **模型供应商** | **20+** | 9 | ~8 | ~10 |
| **沙箱后端** | **8** | **8** | 4 (Python/Bash/Docker/Node) | 0 |
| **RAG** | ❌ 无 | ✅ 完整 | ❌ 无 | ✅ 基础 |
| **MCP** | ✅ 已实现 | ✅ 3 种传输 | ✅ stdio+HTTP | ❌ |
| **安全体系** | PII+注入+RBAC+限流+审计 | 权限系统 5 模式 | 基础审计 | ❌ |
| **可观测性** | 审计链+OTel | **完整 OTel GenAI** | action_logs | ❌ |
| **REST 服务** | ❌ | ✅ FastAPI 多租户 | FastAPI 基础 | ❌ |
| **维护状态** | 活跃开发 | 活跃（阿里通义） | 活跃 | 活跃 |

---

## 二、InferGlow 逐模块评估（v4 更新）

> v3→v4 主要变化：6-Wave Optimization — RateLimitHook 接入、Parallel 真并行、Middleware 链、Callbacks+OTel、Memory 接口实现、Prompt 组件扩展

### 模块 1：模型连接层（14 项）

| 规格项 | 支持 | 成熟度 | 说明 |
|--------|:----:|--------|------|
| 模型供应商 ≥ 20 | ✅ **20+** | ⭐⭐⭐⭐⭐ | OpenAI/Anthropic/Ollama + DeepSeek/Qwen/GLM/Kimi/StepFun/Baidu/Spark/SenseNova/MiMo/Tencent/Volcengine/ZeroOne/MiniMax/SiliconFlow/OpenRouter 等 |
| 统一接口抽象 | ✅ | ⭐⭐⭐⭐ | `ModelRequester` 接口 + `ModelRequest`/`ModelResponse` |
| 模型路由/降级 | ✅ **完善** | ⭐⭐⭐⭐⭐ | 6 种策略：First/Random/Cost/Latency/Quality/Fallback + `FailoverModelRequester` 自动故障转移+冷却期+健康检查 |
| Rate Limit 管理 | ✅ | ⭐⭐⭐⭐ | `RateLimitWrap` 429 退避 + `ProviderLimiter` 每 Provider RPM/TPM/日限额 + **RateLimitHook executeLoop 前置检查** |
| 流式输出 | ✅ | ⭐⭐⭐⭐ | `<-chan *StreamChunk` SSE 流式 |
| 批处理 | ⚠️ | ⭐⭐ | 无专用 Batch API |
| 函数/工具调用 | ✅ | ⭐⭐⭐⭐ | `ModelRequest.Tools` / `StreamChunk.ToolCalls` 全链路 |
| 多模态输入 | ⚠️ | ⭐⭐ | `Attachment` 字段存在，Provider 层未深入 |
| 结构化输出 | ✅ | ⭐⭐⭐⭐ | `OutputSchema` + `force_json` |
| 异步支持 | ✅ | ⭐⭐⭐⭐ | goroutine + channel |
| Token 计数 | ✅ | ⭐⭐⭐ | `UsageInfo` 完善（优先 provider CompletionTokens，回退 len-based 估算） |
| 成本追踪 | ⚠️ | ⭐⭐ | Routing 有 Cost 策略，但无内置定价表 |
| 预算控制 | ⚠️ | ⭐⭐ | LoopGuard 有 Token 预算 |
| 全链路 async | ✅ | ⭐⭐⭐⭐ | goroutine + channel |

**覆盖率：~82%**（v3: 79%）。**亮点**：20+ Provider 覆盖行业领先、6 种路由策略+自动 failover、reasoning_content 回传（DeepSeek/MiMo/Ollama）、Prefix Cache 感知、RateLimitHook 前置限流、UsageInfo token 精确统计。

---

### 模块 2：Prompt 工程（5 项）

| 规格项 | 支持 | 成熟度 | 说明 |
|--------|:----:|--------|------|
| Prompt Template | ✅ | ⭐⭐⭐⭐ | System/Developer/Instruct/Info 四槽位 + **SystemTemplate 条件段+变量替换** |
| Chat Prompt | ✅ | ⭐⭐⭐⭐ | `ChatMessage` Role 支持 |
| Few-shot | ✅ | ⭐⭐⭐⭐ | **FewShotTemplate** 实现 ChatTemplate（system + 示例对 + 用户输入） |
| 模板组合 | ✅ | ⭐⭐⭐⭐ | **SystemTemplate** 条件段 + Go text/template 变量替换 |
| Prompt 版本管理 | ❌ | ⭐ | 无 |

**覆盖率：75%**（v3: 60%）。**亮点**：三区域 Session prefix cache 优化 + FewShotTemplate/SystemTemplate 独立组件。

---

### 模块 3：Chain 编排（15 项）

| 规格项 | 支持 | 成熟度 | 说明 |
|--------|:----:|--------|------|
| LLMChain | ✅ | ⭐⭐⭐⭐ | Engine.executeLoop |
| SequentialChain | ✅ | ⭐⭐⭐⭐ | Linear Flow |
| PipelineChain | ✅ | ⭐⭐⭐⭐ | TriggerFlow batch/for_each |
| StateGraph | ⚠️ | ⭐⭐⭐ | 事件驱动图 SignalNet |
| 条件分支 | ✅ | ⭐⭐⭐⭐ | Branch.If + match/case |
| 子图/嵌套 | ✅ | ⭐⭐⭐⭐ | SubFlowHandler + SubFlowRegistry |
| 并行执行 | ✅ | ⭐⭐⭐⭐⭐ | WorkerPool + batch_fanout + **RunAgentParallel 真并行（cloneEngineForParallel）** |
| 中断/恢复 | ✅ | ⭐⭐⭐⭐⭐ | Pause/Resume + LifecycleMachine |
| Checkpointer | ✅ | ⭐⭐⭐⭐ | ExecutionSnapshot JSON/YAML |
| 线程管理 | ⚠️ | ⭐⭐⭐ | ExecutionID |
| 断点续跑 | ✅ | ⭐⭐⭐⭐⭐ | Snapshot + Flow.Resume 跨进程 |
| 时间旅行 | ❌ | ⭐ | 无 |
| HITL | ✅ | ⭐⭐⭐⭐⭐ | Pause + InterventionPointHandler |
| Fault Tolerance | ✅ | ⭐⭐⭐⭐ | LifecycleMachine + panic 恢复 |
| 跨会话存储 | ⚠️ | ⭐⭐⭐ | Session JSON/YAML 持久化 |

**覆盖率：63%**（v3: 60%）。13 种算子 + LifecycleMachine + SubFlow 深拷贝 + RunAgentParallel 真并行。

---

### 模块 4：Agent 核心（10 项）

| 规格项 | 支持 | 成熟度 | 说明 |
|--------|:----:|--------|------|
| ReAct Agent | ✅ | ⭐⭐⭐⭐ | Engine.executeLoop PLAN → EXECUTE |
| Plan-and-Execute | ✅ | ⭐⭐⭐⭐ | `TaskDAGStrategy` + `RepairLLMJSON` |
| Tool-Calling Agent | ✅ | ⭐⭐⭐⭐ | ActionDispatcher + ActionExtension |
| Zero-shot Agent | ✅ | ⭐⭐⭐ | systemPrompt 驱动 |
| Multi-Agent 协作 | ❌ | ⭐ | 无内置 Multi-Agent 路由 |
| Sub-Agent | ⚠️ | ⭐⭐⭐ | SubFlow 可视为轻量 Sub-Agent |
| Task Decomposition | ✅ | ⭐⭐⭐⭐ | `TaskDAG` 模型生成任务图+拓扑排序+依赖传递 |
| Self-Correction | ✅ | ⭐⭐⭐⭐ | RepairLLMJSON 三级修复 + **Middleware 链支持自定义校正策略** |
| Deep Agents | ❌ | ⭐ | 无 |

**覆盖率：58%**（v3: 55%）。**亮点**：LoopGuard + TurnLoop 抢占控制 + CancelManager + **Middleware 链** + **Callbacks 生命周期钩子**。

---

### 模块 5：记忆管理（6 项）

| 规格项 | 支持 | 成熟度 | 说明 |
|--------|:----:|--------|------|
| BufferMemory | ✅ | ⭐⭐⭐⭐ | Session.FullContext |
| BufferWindowMemory | ✅ | ⭐⭐⭐⭐ | ContextWindow + MaxLength |
| SummaryMemory | ✅ | ⭐⭐⭐⭐ | **独立 SummaryMemory 实现**：token 阈值触发自动摘要旧消息 + Summarizer 接口 |
| TokenBufferMemory | ✅ | ⭐⭐⭐⭐ | **独立 TokenBufferMemory 实现**：token 预算裁剪，精确/快速双模式估算 |
| VectorStoreMemory | ❌ | ⭐ | 无 |
| 短/长期分离 | ✅ | ⭐⭐⭐⭐ | ThreeZoneSession + Memo |

**覆盖率：67%**（v3: 50%）。三级 resize 链 + PII Masker Hook + **Memory 接口统一 + SummaryMemory/TokenBufferMemory 独立实现**。

---

### 模块 6：向量存储与 RAG（9 项）

全部 ❌。**覆盖率 0%**。与 Agently 一致，完全未涉及 RAG。

---

### 模块 7：工具系统（7 项）

| 规格项 | 支持 | 成熟度 | 说明 |
|--------|:----:|--------|------|
| 自定义工具 | ✅ | ⭐⭐⭐⭐ | ActionRegistry.Register 三种签名 |
| 工具描述自动生成 | ✅ | ⭐⭐⭐⭐ | `schema_from_func` 自动生成 JSON Schema |
| 内置工具 | ✅ **8 个** | ⭐⭐⭐⭐ | bash/code/calculator/file_read/file_write/json_processor/url_fetch/web_search |
| 工具输出解析 | ✅ | ⭐⭐⭐ | ActionResult OK/Status/Result/Error/Metadata |
| 工具路由 | ✅ | ⭐⭐⭐⭐ | ActionDispatcher |
| 沙箱执行 | ✅ **8 后端** | ⭐⭐⭐⭐⭐ | Docker/gVisor/Landlock/Seatbelt/Windows/E2B/TrustedLocal/Bubblewrap |
| 工具组合 | ✅ | ⭐⭐⭐ | ActionExtension + ActionDispatcher |

**额外亮点**：MCPExecutor 已完善（MCP tools/call 代理 + DiscoverMCPTools 自动发现）、ActionSpec 安全规格、LoopGuard、AuditHook、Panic 恢复。

**覆盖率：86%**（v2: 55%）。**MCP 已实现是最大变化**。

---

### 模块 8：数据规格校验（4 项）

**覆盖率 100%**（不变）。泛型推导 `DefineOutput[T]()` + ExtractJSON 三级策略 + Blueprint 序列化。

---

### 模块 9：安全与合规（5 项）

| 规格项 | 支持 | 成熟度 | 说明 |
|--------|:----:|--------|------|
| Prompt 注入防护 | ✅ | ⭐⭐⭐⭐ | `prompt_injection/` 三级严重度 + 关键词/正则检测 + 自定义模式 |
| PII 脱敏 | ✅ | ⭐⭐⭐⭐ | `pii/` 正则模式（邮箱/电话/身份证/银行卡）+ Input/Output 双向 |
| 审计日志 | ✅ | ⭐⭐⭐⭐⭐ | 链式哈希 SHA-256 + HMAC 签名 + 全链验证 + JSONL 轮转 |
| 访问控制 | ✅ | ⭐⭐⭐⭐ | `rbac/` 4 角色（viewer/editor/admin/custom）+ PermissionMatrix + RBACMiddleware |
| Secret 管理 | ✅ | ⭐⭐⭐ | ModelRequester API Key + ProviderLimiter 限额 |

**额外亮点**：7 种沙箱后端、ActionSpec 安全规格、LoopGuard、LifecycleMachine、Panic 恢复、`ratelimit/` TokenBucket 令牌桶。

**覆盖率：80%**（v2: 40%）。**安全体系从短板变为最强维度之一**。

---

### 模块 10：可观测性与运维（9 项）

| 规格项 | 支持 | 成熟度 | 说明 |
|--------|:----:|--------|------|
| 分布式追踪 | ✅ | ⭐⭐⭐⭐ | 审计链 + Engine OTel 集成 + **CallbacksTracer 桥接 OTel span** |
| 评估框架 | ❌ | ⭐ | 无 |
| A/B 测试 | ❌ | ⭐ | 无 |
| 自动修复 | ✅ | ⭐⭐⭐ | RepairLLMJSON |
| 性能监控 | ✅ | ⭐⭐⭐ | StepLogEntry.Duration + AuditEntry.Duration + **Callbacks 生命周期 timing** |
| REST API | ❌ | ⭐ | 无 |
| Agent Server | ❌ | ⭐ | 无 |
| 容器化 | ✅ | ⭐⭐⭐⭐ | Go 二进制 |

**覆盖率：40%**（v3: 35%）。差距：无 Eval/A-B、无 REST 托管。**提升**：CallbacksTracer 桥接 OTel span（SpanAgentRun/SpanLLMCall/SpanToolCall）。

---

### 模块 11：通道适配（3 项）

**覆盖率 15%**（不变）。

---

### InferGlow v4 综合矩阵

| 能力域 | 项数 | 完整 | 部分 | 不支持 | 覆盖率 | 成熟度 | v3→v4 变化 |
|--------|:----:|:----:|:----:|:------:|:------:|:------:|:----------:|
| 模型连接层 | 14 | 9 | 3 | 2 | **82%** | ⭐⭐⭐⭐ | 79%→82% |
| Prompt 工程 | 5 | 3 | 1 | 1 | **75%** | ⭐⭐⭐⭐ | 60%→75% |
| Chain 编排 | 15 | 8 | 2 | 5 | **63%** | ⭐⭐⭐⭐ | 60%→63% |
| Agent 核心 | 10 | 5 | 3 | 2 | **58%** | ⭐⭐⭐⭐ | 55%→58% |
| 记忆管理 | 6 | 3 | 2 | 1 | **67%** | ⭐⭐⭐⭐ | 50%→67% |
| 向量存储与 RAG | 9 | 0 | 0 | 9 | **0%** | ⭐ | 不变 |
| 工具系统 | 7 | 5 | 2 | 0 | **86%** | ⭐⭐⭐⭐⭐ | 不变 |
| 数据规格校验 | 4 | 4 | 0 | 0 | **100%** | ⭐⭐⭐⭐⭐ | 不变 |
| 安全与合规 | 5 | 4 | 1 | 0 | **80%** | ⭐⭐⭐⭐⭐ | 不变 |
| 可观测性与运维 | 9 | 3 | 3 | 3 | **40%** | ⭐⭐⭐ | 35%→40% |
| 通道适配 | 3 | 0 | 1 | 2 | **15%** | ⭐⭐ | 不变 |
| **总计** | **87** | **44** | **18** | **25** | **68%** | **⭐⭐⭐⭐** | **65%→68%** |

---

## 三、Agently 逐模块评估

> 分析对象：[AgentEra/Agently](https://github.com/AgentEra/Agently)（~1,623 Stars，v4.1.4.1）
> 核心定位：**Engineering-grade AI 应用开发框架**——契约优先（Contract-First）+ TriggerFlow 编排 + Action Runtime 工具

### 模块 1：模型连接层（14 项）

| 规格项 | 支持 | 成熟度 | 说明 |
|--------|:----:|--------|------|
| 模型供应商 ≥ 20 | ~8+ | ⭐⭐⭐ | OpenAI / DeepSeek / Claude / Qwen / Ollama / 任意 OpenAI 兼容端点 |
| 统一接口 | ✅ | ⭐⭐⭐⭐ | `Agently.create_agent()` 统一接口，内置模型 Profile 系统 |
| 模型路由/降级 | ⚠️ | ⭐⭐⭐ | `ModelRequester` 插件切换 Provider，无自动降级 |
| Rate Limit | ❌ | ⭐ | 无内置 |
| 流式输出 | ✅ | ⭐⭐⭐⭐ | 标准 stream 模式 |
| 批处理 | ⚠️ | ⭐⭐⭐ | 基础批量调用 |
| 函数/工具调用 | ✅ | ⭐⭐⭐⭐ | Action Runtime 内置工具调用调度 |
| 多模态 | ⚠️ | ⭐⭐⭐ | 依赖底层 Provider 支持 |
| 结构化输出 | ✅⭐ | ⭐⭐⭐⭐⭐ | **核心卖点**：Contract-First Schema，`ensure_keys` + `.validate()` |
| 异步 | ✅ | ⭐⭐⭐⭐ | 全链路 async/await |
| Token 计数 | ❌ | ⭐ | 无内置 |
| 成本追踪 | ❌ | ⭐ | 无内置 |
| 预算控制 | ❌ | ⭐ | 无内置 |
| 全链路 async | ✅ | ⭐⭐⭐⭐ | Python async 全链路 |

**覆盖率：57%**。**亮点**：结构化输出是核心优势（Contract-First + `ensure_all_keys=True`），优于 LangChain 的 Pydantic。**差距**：无成本追踪、无 Rate Limit、供应商覆盖少。

---

### 模块 2：Prompt 工程（5 项）

| 规格项 | 支持 | 成熟度 | 说明 |
|--------|:----:|--------|------|
| Prompt Template | ✅ | ⭐⭐⭐⭐ | YAML/JSON 模板（`$ensure_all_keys`） |
| Chat Prompt | ✅ | ⭐⭐⭐⭐ | input/instruct/info/output 四槽位设计 |
| Few-shot | ❌ | ⭐ | 无 ExampleSelector |
| 模板组合 | ⚠️ | ⭐⭐⭐ | Hierarchical Settings 组合 |
| Prompt 版本 | ❌ | ⭐ | 无 |

**覆盖率：40%**。四槽位设计（input/instruct/info/output）是独特路径，但生态工具少。

---

### 模块 3：Chain 编排（15 项）

> **核心亮点**：TriggerFlow 是除 LangGraph 外最接近 StateGraph 级别的编排能力。

| 规格项 | 支持 | 成熟度 | 说明 |
|--------|:----:|--------|------|
| LLMChain | ✅ | ⭐⭐⭐⭐ | `agent.input().output().get_result()` |
| SequentialChain | ✅ | ⭐⭐⭐⭐ | `flow.to(A).to(B).to(C)` |
| PipelineChain | ✅ | ⭐⭐⭐⭐ | `flow.batch()` Fan-out + `for_each()` Fan-in |
| StateGraph | ⚠️ | ⭐⭐⭐ | 事件驱动图（`when`/`emit`），非 TypedDict 状态图 |
| 条件分支 | ✅ | ⭐⭐⭐⭐ | `if/elif/else`、`match/case` |
| 子图/嵌套 | ❌ | ⭐ | 无显式子图机制 |
| 并行执行 | ✅ | ⭐⭐⭐⭐ | `flow.batch()` + `for_each(concurrency=N)` |
| 中断/恢复 | ✅⭐ | ⭐⭐⭐⭐⭐ | `seal()`/`unseal()` + `pause/resume`，比 LangChain 更明确的状态机 |
| Checkpointer | ✅⭐ | ⭐⭐⭐⭐⭐ | `execution.save()` / `flow.load()` 磁盘持久化 |
| 线程管理 | ⚠️ | ⭐⭐⭐ | Execution handle |
| 断点续跑 | ✅⭐ | ⭐⭐⭐⭐⭐ | `execution.save()` + `restored.load()` 跨进程恢复 |
| 时间旅行 | ❌ | ⭐ | 无 |
| HITL | ✅⭐ | ⭐⭐⭐⭐⭐ | `pause`/`resume` + `emit("UserFeedback")`，一等公民 |
| Fault Tolerance | ✅ | ⭐⭐⭐⭐ | `auto_close` 超时 + `seal()` 状态冻结 |
| 跨会话存储 | ⚠️ | ⭐⭐⭐ | Session 持久化（JSON/YAML） |

**覆盖率：60%**。**亮点**：`open → sealed → closed` 明确状态机 + Blueprint 序列化（YAML/JSON flow topology export）是独特能力。**差距**：无子图、无时间旅行。

---

### 模块 4：Agent 核心（10 项）

| 规格项 | 支持 | 成熟度 | 说明 |
|--------|:----:|--------|------|
| ReAct | ✅ | ⭐⭐⭐⭐ | Action Runtime 内置 ReAct 循环 |
| Plan-and-Execute | ✅ | ⭐⭐⭐⭐ | TaskDAG + `create_dynamic_task`，支持目标/成功标准 |
| Tool-Calling | ✅ | ⭐⭐⭐⭐ | Action Runtime 三层（planning → loop → execution） |
| Zero-shot | ✅ | ⭐⭐⭐ | instruct 槽位驱动 |
| Multi-Agent | ❌ | ⭐ | 无内置 Multi-Agent 路由 |
| Sub-Agent | ❌ | ⭐ | 无 |
| Task Decomposition | ✅ | ⭐⭐⭐⭐ | `create_dynamic_task` + `create_task` 目标拆解 |
| Self-Correction | ⚠️ | ⭐⭐⭐ | Task 级别 success_criteria 校验 |
| Deep Agents | ⚠️ | ⭐⭐⭐ | `create_dynamic_task` 轻量版 |

**覆盖率：55%**。**亮点**：TaskDAG 是独特能力——定义任务目标、成功标准、重试策略。**差距**：无 Multi-Agent。

---

### 模块 5：记忆管理（6 项）

| 规格项 | 支持 | 成熟度 | 说明 |
|--------|:----:|--------|------|
| BufferMemory | ✅ | ⭐⭐⭐⭐ | Session 自动维护对话历史 |
| BufferWindow | ✅ | ⭐⭐⭐⭐ | `session.max_length` 控制窗口 |
| SummaryMemory | ⚠️ | ⭐⭐⭐ | 自定义 `resize_handler` |
| TokenBuffer | ⚠️ | ⭐⭐⭐ | Token 级别窗口控制 |
| VectorStoreMemory | ❌ | ⭐ | 无内置向量检索记忆 |
| 短/长期分离 | ⚠️ | ⭐⭐⭐ | Session 持久化支持 |

**覆盖率：50%**。Session 设计简洁（JSON/YAML 持久化 + 窗口裁剪），但无向量检索能力。

---

### 模块 6：向量存储与 RAG（9 项）

全部 ❌。**覆盖率 0%**。**Agently 完全未涉及 RAG 能力**——这是最大功能空白。

---

### 模块 7：工具系统（7 项）

> **核心亮点**：Action Runtime v4.1 是最大亮点——三层插件架构。

| 规格项 | 支持 | 成熟度 | 说明 |
|--------|:----:|--------|------|
| 自定义工具 | ✅⭐ | ⭐⭐⭐⭐⭐ | `@agent.action_func` / `@agent.tool_func` 装饰器，零样板代码 |
| 描述自动生成 | ✅ | ⭐⭐⭐⭐ | 从函数签名/docstring 自动提取 |
| 内置工具 | ⚠️ | ⭐⭐⭐ | `Search` / `Browse`（builtins.actions） |
| 输出解析 | ✅ | ⭐⭐⭐⭐ | `ActionResult` 含 `model_digest` + `artifact_refs` |
| 工具路由 | ✅ | ⭐⭐⭐⭐ | Action Runtime planning → dispatch |
| 沙箱执行 | ✅ | ⭐⭐⭐⭐⭐ | PythonSandbox / BashSandbox / Docker / Node.js |
| 工具组合 | ✅ | ⭐⭐⭐⭐ | Action Runtime 三层编排 |

**额外亮点**：MCPExecutor（stdio + HTTP）、完整 action_logs、三层可替换架构（ActionExtension → ActionRuntime → ActionExecutor）。

**覆盖率：75%**。**Action Runtime 是 Agently 最强差异化能力**——MCP + 沙盒 + 本地函数统一调度。

---

### 模块 8：数据规格校验（4 项）

| 规格项 | 支持 | 成熟度 | 说明 |
|--------|:----:|--------|------|
| JSON 模式 | ✅⭐ | ⭐⭐⭐⭐⭐ | **Contract-First 核心卖点**：`output({"field": (type, "desc", True)})` |
| 响应校验 | ✅⭐ | ⭐⭐⭐⭐⭐ | `ensure_keys` + `ensure_all_keys=True` + `.validate()` + `validate_handler` |
| 类型安全提取 | ✅ | ⭐⭐⭐⭐ | Python 类型提示 + 运行时校验 |
| I/O Schema | ✅ | ⭐⭐⭐⭐ | 四槽位 Schema + Hierarchical Settings |

**覆盖率：100%**。**Agently 的 Contract-First 是 LangChain Pydantic 之上更严格的一层**——`ensure_all_keys=True` 强制全结构校验、`ensure_keys` 运行时路径校验。

---

### 模块 9：安全与合规（5 项）

| 规格项 | 支持 | 成熟度 | 说明 |
|--------|:----:|--------|------|
| Prompt 注入防护 | ❌ | ⭐ | 无 |
| PII 脱敏 | ❌ | ⭐ | 无 |
| 审计日志 | ✅ | ⭐⭐⭐⭐ | `action_logs` 完整记录每次工具调用（input/output/timing） |
| 访问控制 | ❌ | ⭐ | 无 |
| Secret 管理 | ⚠️ | ⭐⭐⭐ | Hierarchical Settings + env 变量注入 |

**覆盖率：25%**。Action 日志完整但无高级安全能力。

---

### 模块 10：可观测性与运维（9 项）

| 规格项 | 支持 | 成熟度 | 说明 |
|--------|:----:|--------|------|
| 分布式追踪 | ✅ | ⭐⭐⭐⭐ | `action_logs` 全链路日志（input/output/timing） |
| 评估框架 | ❌ | ⭐ | 无 |
| A/B 测试 | ❌ | ⭐ | 无 |
| 自动修复 | ❌ | ⭐ | 无 |
| 性能监控 | ⚠️ | ⭐⭐⭐ | Action timing + 日志 |
| REST API | ⚠️ | ⭐⭐⭐ | FastAPI 集成（`FastAPIHelper`） |
| Agent Server | ❌ | ⭐ | 无托管运行时 |
| 容器化 | ✅ | ⭐⭐⭐⭐ | Python 包标准部署 |

**额外亮点**：Blueprint 序列化、DevTools 脚手架、Skills 系统。

**覆盖率：35%**。

---

### 模块 11：通道适配（3 项）

| 规格项 | 支持 | 成熟度 | 说明 |
|--------|:----:|--------|------|
| 即时通讯 | ❌ | ⭐ | 无 |
| 邮件 | ❌ | ⭐ | 无 |
| Webhook/API | ✅ | ⭐⭐⭐⭐ | FastAPI + Execution REST 接口 |

**覆盖率：35%**。

---

### Agently 综合矩阵

| 能力域 | 项数 | 完整 | 部分 | 不支持 | 覆盖率 | 成熟度 |
|--------|:----:|:----:|:----:|:------:|:------:|:------:|
| 模型连接层 | 14 | 5 | 4 | 5 | **57%** | ⭐⭐⭐ |
| Prompt 工程 | 5 | 2 | 1 | 2 | **40%** | ⭐⭐⭐ |
| Chain 编排 | 15 | 7 | 3 | 5 | **60%** | ⭐⭐⭐⭐ |
| Agent 核心 | 10 | 5 | 2 | 3 | **55%** | ⭐⭐⭐⭐ |
| 记忆管理 | 6 | 2 | 3 | 1 | **50%** | ⭐⭐⭐ |
| 向量存储与 RAG | 9 | 0 | 0 | 9 | **0%** | ⭐ |
| 工具系统 | 7 | 5 | 1 | 1 | **75%** | ⭐⭐⭐⭐⭐ |
| 数据规格校验 | 4 | 4 | 0 | 0 | **100%** | ⭐⭐⭐⭐⭐ |
| 安全与合规 | 5 | 1 | 1 | 3 | **25%** | ⭐⭐⭐ |
| 可观测性与运维 | 9 | 2 | 3 | 4 | **35%** | ⭐⭐⭐ |
| 通道适配 | 3 | 1 | 0 | 2 | **35%** | ⭐⭐⭐ |
| **总计** | **87** | **34** | **18** | **35** | **50%** | **⭐⭐⭐⭐** |

---

## 四、LangChainGo 逐模块评估

> 分析对象：[tmc/langchaingo](https://github.com/tmc/langchaingo)（~8K Stars）
> 核心定位：**Go 语言的 LangChain 替代框架**——基础 Chain/Memory/RAG 能力

### 模块 1：模型连接层（14 项）

| 规格项 | 支持 | 成熟度 | 说明 |
|--------|:----:|--------|------|
| 模型供应商 ≥ 20 | ~10 家 | ⭐⭐⭐ | OpenAI / Anthropic / Google / Azure / Ollama 等 |
| 统一接口 | ✅ | ⭐⭐⭐⭐ | `llms.LLM`、`llms.ChatModel`、`schema.Embedder` |
| 模型路由/降级 | ❌ | ⭐ | 无内置路由或降级 |
| Rate Limit | ⚠️ | ⭐⭐ | 部分 Provider 有重试，无统一管理 |
| 流式输出 | ✅ | ⭐⭐⭐⭐ | `llms.Stream()` / `Chat()` SSE 流式 |
| 批处理 | ⚠️ | ⭐⭐⭐ | `Batch()` 方法，并发控制弱 |
| 函数/工具调用 | ✅ | ⭐⭐⭐ | Function Calling，Schema 生成基础 |
| 多模态 | ❌ | ⭐ | 仅文本 |
| 结构化输出 | ⚠️ | ⭐⭐ | JSON 输出，无 Pydantic 级强制校验 |
| 异步 | ⚠️ | ⭐⭐⭐ | 部分 Provider async，非全链路 |
| Token 计数 | ⚠️ | ⭐⭐ | 部分 Provider 返回，无统一工具 |
| 成本追踪 | ❌ | ⭐ | 无定价表 |
| 预算控制 | ❌ | ⭐ | 无 |
| 全链路 async | ❌ | ⭐ | goroutine 模型与 Python async 不同 |

**覆盖率：50%**。**差距**：无路由降级、无成本追踪、无多模态、异步不完整。

---

### 模块 2：Prompt 工程（5 项）

| 规格项 | 支持 | 成熟度 | 说明 |
|--------|:----:|--------|------|
| Prompt Template | ✅ | ⭐⭐⭐⭐ | `prompts.PromptTemplate` 变量替换 |
| Chat Prompt | ✅ | ⭐⭐⭐⭐ | `prompts.ChatPromptTemplate` system/user/assistant |
| Few-shot | ❌ | ⭐ | 无 ExampleSelector |
| 模板组合 | ⚠️ | ⭐⭐⭐ | `Runnable` 组合，API 不如 Python 直观 |
| Prompt 版本 | ❌ | ⭐ | 无 |

**覆盖率：45%**。

---

### 模块 3：Chain 编排（15 项）

> **核心差距**：LangChainGo 完全缺失 LangGraph 级别的图编排能力。

| 规格项 | 支持 | 成熟度 | 说明 |
|--------|:----:|--------|------|
| LLMChain | ✅ | ⭐⭐⭐⭐ | `chains.LLMChain` prompt + LLM + output parser |
| SequentialChain | ✅ | ⭐⭐⭐ | `chains.SequentialChain` |
| PipelineChain | ❌ | ⭐ | 无并行 Fan-out/Fan-in |
| StateGraph | ❌ | ⭐ | **无图编排能力** |
| 条件分支 | ❌ | ⭐ | 无 |
| 子图/嵌套 | ❌ | ⭐ | 无 |
| 并行执行 | ⚠️ | ⭐⭐ | goroutine 手动并行 |
| 中断/恢复 | ❌ | ⭐ | 无 |
| Checkpointer | ❌ | ⭐ | **无状态快照** |
| 线程管理 | ❌ | ⭐ | 无 thread_id |
| 断点续跑 | ❌ | ⭐ | 无 Checkpoint |
| 时间旅行 | ❌ | ⭐ | 无 |
| HITL | ❌ | ⭐ | 无 |
| Fault Tolerance | ❌ | ⭐ | 无节点级故障恢复 |
| 跨会话存储 | ❌ | ⭐ | 无跨线程 KV |

**覆盖率：13%**。**这是 LangChainGo 最大短板**——完全缺失图编排、Checkpoint、HITL、断点续跑。

---

### 模块 4：Agent 核心（10 项）

| 规格项 | 支持 | 成熟度 | 说明 |
|--------|:----:|--------|------|
| ReAct | ✅ | ⭐⭐⭐ | `agents.ReAct` |
| Plan-and-Execute | ❌ | ⭐ | 无 |
| Tool-Calling | ⚠️ | ⭐⭐⭐ | Function Calling 驱动 |
| Zero-shot | ✅ | ⭐⭐⭐ | `agents.ZeroShotAgent` |
| Multi-Agent | ❌ | ⭐ | 无 |
| Sub-Agent | ❌ | ⭐ | 无 |
| Task Decomposition | ❌ | ⭐ | 无 `write_todos` 等效 |
| Self-Correction | ❌ | ⭐ | 无 |
| Deep Agents | ❌ | ⭐ | 无 |

**覆盖率：20%**。仅支持 ReAct/Zero-shot 两种基础模式。

---

### 模块 5：记忆管理（6 项）

| 规格项 | 支持 | 成熟度 | 说明 |
|--------|:----:|--------|------|
| BufferMemory | ✅ | ⭐⭐⭐⭐ | `memory.ConversationBufferMemory` |
| BufferWindow | ✅ | ⭐⭐⭐⭐ | `memory.ConversationBufferWindowMemory` |
| SummaryMemory | ❌ | ⭐ | 无 |
| TokenBuffer | ❌ | ⭐ | 无 |
| VectorStoreMemory | ⚠️ | ⭐⭐⭐ | 可通过 `stores` 实现，无专用组件 |
| 短/长期分离 | ❌ | ⭐ | 无 Checkpointer + Store |

**覆盖率：35%**。

---

### 模块 6：向量存储与 RAG（9 项）

| 规格项 | 支持 | 成熟度 | 说明 |
|--------|:----:|--------|------|
| 文档加载器 | ⚠️ ~10 种 | ⭐⭐ | PDF/HTML/Markdown 等基础格式 |
| 文本分割器 | ✅ | ⭐⭐⭐⭐ | `textsplitter` RecursiveCharacter |
| Embedding | ✅ | ⭐⭐⭐ | OpenAI/Google/Ollama |
| 向量数据库 | ✅ ~10 后端 | ⭐⭐⭐ | Pinecone/Chroma/Weaviate/Milvus/Pgvector |
| 检索策略 | ⚠️ | ⭐⭐⭐ | 相似度搜索，MMR 有限 |
| 重排序 | ❌ | ⭐ | 无 |
| 多路召回 | ❌ | ⭐ | 无 |
| 元数据过滤 | ⚠️ | ⭐⭐⭐ | 部分后端支持 |

**覆盖率：45%**。**RAG 是 LangChainGo 相对于 InferGlow/Agently 的优势**。

---

### 模块 7：工具系统（7 项）

| 规格项 | 支持 | 成熟度 | 说明 |
|--------|:----:|--------|------|
| 自定义工具 | ✅ | ⭐⭐⭐ | `tool` 包，自动 Schema 生成 |
| 描述自动生成 | ⚠️ | ⭐⭐⭐ | 函数签名提取，无 docstring 解析 |
| 内置工具 | ⚠️ ~10 种 | ⭐⭐ | 计算器、搜索等基础工具 |
| 输出解析 | ⚠️ | ⭐⭐⭐ | 有工具返回解析 |
| 工具路由 | ✅ | ⭐⭐⭐ | Agent 内置工具选择 |
| 沙箱执行 | ❌ | ⭐ | **无隔离代码执行** |
| 工具组合 | ⚠️ | ⭐⭐ | 基础编排 |

**覆盖率：45%**。无沙箱、无 MCP。

---

### 模块 8：数据规格校验（4 项）

| 规格项 | 支持 | 成熟度 | 说明 |
|--------|:----:|--------|------|
| JSON 模式 | ⚠️ | ⭐⭐⭐ | 有 JSON 输出，无强制 Schema 校验 |
| 响应校验 | ❌ | ⭐ | Go 无 Pydantic，无内置 |
| 类型安全提取 | ⚠️ | ⭐⭐⭐ | Go 静态类型天然支持 |
| I/O Schema | ⚠️ | ⭐⭐⭐ | Struct 标签，无运行时校验 |

**覆盖率：50%**。

---

### 模块 9：安全与合规（5 项）

| 规格项 | 支持 | 成熟度 | 说明 |
|--------|:----:|--------|------|
| Prompt 注入防护 | ❌ | ⭐ | 无 |
| PII 脱敏 | ❌ | ⭐ | 无 |
| 审计日志 | ❌ | ⭐ | 无 |
| 访问控制 | ❌ | ⭐ | 无 |
| Secret 管理 | ⚠️ | ⭐⭐ | 环境变量，无自动清除 |

**覆盖率：10%**。**完全无安全合规内置能力**。

---

### 模块 10：可观测性与运维（9 项）

| 规格项 | 支持 | 成熟度 | 说明 |
|--------|:----:|--------|------|
| 分布式追踪 | ❌ | ⭐ | 无 |
| 评估框架 | ❌ | ⭐ | 无 |
| A/B 测试 | ❌ | ⭐ | 无 |
| 自动修复 | ❌ | ⭐ | 无 |
| 性能监控 | ❌ | ⭐ | 无 |
| REST API | ❌ | ⭐ | 需自行封装 |
| Agent Server | ❌ | ⭐ | 无 |
| 容器化 | ✅ | ⭐⭐⭐⭐ | Go 二进制天然容器化 |

**覆盖率：15%**。

---

### 模块 11：通道适配（3 项）

| 规格项 | 支持 | 成熟度 | 说明 |
|--------|:----:|--------|------|
| 即时通讯 | ❌ | ⭐ | 无 |
| 邮件 | ❌ | ⭐ | 无 |
| Webhook/API | ⚠️ | ⭐⭐⭐ | Go net/http 自行实现 |

**覆盖率：15%**。

---

### LangChainGo 综合矩阵

| 能力域 | 项数 | 完整 | 部分 | 不支持 | 覆盖率 | 成熟度 |
|--------|:----:|:----:|:----:|:------:|:------:|:------:|
| 模型连接层 | 14 | 4 | 3 | 7 | **50%** | ⭐⭐⭐ |
| Prompt 工程 | 5 | 2 | 1 | 2 | **45%** | ⭐⭐⭐ |
| Chain 编排 | 15 | 2 | 1 | 12 | **13%** | ⭐⭐ |
| Agent 核心 | 10 | 2 | 1 | 7 | **20%** | ⭐⭐ |
| 记忆管理 | 6 | 2 | 1 | 3 | **35%** | ⭐⭐⭐ |
| 向量存储与 RAG | 9 | 2 | 3 | 4 | **45%** | ⭐⭐⭐ |
| 工具系统 | 7 | 2 | 3 | 2 | **45%** | ⭐⭐⭐ |
| 数据规格校验 | 4 | 0 | 3 | 1 | **50%** | ⭐⭐⭐ |
| 安全与合规 | 5 | 0 | 1 | 4 | **10%** | ⭐ |
| 可观测性与运维 | 9 | 1 | 0 | 8 | **15%** | ⭐⭐ |
| 通道适配 | 3 | 0 | 1 | 2 | **15%** | ⭐⭐ |
| **总计** | **87** | **17** | **18** | **52** | **37%** | **⭐⭐⭐** |

---

## 五、四框架综合能力矩阵

| 能力域（项数） | InferGlow | AgentScope | Agently | LangChainGo |
|:---|:---:|:---:|:---:|:---:|
| **模型连接层**（14） | **82%** ⭐⭐⭐⭐ | 64% ⭐⭐⭐⭐ | 57% ⭐⭐⭐ | 50% ⭐⭐⭐ |
| **Prompt 工程**（5） | **75%** ⭐⭐⭐⭐ | 40% ⭐⭐⭐ | 40% ⭐⭐⭐ | 45% ⭐⭐⭐ |
| **Chain 编排**（15） | **63%** ⭐⭐⭐⭐ | 33% ⭐⭐⭐ | **60%** ⭐⭐⭐⭐ | 13% ⭐⭐ |
| **Agent 核心**（10） | **58%** ⭐⭐⭐⭐ | **65%** ⭐⭐⭐⭐ | 55% ⭐⭐⭐⭐ | 20% ⭐⭐ |
| **记忆管理**（6） | **67%** ⭐⭐⭐⭐ | **83%** ⭐⭐⭐⭐⭐ | 50% ⭐⭐⭐ | 35% ⭐⭐⭐ |
| **向量存储与 RAG**（9） | 0% ⭐ | **67%** ⭐⭐⭐⭐ | 0% ⭐ | 45% ⭐⭐⭐ |
| **工具系统**（7） | **86%** ⭐⭐⭐⭐⭐ | **86%** ⭐⭐⭐⭐⭐ | 75% ⭐⭐⭐⭐⭐ | 45% ⭐⭐⭐ |
| **数据规格校验**（4） | **100%** ⭐⭐⭐⭐⭐ | 75% ⭐⭐⭐⭐ | **100%** ⭐⭐⭐⭐⭐ | 50% ⭐⭐⭐ |
| **安全与合规**（5） | **80%** ⭐⭐⭐⭐⭐ | 40% ⭐⭐⭐ | 25% ⭐⭐⭐ | 10% ⭐ |
| **可观测性与运维**（9） | **40%** ⭐⭐⭐ | **55%** ⭐⭐⭐⭐ | 35% ⭐⭐⭐ | 15% ⭐⭐ |
| **通道适配**（3） | 15% ⭐⭐ | **35%** ⭐⭐⭐ | 35% ⭐⭐⭐ | 15% ⭐⭐ |
| **总覆盖率** | **68%** ⭐⭐⭐⭐ | **70%** ⭐⭐⭐⭐ | 50% ⭐⭐⭐⭐ | 37% ⭐⭐⭐ |

---

## 5.1 逐模块四框架逐项对比

> 以下 11 个模块对比表展示四框架在每一项 LangChain 规格指标上的具体实现差异。

### 模块 1：模型连接层（14 项）— 四框架对比

| 规格项 | InferGlow | AgentScope | Agently | LangChainGo |
|--------|:---------:|:----------:|:-------:|:-----------:|
| 供应商 ≥ 20 | ✅ **20+**（含 DeepSeek/Qwen/GLM/Kimi/SiliconFlow/OpenRouter 等国内全覆盖） | 9（OpenAI/Anthropic/DashScope/DeepSeek/Gemini/Moonshot/Ollama/xAI） | ~8（OpenAI/DeepSeek/Claude/Qwen/Ollama + OpenAI 兼容） | ~10（OpenAI/Anthropic/Google/Azure/Ollama 等） |
| 统一接口 | ✅ `ModelRequester` | ✅ `ChatModelBase` | ✅ `create_agent()` 统一 | ✅ `llms.LLM`/`ChatModel` |
| 路由/降级 | ✅ **6 策略** + FailoverModelRequester 冷却期+健康检查 | ✅ `fallback_model` 单降级 | ⚠️ 手动切换 Provider | ❌ 无 |
| Rate Limit | ✅ `RateLimitWrap` 429 + `ProviderLimiter` RPM/TPM/日 | ⚠️ 重试机制 | ❌ 无 | ⚠️ 部分 Provider 重试 |
| 流式输出 | ✅ `<-chan StreamChunk` | ✅ `stream=True` | ✅ 标准 stream | ✅ `Stream()`/`Chat()` |
| 批处理 | ⚠️ 无专用 API | ⚠️ 基础批量 | ⚠️ 基础批量 | ⚠️ `Batch()` 弱并发 |
| 函数/工具调用 | ✅ `ModelRequest.Tools` 全链路 | ✅ `tools` + `tool_choice` | ✅ Action Runtime 调度 | ✅ Function Calling |
| 多模态 | ⚠️ Attachment 字段 | ✅ TextBlock/DataBlock/ThinkingBlock | ⚠️ 依赖 Provider | ❌ 仅文本 |
| 结构化输出 | ✅ `OutputSchema`+`force_json` | ✅ `StructuredResponse`+jsonschema | ✅⭐ **Contract-First** | ⚠️ 基础 JSON |
| 异步 | ✅ goroutine+channel | ✅ Python async | ✅ async/await | ⚠️ 部分 Provider |
| Token 计数 | ⚠️ UsageInfo | ⚠️ Usage | ❌ 无 | ⚠️ 部分 Provider |
| 成本追踪 | ⚠️ Routing Cost 策略 | ❌ 无 | ❌ 无 | ❌ 无 |
| 预算控制 | ⚠️ LoopGuard Token 预算 | ✅ `ReplyBudgetControlMiddleware` | ❌ 无 | ❌ 无 |
| 全链路 async | ✅ goroutine | ✅ Python async | ✅ async/await | ❌ 不完整 |
| **覆盖率** | **79%** | **64%** | **57%** | **50%** |
| **胜出者** | **InferGlow** | | | |

**关键差异**：InferGlow 以 20+ Provider + 6 种路由策略领先；AgentScope 多模态+预算控制较好；Agently 结构化输出最强但供应商少；LangChainGo 基础但无路由/降级。

---

### 模块 2：Prompt 工程（5 项）— 四框架对比

| 规格项 | InferGlow | AgentScope | Agently | LangChainGo |
|--------|:---------:|:----------:|:-------:|:-----------:|
| Prompt Template | ✅ 四槽位 System/Developer/Instruct/Info | ✅ Jinja2 模板 | ✅ YAML/JSON 模板 | ✅ `PromptTemplate` 变量替换 |
| Chat Prompt | ✅ ChatMessage Role | ✅ system/user/assistant | ✅ input/instruct/info/output 四槽位 | ✅ `ChatPromptTemplate` |
| Few-shot | ✅ `ModelRequest.Examples` | ❌ 无 ExampleSelector | ❌ 无 | ❌ 无 |
| 模板组合 | ⚠️ Session+Flow 组合 | ⚠️ Middleware 组合 | ⚠️ Hierarchical Settings | ⚠️ Runnable 组合 |
| Prompt 版本 | ❌ 无 | ❌ 无 | ❌ 无 | ❌ 无 |
| **覆盖率** | **60%** | **40%** | **40%** | **45%** |
| **胜出者** | **InferGlow** | | | |

**关键差异**：InferGlow 的 Few-shot 支持是三区域 Session 的延伸；Agently 的四槽位设计独特但生态工具少；所有框架均无 Prompt 版本管理。

---

### 模块 3：Chain 编排（15 项）— 四框架对比

| 规格项 | InferGlow | AgentScope | Agently | LangChainGo |
|--------|:---------:|:----------:|:-------:|:-----------:|
| LLMChain | ✅ Engine.executeLoop | ✅ Agent.reply | ✅ `input().output().get_result()` | ✅ `chains.LLMChain` |
| SequentialChain | ✅ Linear Flow | ⚠️ Middleware 链 | ✅ `flow.to(A).to(B)` | ✅ `SequentialChain` |
| PipelineChain | ✅ TriggerFlow batch/for_each | ❌ 无 | ✅ `flow.batch()` Fan-out/Fan-in | ❌ 无 |
| StateGraph | ⚠️ SignalNet 事件驱动图 | ❌ 无 | ⚠️ when/emit 事件驱动 | ❌ 无 |
| 条件分支 | ✅ Branch.If + match/case | ❌ 无框架级 | ✅ if/elif/else + match/case | ❌ 无 |
| 子图/嵌套 | ✅ SubFlowHandler + SubFlowRegistry | ❌ 无 | ❌ 无 | ❌ 无 |
| 并行执行 | ✅ WorkerPool + batch_fanout | ❌ 无框架级 | ✅ `batch()` + `for_each(concurrency=N)` | ⚠️ goroutine 手动 |
| 中断/恢复 | ✅⭐ Pause/Resume + LifecycleMachine | ✅ UserInterruptEvent + RequireExternalExecution | ✅⭐ seal/unseal + pause/resume | ❌ 无 |
| Checkpointer | ✅ ExecutionSnapshot JSON/YAML | ⚠️ SQL/Redis Session 存储 | ✅⭐ `execution.save()`/`flow.load()` | ❌ 无 |
| 线程管理 | ⚠️ ExecutionID | ⚠️ Session ID | ⚠️ Execution handle | ❌ 无 |
| 断点续跑 | ✅⭐ Snapshot + Flow.Resume 跨进程 | ⚠️ 存储后端恢复 | ✅⭐ `save()`+`restored.load()` 跨进程 | ❌ 无 |
| 时间旅行 | ❌ 无 | ❌ 无 | ❌ 无 | ❌ 无 |
| HITL | ✅⭐ Pause + InterventionPointHandler | ✅⭐ RequireUserConfirm 一等公民 | ✅⭐ pause/resume + emit | ❌ 无 |
| Fault Tolerance | ✅ LifecycleMachine + panic 恢复 | ⚠️ 重试+降级 | ✅ auto_close + seal() 状态冻结 | ❌ 无 |
| 跨会话存储 | ⚠️ Session JSON/YAML | ✅ SQL/Redis/S3 三后端 | ⚠️ Session JSON/YAML | ❌ 无 |
| **覆盖率** | **60%** | **33%** | **60%** | **13%** |
| **胜出者** | **InferGlow = Agently** | | | |

**关键差异**：InferGlow 和 Agently 编排能力并列第一（60%），但路径不同——InferGlow 有 13 种算子+SubFlow 嵌套，Agently 有事件驱动+Blueprint 序列化。AgentScope 设计哲学不同（Middleware 驱动而非显式编排）。LangChainGo 编排能力极弱（13%）。

---

### 模块 4：Agent 核心（10 项）— 四框架对比

| 规格项 | InferGlow | AgentScope | Agently | LangChainGo |
|--------|:---------:|:----------:|:-------:|:-----------:|
| ReAct | ✅ Engine PLAN→EXECUTE | ✅ ReActConfig max_iters=20 | ✅ Action Runtime ReAct 循环 | ✅ `agents.ReAct` |
| Plan-and-Execute | ✅ TaskDAGStrategy + RepairLLMJSON | ✅ TaskCreate/Get/List/Update | ✅ TaskDAG + create_dynamic_task | ❌ 无 |
| Tool-Calling | ✅ ActionDispatcher | ✅ Toolkit 统一 | ✅ Action Runtime 三层 | ⚠️ Function Calling 驱动 |
| Zero-shot | ✅ systemPrompt | ✅ system_prompt | ✅ instruct 槽位 | ✅ `ZeroShotAgent` |
| Multi-Agent | ❌ 无 | ✅ **Team 工具 + SubAgentTemplate** | ❌ 无 | ❌ 无 |
| Sub-Agent | ⚠️ SubFlow 轻量 | ✅ SubAgentTemplate 动态派生 | ❌ 无 | ❌ 无 |
| Task Decomposition | ✅ TaskDAG 拓扑排序+依赖传递 | ✅ Task 工具 + InjectionConfig | ✅ create_dynamic_task | ❌ 无 |
| Self-Correction | ✅ RepairLLMJSON 三级修复 | ✅ structured_schema grace_iters=5 | ⚠️ success_criteria 校验 | ❌ 无 |
| Deep Agents | ❌ 无 | ❌ 无 | ⚠️ create_dynamic_task 轻量版 | ❌ 无 |
| **覆盖率** | **55%** | **65%** | **55%** | **20%** |
| **胜出者** | | **AgentScope** | | |

**关键差异**：AgentScope 的 Multi-Agent Team + SubAgentTemplate 是唯一支持动态多 Agent 协作的框架。InferGlow 和 Agently 在单 Agent 场景能力相当。LangChainGo Agent 能力最弱。

---

### 模块 5：记忆管理（6 项）— 四框架对比

| 规格项 | InferGlow | AgentScope | Agently | LangChainGo |
|--------|:---------:|:----------:|:-------:|:-----------:|
| BufferMemory | ✅ Session.FullContext | ✅ Agent 内建上下文 | ✅ Session 自动维护 | ✅ `ConversationBufferMemory` |
| BufferWindow | ✅ ContextWindow+MaxLength | ✅ 上下文窗口 | ✅ `session.max_length` | ✅ `WindowMemory` |
| SummaryMemory | ⚠️ SummaryReplace ResizeHandler | ✅ SummarySchema 结构化压缩 | ⚠️ 自定义 resize_handler | ❌ 无 |
| TokenBuffer | ⚠️ TokenAwareResizeHandler | ✅ trigger_ratio+reserve_ratio | ⚠️ Token 级窗口 | ❌ 无 |
| VectorStoreMemory | ❌ 无 | ✅ RAG Middleware+KnowledgeBase | ❌ 无 | ⚠️ 可通过 stores 实现 |
| 短/长期分离 | ✅ ThreeZoneSession+Memo | ✅⭐ **3 种长期记忆**（AgenticMemory/Mem0/ReMe） | ⚠️ Session 持久化 | ❌ 无 |
| **覆盖率** | **50%** | **83%** | **50%** | **35%** |
| **胜出者** | | **AgentScope** | | |

**关键差异**：AgentScope 的记忆系统是四框架中最强的——3 种长期记忆中间件 + 上下文自动压缩（SummarySchema）。InferGlow 的三区域 Session 在 prefix cache 优化上独特，但缺乏长期记忆和向量记忆。

---

### 模块 6：向量存储与 RAG（9 项）— 四框架对比

| 规格项 | InferGlow | AgentScope | Agently | LangChainGo |
|--------|:---------:|:----------:|:-------:|:-----------:|
| 文档加载器 | ❌ | ✅ 6 种（PDF/Word/Excel/PPT/Image/Text） | ❌ | ⚠️ ~10 种基础格式 |
| 文本分割器 | ❌ | ✅ ApproxTokenChunker | ❌ | ✅ RecursiveCharacter |
| Embedding | ❌ | ✅ 4 家（OpenAI/DashScope/Gemini/Ollama）+缓存 | ❌ | ✅ OpenAI/Google/Ollama |
| 向量数据库 | ❌ | ✅ 4 种（Milvus/Qdrant/ES/MongoDB） | ❌ | ✅ ~10 后端 |
| 检索策略 | ❌ | ✅ KnowledgeBase.search+metadata_filter | ❌ | ⚠️ 相似度搜索 |
| 重排序 | ❌ | ❌ | ❌ | ❌ |
| 多路召回 | ❌ | ❌ | ❌ | ❌ |
| 元数据过滤 | ❌ | ✅ metadata_filter 多租户 | ❌ | ⚠️ 部分后端 |
| **覆盖率** | **0%** | **67%** | **0%** | **45%** |
| **胜出者** | | **AgentScope** | | |

**关键差异**：RAG 是四框架分化最大的维度。AgentScope 有完整 RAG 管道（6 解析器+4 向量库+知识库管理）。LangChainGo 有基础 RAG。InferGlow 和 Agently 完全空白。

---

### 模块 7：工具系统（7 项）— 四框架对比

| 规格项 | InferGlow | AgentScope | Agently | LangChainGo |
|--------|:---------:|:----------:|:-------:|:-----------:|
| 自定义工具 | ✅ ActionRegistry 3 签名 | ✅ FunctionTool+ToolBase | ✅⭐ `@agent.action_func` 装饰器 | ✅ `tool` 包 |
| 描述自动生成 | ✅ schema_from_func | ✅⭐ docstring_parser | ✅ 函数签名/docstring | ⚠️ 基础 |
| 内置工具 | ✅ 8 个 | ✅⭐ 14 个（含 Task 4 工具+BashParser） | ⚠️ 少量（Search/Browse） | ⚠️ ~10 个 |
| 输出解析 | ✅ ActionResult 5 字段 | ✅ ToolResultBlock | ✅ ActionResult+model_digest | ⚠️ 基础 |
| 工具路由 | ✅ ActionDispatcher | ✅ Toolkit+ToolChoice | ✅ Action Runtime planning→dispatch | ✅ Agent 工具选择 |
| 沙箱执行 | ✅⭐ **8 后端** | ✅⭐ **8 后端** | ✅ 4 后端（Python/Bash/Docker/Node） | ❌ 无 |
| 工具组合 | ✅ ActionExtension+Dispatcher | ✅ Toolkit+ToolGroup | ✅ 三层架构编排 | ⚠️ 基础 |
| **覆盖率** | **86%** | **86%** | **75%** | **45%** |
| **胜出者** | **InferGlow = AgentScope** | | | |

**关键差异**：InferGlow 和 AgentScope 工具系统并列第一（86%），但侧重不同——InferGlow 沙箱跨平台（Linux/macOS/Windows/Cloud 8 后端），AgentScope 内置工具更多（14 个）+ MCP 3 种传输更完善。Agently 的 MCP+沙箱也强但沙箱后端少于前两者。

**MCP 对比**：AgentScope（STDIO/SSE/StreamableHTTP + 有状态/无状态双模式）> Agently（stdio+HTTP）> InferGlow（MCP tools/call 代理）> LangChainGo（❌）

---

### 模块 8：数据规格校验（4 项）— 四框架对比

| 规格项 | InferGlow | AgentScope | Agently | LangChainGo |
|--------|:---------:|:----------:|:-------:|:-----------:|
| JSON 模式输出 | ✅ ContractEngine+JSON Schema | ✅ StructuredResponse+jsonschema | ✅⭐ **Contract-First 核心卖点** | ⚠️ 基础 JSON 无强制校验 |
| Pydantic 响应校验 | ✅⭐ Go 泛型 `DefineOutput[T]()` | ✅ Pydantic BaseModel | ✅⭐ `ensure_keys`+`ensure_all_keys` | ❌ 无 Pydantic 等价 |
| 类型安全提取 | ✅ ExtractJSON 三级策略 | ✅ json_repair+容错 | ✅ Python 类型提示+运行时 | ⚠️ Go 静态类型但无自动化 |
| I/O Schema | ✅ OutputSchema+FieldDef 嵌套 | ✅ structured_schema | ✅ 四槽位 Schema+Settings | ⚠️ Struct 标签但无运行时校验 |
| **覆盖率** | **100%** | **75%** | **100%** | **50%** |
| **胜出者** | **InferGlow = Agently** | | | |

**关键差异**：InferGlow 和 Agently 并列满分。InferGlow 的 Go 泛型推导 `DefineOutput[T]()` 提供编译期类型安全，比 Agently 的 Python 运行时校验更严格。AgentScope 有 jsonschema 但无编译期保证。LangChainGo 最弱。

---

### 模块 9：安全与合规（5 项）— 四框架对比

| 规格项 | InferGlow | AgentScope | Agently | LangChainGo |
|--------|:---------:|:----------:|:-------:|:-----------:|
| Prompt 注入防护 | ✅ `prompt_injection/` 三级严重度+正则 | ❌ 无 | ❌ 无 | ❌ 无 |
| PII 脱敏 | ✅ `pii/` 正则+Input/Output 双向 | ❌ 无 | ❌ 无 | ❌ 无 |
| 审计日志 | ✅⭐ **链式哈希 SHA-256+HMAC+全链验证** | ✅ OTel+事件日志 | ✅ action_logs（input/output/timing） | ❌ 无 |
| 访问控制 | ✅ `rbac/` 4 角色+PermissionMatrix | ✅⭐ **5 种权限模式**+细粒度规则 | ❌ 无 | ❌ 无 |
| Secret 管理 | ✅ ModelRequester+ProviderLimiter | ✅ CredentialFactory+9 凭据 | ⚠️ Hierarchical Settings+env | ⚠️ 环境变量 |
| **覆盖率** | **80%** | **40%** | **25%** | **10%** |
| **胜出者** | **InferGlow** | | | |

**关键差异**：InferGlow 安全能力远超所有框架——是唯一实现 PII 脱敏 + Prompt 注入检测 + RBAC + 限流 + 链式审计的框架。AgentScope 的权限系统（5 种模式）设计精巧但缺 PII/注入检测。Agently 仅有 action_logs 审计。LangChainGo 完全空白。

---

### 模块 10：可观测性与运维（9 项）— 四框架对比

| 规格项 | InferGlow | AgentScope | Agently | LangChainGo |
|--------|:---------:|:----------:|:-------:|:-----------:|
| 分布式追踪 | ✅ 审计链+Engine OTel | ✅⭐ **完整 OTel GenAI 语义规范** | ✅ action_logs 全链路 | ❌ 无 |
| 评估框架 | ❌ | ❌ | ❌ | ❌ |
| A/B 测试 | ❌ | ❌ | ❌ | ❌ |
| 自动修复 | ✅ RepairLLMJSON | ❌ | ❌ | ❌ |
| 性能监控 | ✅ StepLog+AuditLog Duration | ✅ OTel 性能指标 | ⚠️ Action timing | ❌ 无 |
| REST API | ❌ | ✅⭐ **FastAPI 9 路由+多租户** | ⚠️ FastAPIHelper 基础 | ❌ 需自行封装 |
| Agent Server | ❌ | ✅⭐ **完整 Service+Web UI** | ❌ | ❌ |
| 容器化 | ✅ Go 二进制 | ✅ Python 标准 | ✅ Python 标准 | ✅ Go 二进制 |
| **覆盖率** | **35%** | **55%** | **35%** | **15%** |
| **胜出者** | | **AgentScope** | | |

**关键差异**：AgentScope 可观测性远超——完整 OTel GenAI + FastAPI 9 路由服务 + Web UI 是独特优势。InferGlow 和 Agently 在追踪/日志层面相当，但都缺 REST 托管。所有框架均无 Eval/A-B 测试。

---

### 模块 11：通道适配（3 项）— 四框架对比

| 规格项 | InferGlow | AgentScope | Agently | LangChainGo |
|--------|:---------:|:----------:|:-------:|:-----------:|
| 即时通讯 | ❌ | ❌ | ❌ | ❌ |
| 邮件 | ❌ | ❌ | ❌ | ❌ |
| Webhook/API | ⚠️ Go net/http 自行实现 | ✅⭐ FastAPI+WebSocket+ag-ui | ✅ FastAPI+Execution REST | ⚠️ Go net/http 自行实现 |
| **覆盖率** | **15%** | **35%** | **35%** | **15%** |
| **胜出者** | | **AgentScope = Agently** | | |

---

## 5.2 四框架 LangChain 规格 87 项逐项对照速查表

| # | 规格项 | InferGlow | AgentScope | Agently | LangChainGo |
|---|--------|:---------:|:----------:|:-------:|:-----------:|
| **模型连接层（14 项）** |
| 1 | 供应商 ≥ 20 | ✅ 20+ | 9 | ~8 | ~10 |
| 2 | 统一接口 | ✅ | ✅ | ✅ | ✅ |
| 3 | 路由/降级 | ✅ 6策略 | ✅ fallback | ⚠️ 手动 | ❌ |
| 4 | Rate Limit | ✅ | ⚠️ | ❌ | ⚠️ |
| 5 | 流式输出 | ✅ | ✅ | ✅ | ✅ |
| 6 | 批处理 | ⚠️ | ⚠️ | ⚠️ | ⚠️ |
| 7 | 函数/工具调用 | ✅ | ✅ | ✅ | ✅ |
| 8 | 多模态 | ⚠️ | ✅ | ⚠️ | ❌ |
| 9 | 结构化输出 | ✅ | ✅ | ✅⭐ | ⚠️ |
| 10 | 异步 | ✅ | ✅ | ✅ | ⚠️ |
| 11 | Token 计数 | ✅ | ⚠️ | ❌ | ⚠️ |
| 12 | 成本追踪 | ⚠️ | ❌ | ❌ | ❌ |
| 13 | 预算控制 | ⚠️ | ✅ | ❌ | ❌ |
| 14 | 全链路 async | ✅ | ✅ | ✅ | ❌ |
| **Prompt 工程（5 项）** |
| 15 | Prompt Template | ✅ | ✅ | ✅ | ✅ |
| 16 | Chat Prompt | ✅ | ✅ | ✅ | ✅ |
| 17 | Few-shot | ✅ | ❌ | ❌ | ❌ |
| 18 | 模板组合 | ✅ | ⚠️ | ⚠️ | ⚠️ |
| 19 | Prompt 版本 | ❌ | ❌ | ❌ | ❌ |
| **Chain 编排（15 项）** |
| 20 | LLMChain | ✅ | ✅ | ✅ | ✅ |
| 21 | SequentialChain | ✅ | ⚠️ | ✅ | ✅ |
| 22 | PipelineChain | ✅ | ❌ | ✅ | ❌ |
| 23 | StateGraph | ⚠️ | ❌ | ⚠️ | ❌ |
| 24 | 条件分支 | ✅ | ❌ | ✅ | ❌ |
| 25 | 子图/嵌套 | ✅ | ❌ | ❌ | ❌ |
| 26 | 并行执行 | ✅ | ❌ | ✅ | ⚠️ |
| 27 | 中断/恢复 | ✅ | ✅ | ✅ | ❌ |
| 28 | Checkpointer | ✅ | ⚠️ | ✅ | ❌ |
| 29 | 线程管理 | ⚠️ | ⚠️ | ⚠️ | ❌ |
| 30 | 断点续跑 | ✅ | ⚠️ | ✅ | ❌ |
| 31 | 时间旅行 | ❌ | ❌ | ❌ | ❌ |
| 32 | HITL | ✅ | ✅ | ✅ | ❌ |
| 33 | Fault Tolerance | ✅ | ⚠️ | ✅ | ❌ |
| 34 | 跨会话存储 | ⚠️ | ✅ | ⚠️ | ❌ |
| **Agent 核心（10 项）** |
| 35 | ReAct | ✅ | ✅ | ✅ | ✅ |
| 36 | Plan-and-Execute | ✅ | ✅ | ✅ | ❌ |
| 37 | Tool-Calling | ✅ | ✅ | ✅ | ⚠️ |
| 38 | Zero-shot | ✅ | ✅ | ✅ | ✅ |
| 39 | Multi-Agent | ❌ | ✅ | ❌ | ❌ |
| 40 | Sub-Agent | ⚠️ | ✅ | ❌ | ❌ |
| 41 | Task Decomposition | ✅ | ✅ | ✅ | ❌ |
| 42 | Self-Correction | ✅ | ✅ | ⚠️ | ❌ |
| 43 | Deep Agents | ❌ | ❌ | ⚠️ | ❌ |
| **记忆管理（6 项）** |
| 44 | BufferMemory | ✅ | ✅ | ✅ | ✅ |
| 45 | BufferWindow | ✅ | ✅ | ✅ | ✅ |
| 46 | SummaryMemory | ✅ | ✅ | ⚠️ | ❌ |
| 47 | TokenBuffer | ✅ | ✅ | ⚠️ | ❌ |
| 48 | VectorStoreMemory | ❌ | ✅ | ❌ | ⚠️ |
| 49 | 短/长期分离 | ✅ | ✅ | ⚠️ | ❌ |
| **RAG（9 项）** |
| 50 | 文档加载器 | ❌ | ✅ | ❌ | ⚠️ |
| 51 | 文本分割器 | ❌ | ✅ | ❌ | ✅ |
| 52 | Embedding | ❌ | ✅ | ❌ | ✅ |
| 53 | 向量数据库 | ❌ | ✅ | ❌ | ✅ |
| 54 | 检索策略 | ❌ | ✅ | ❌ | ⚠️ |
| 55 | 重排序 | ❌ | ❌ | ❌ | ❌ |
| 56 | 多路召回 | ❌ | ❌ | ❌ | ❌ |
| 57 | 元数据过滤 | ❌ | ✅ | ❌ | ⚠️ |
| **工具系统（7 项）** |
| 58 | 自定义工具 | ✅ | ✅ | ✅ | ✅ |
| 59 | 描述自动生成 | ✅ | ✅ | ✅ | ⚠️ |
| 60 | 内置工具 | ✅ 8个 | ✅ 14个 | ⚠️ 少量 | ⚠️ ~10个 |
| 61 | 输出解析 | ✅ | ✅ | ✅ | ⚠️ |
| 62 | 工具路由 | ✅ | ✅ | ✅ | ✅ |
| 63 | 沙箱执行 | ✅ 8后端 | ✅ 8后端 | ✅ 4后端 | ❌ |
| 64 | 工具组合 | ✅ | ✅ | ✅ | ⚠️ |
| **数据校验（4 项）** |
| 65 | JSON 模式 | ✅ | ✅ | ✅ | ⚠️ |
| 66 | 响应校验 | ✅ 泛型 | ✅ Pydantic | ✅ ensure_keys | ❌ |
| 67 | 类型安全提取 | ✅ 三级策略 | ✅ json_repair | ✅ 运行时 | ⚠️ |
| 68 | I/O Schema | ✅ | ✅ | ✅ | ⚠️ |
| **安全与合规（5 项）** |
| 69 | Prompt 注入防护 | ✅ | ❌ | ❌ | ❌ |
| 70 | PII 脱敏 | ✅ | ❌ | ❌ | ❌ |
| 71 | 审计日志 | ✅ 链式哈希 | ✅ OTel | ✅ action_logs | ❌ |
| 72 | 访问控制 | ✅ RBAC | ✅ 5模式 | ❌ | ❌ |
| 73 | Secret 管理 | ✅ | ✅ | ⚠️ | ⚠️ |
| **可观测性（9 项）** |
| 74 | 分布式追踪 | ✅ | ✅ OTel | ✅ | ❌ |
| 75 | 评估框架 | ❌ | ❌ | ❌ | ❌ |
| 76 | A/B 测试 | ❌ | ❌ | ❌ | ❌ |
| 77 | 自动修复 | ✅ | ❌ | ❌ | ❌ |
| 78 | 性能监控 | ✅ | ✅ | ⚠️ | ❌ |
| 79 | REST API | ❌ | ✅ FastAPI | ⚠️ | ❌ |
| 80 | Agent Server | ❌ | ✅ Service+WebUI | ❌ | ❌ |
| 81 | 容器化 | ✅ | ✅ | ✅ | ✅ |
| **通道适配（3 项）** |
| 82 | 即时通讯 | ❌ | ❌ | ❌ | ❌ |
| 83 | 邮件 | ❌ | ❌ | ❌ | ❌ |
| 84 | Webhook/API | ⚠️ | ✅ | ✅ | ⚠️ |
| **总计** | | **68%** | **70%** | **50%** | **37%** |

> 注：✅ = 完整支持，⚠️ = 部分支持，❌ = 不支持。覆盖率 = (完整×1.0 + 部分×0.5) / 项数。

---

## 六、维度雷达图对照

| 维度 | InferGlow | AgentScope | Agently | LangChainGo |
|------|:---------:|:----------:|:-------:|:-----------:|
| **模型生态** | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐ |
| **编排能力** | ⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐ |
| **工具系统** | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ |
| **RAG 能力** | ⭐ | ⭐⭐⭐⭐ | ⭐ | ⭐⭐⭐ |
| **安全能力** | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐ | ⭐ |
| **可观测性** | ⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐ |
| **记忆管理** | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐ |
| **数据校验** | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ |
| **沙箱支持** | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐ |
| **服务化** | ⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐ |
| **部署便利性** | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐⭐⭐ |
| **Multi-Agent** | ⭐ | ⭐⭐⭐⭐ | ⭐ | ⭐ |

---

## 七、差异化竞争力分析

### InferGlow 独有/领先能力

| 能力 | 说明 | 其他框架状态 |
|------|------|-------------|
| **6 种模型路由策略** | First/Random/Cost/Latency/Quality/Fallback + 自动 failover | AgentScope 仅 fallback_model |
| **链式哈希审计日志** | SHA-256 链式 + HMAC 签名 + 全链验证 + JSONL 轮转 | 其他框架无等价能力 |
| **PII + 注入 + RBAC + 限流** | 四层安全纵深防御 | AgentScope 仅有权限系统 |
| **reasoning_content** | DeepSeek/MiMo/Ollama 推理内容分离 | AgentScope 有 ThinkingBlock |
| **Prefix Cache 感知** | 7 Provider 缓存能力画像 + 三区域 Session | 其他框架无 |
| **Middleware + Callbacks** | Agent 中间件链 + 6 钩子生命周期回调 + CallbacksTracer OTel 桥接 | AgentScope 有 Middleware 但无 CallbacksTracer |
| **Go 单二进制** | ~15MB 无依赖部署 | Python 框架无法比拟 |

### AgentScope 独有/领先能力

| 能力 | 说明 | 其他框架状态 |
|------|------|-------------|
| **完整 RAG 管道** | 6 解析器 + 4 向量库 + 知识库管理 + 多租户 | InferGlow/Agently 完全无 |
| **3 种长期记忆** | AgenticMemory + Mem0 + ReMe | 其他框架无等价 |
| **OTel GenAI 规范** | 完整 OpenTelemetry 语义属性集成 | InferGlow 部分 OTel |
| **FastAPI 多租户服务** | 9 路由 + Team + 调度 + 消息总线 + Web UI | InferGlow/Agently 无 REST |
| **28 种事件类型** | 完整流式事件协议（Start/Delta/End） | InferGlow 基础事件 |
| **Multi-Agent Team** | Team 工具 + SubAgentTemplate + 协作对话 | InferGlow 无 |
| **MCP 3 种传输** | STDIO + SSE + StreamableHTTP + 有状态/无状态 | InferGlow MCP 基础 |
| **TTS 语音合成** | 3 Provider 5 模型 | 其他框架无 |

### 关键差异总结

| 对比维度 | InferGlow 胜出 | AgentScope 胜出 |
|----------|:--------------:|:---------------:|
| 模型供应商数量 | ✅ 20+ vs 9 | |
| 模型路由策略 | ✅ 6 种 vs fallback | |
| 安全纵深 | ✅ PII+注入+RBAC+限流+审计 | |
| **Chain 编排** | ✅ 63% vs 33% | |
| 数据校验 | ✅ 100% vs 75% | |
| RAG | | ✅ 67% vs 0% |
| 记忆管理 | | ✅ 83% vs 67% |
| 可观测性 | | ✅ OTel + REST |
| Multi-Agent | | ✅ Team vs 无 |
| 服务化部署 | | ✅ FastAPI vs 无 |
| Go 部署 | ✅ 单二进制 | |

---

## 八、适用场景推荐

| 场景 | InferGlow | AgentScope | Agently | LangChainGo |
|------|:---------:|:----------:|:-------:|:-----------:|
| Go 技术栈 + 安全敏感 | ⭐⭐⭐⭐⭐ | — | — | ⭐ |
| Go 技术栈 + 沙箱隔离 | ⭐⭐⭐⭐⭐ | — | — | ⭐ |
| Go 技术栈 + 结构化输出 | ⭐⭐⭐⭐⭐ | — | — | ⭐⭐⭐ |
| Python + RAG 系统 | — | ⭐⭐⭐⭐⭐ | ⭐ | ⭐⭐⭐ |
| Python + Multi-Agent | — | ⭐⭐⭐⭐⭐ | ⭐ | ⭐ |
| Python + 长期记忆 | — | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐ |
| Python + 服务化部署 | — | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐ |
| 审批门控/长流程 | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐ |
| MCP 集成 | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐ |
| 快速原型 | ⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐⭐ |
| 企业级运维 | ⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐ |

---

## 九、InferGlow 改进建议（更新）

### P0 必须

| 改进项 | 说明 | 预期提升 |
|--------|------|---------|
| **REST API 服务层** | AgentScope 已有完整 FastAPI 服务，InferGlow 尚无。需至少提供 Agent HTTP API + Session 管理 | 可观测性 40%→55% |
| **Multi-Agent 协作** | AgentScope 的 Team 工具 + SubAgentTemplate 是差异化能力。InferGlow 需支持动态 Agent 路由 | Agent 核心 58%→68% |

### P1 重要

| 改进项 | 说明 | 预期提升 |
|--------|------|---------|
| **OTel GenAI 完整集成** | AgentScope 已完整实现 OTel GenAI 语义规范，InferGlow 已有 CallbacksTracer 基础，需补全语义属性 | 可观测性 +10% |
| **VectorStoreMemory** | 已有 Memory 接口 + SummaryMemory/TokenBufferMemory，需补向量检索记忆层 | 记忆管理 67%→83% |
| **RAG 接口层** | 至少提供 Embedding + VectorStore 接口 | RAG 0%→30% |

### P2 可选

| 改进项 | 说明 |
|--------|------|
| **TTS 集成** | AgentScope 有 3 Provider TTS，InferGlow 可考虑 |
| **权限模式系统** | AgentScope 5 种权限模式设计值得参考 |
| **事件协议标准化** | AgentScope 28 种事件类型 + Start/Delta/End 流式模式 |

---

## 十、总结

### 四框架定位一句话

| 框架 | 定位 |
|------|------|
| **InferGlow** | **Go 生态安全最强的 Agent 基础设施**——20+ Provider + 四层安全纵深 + 8 沙箱 + 链式审计 + Callbacks/Middleware |
| **AgentScope** | **Python 生产级全栈 Agent 平台**——RAG + 长期记忆 + OTel + FastAPI 多租户服务 + Multi-Agent Team |
| **Agently** | **Python 工程化交付框架**——Contract-First 数据校验 + TriggerFlow 编排 + Action Runtime 工具 |
| **LangChainGo** | **Go 基础 LangChain 替代**——基础 Chain/Memory/RAG 能力，适合中等复杂度 |

### 核心洞察

1. **InferGlow v4 从 65% 提升至 68%**，最大变化在记忆管理（50%→67%）、Prompt 工程（60%→75%）、可观测性（35%→40%）
2. **AgentScope 以 70% 仍为四框架中覆盖率最高的**，但 InferGlow 差距缩小至 2%（v3 时为 5%）
3. **InferGlow 在安全与合规（80%）上超越所有框架**——PII + 注入检测 + RBAC + 限流 + 链式审计
4. **InferGlow 记忆管理从 50% 提升至 67%**——SummaryMemory + TokenBufferMemory 独立实现，与 AgentScope 差距缩小
5. **InferGlow 的最大短板仍是 RAG（0%）和服务化（无 REST API）**
6. **Go 选 InferGlow，Python 选 AgentScope**——两者在各自生态中都是最强选择
7. **InferGlow 的 Go 单二进制 + 无 Python 依赖**在边缘部署、嵌入式 Agent 场景有不可替代的优势
