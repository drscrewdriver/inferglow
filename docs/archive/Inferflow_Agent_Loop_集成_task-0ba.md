# Inferflow Agent Loop 集成

## 背景

上游 inferglow 的 `flow.FlowContext` 接口新增了 `RunAgent` 和 `RunAgentParallel` 两个方法（多范式共存架构），导致 inferflow 编译中断（`runtime/engine/engine.go` L569, L741）。当前 inferflow 的 12 个 stage 全部为单次 LLM 调用（single-shot），coder→write_files→reviewer 的重试靠 YAML 硬编码 2 轮重复步骤实现。

**目标**：修复编译 + 让 coder/reviewer stage 可选使用多轮 Agent 循环（读取代码→生成→验证→修复），替换硬编码重试。

## 实施步骤

### Task 1: 上游 — agent.Engine 添加公开 RunLoop 方法

**文件**: `inferglow/orchestrator/agent/engine.go`（L107 后）

新增公开方法，包装私有 `executeLoop`，只返回 `FinalResponse` 字符串（不暴露 `actionruntime.Decision` 类型给外部包）：

```go
func (e *Engine) RunLoop(ctx context.Context, userMessage string, maxRounds int, systemPrompt string) (string, error) {
    decision, err := e.executeLoop(ctx, userMessage, maxRounds, systemPrompt)
    if err != nil { return "", err }
    if decision == nil { return "", fmt.Errorf("agent: RunLoop returned nil decision") }
    return decision.FinalResponse, nil
}
```

- 向后兼容：仅新增，不修改任何现有方法
- 命名选择 `RunLoop` 而非 `Run`：避免与未来可能的 `Agent.Run` 入口混淆，且语义更清晰

**同步更新** `flow_context_impl.go` L261：`fc.engine.executeLoop(...)` → `fc.engine.RunLoop(...)`，保持内部一致。

### Task 2: 修复编译 — inferflow flowContext 实现 RunAgent/RunAgentParallel

**文件**: `inferflow/runtime/engine/engine.go`

(a) `flowContext` 结构体（L215-224）新增字段：
```go
agentEngine *agent.Engine  // nil 时 RunAgent 返回 ErrAgentNotConfigured
```

(b) `newFlowContext`（L227-242）新增参数 `agentEngine *agent.Engine`，赋值给 `fc.agentEngine`。

(c) L340 后新增两个方法：
```go
func (fc *flowContext) RunAgent(ctx context.Context, userMessage string, systemPrompt string, opts *flow.AgentRunOptions) (string, error) {
    if fc.agentEngine == nil { return "", flow.ErrAgentNotConfigured }
    maxRounds := 10
    if opts != nil && opts.MaxRounds > 0 { maxRounds = opts.MaxRounds }
    return fc.agentEngine.RunLoop(ctx, userMessage, maxRounds, systemPrompt)
}

func (fc *flowContext) RunAgentParallel(ctx context.Context, agents []flow.AgentSubTask) ([]string, error) {
    if fc.agentEngine == nil { return nil, flow.ErrAgentNotConfigured }
    results := make([]string, len(agents))
    for i, sub := range agents {
        r, err := fc.RunAgent(ctx, sub.UserMessage, sub.SystemPrompt, &flow.AgentRunOptions{MaxRounds: sub.MaxRounds, SessionIsolation: true})
        if err != nil { return nil, fmt.Errorf("parallel agent %q (index %d): %w", sub.Label, i, err) }
        results[i] = r
    }
    return results, nil
}
```

(d) `runFlow`（L568）和 `Resume`（L740）中构建 flowContext 时，创建并注入 `agent.Engine`：
```go
agentEng := agent.NewEngine(sessExt, actExt, e.modelReq)
fc := newFlowContext(sess, actExt, e.modelReq, ..., agentEng)
```

**关键**：`agent.NewEngine` 构造成本极低（赋值 + 创建轻量结构体，无 I/O）。`sessExt` 已在 L556-558 创建。

### Task 3: 修复测试 mock

**文件**: `inferflow/runtime/stage/builtin/mock_fctx_test.go`

`mockFctx` 结构体（L34-56）后新增两个桩方法：
```go
func (m *mockFctx) RunAgent(_ context.Context, _, _ string, _ *flow.AgentRunOptions) (string, error) {
    if m.AgentResp != "" || m.AgentErr != nil { return m.AgentResp, m.AgentErr }
    return "", flow.ErrAgentNotConfigured
}
func (m *mockFctx) RunAgentParallel(_ context.Context, _ []flow.AgentSubTask) ([]string, error) {
    return nil, flow.ErrAgentNotConfigured
}
```

`mockFctx` 结构体新增字段：`AgentResp string`, `AgentErr error`。

### Task 4: Coder stage 支持 Agent 模式

**文件**: `inferflow/runtime/stage/builtin/coder.go`

(a) 新增 `coderAgentSystemPrompt` 常量，指导 Agent 使用工具自主迭代：
- 用 `file_read` / `grep_search` 探索代码库
- 用 `file_write` 写入修改
- 用 `bash_executor` 验证编译（`go build ./...`）
- 完成后返回 JSON `{"files": {...}, "summary": "..."}`

(b) `Coder` 函数增加分支（L63 后）：
```go
var resp string
var err error
if useAgent, _ := in["_agent"].(bool); useAgent {
    agentPrompt := resolveSystemPrompt(in, coderAgentSystemPrompt)
    maxRounds := 5
    if mr, ok := in["_agent_max_rounds"].(int); ok && mr > 0 { maxRounds = mr }
    resp, err = fctx.RunAgent(ctx, taskDescription, agentPrompt, &flow.AgentRunOptions{
        MaxRounds: maxRounds, SessionIsolation: true,
    })
    if errors.Is(err, flow.ErrAgentNotConfigured) {
        resp, err = fctx.GenerateModel(ctx, sysPrompt, taskDescription)  // 自动降级
    }
} else {
    resp, err = fctx.GenerateModel(ctx, sysPrompt, taskDescription)
}
```

**关键**：`ErrAgentNotConfigured` 时自动降级为 single-shot，确保向后兼容。

### Task 5: Reviewer stage 支持 Agent 模式

**文件**: `inferflow/runtime/stage/builtin/reviewer.go`

(a) 新增 `reviewerAgentSystemPrompt`，指导 Agent：
- 用 `file_read` 读取实际生成的文件
- 用 `bash_executor` 运行测试（`go test ./...`）
- 用 `grep_search` 检查模式
- 输出结构化审查结果 `{"passed": bool, "comments": "...", "issues": [...]}`

(b) `Reviewer` 函数同样增加 `_agent` 分支 + `ErrAgentNotConfigured` 降级。

### Task 6: 简化 YAML 流程

**文件**: `inferflow/etc/config/flows/bug_fix_workflow.yaml`

将 6 个硬编码重试步骤（L63-115: retry_coder/retry_write/retry_review × 2）替换为 agent 模式：

```yaml
# Phase 2: Code generation (agent mode — auto-iterates internally)
- name: locate
  operator: stage
  stage: coder
  depends_on: [analyze]
  inputs:
    _agent: true
    _agent_max_rounds: 5

- name: write_files
  operator: stage
  stage: write_files
  depends_on: [locate]

# Phase 3: Review (agent mode — can run tests, read files)
- name: review
  operator: stage
  stage: reviewer
  depends_on: [write_files]
  inputs:
    _agent: true
    _agent_max_rounds: 3
```

13 步 → 7 步，移除所有 `when: "{{not .passed}}"` 守卫。`quality_gate` + `commit_push` 保留。

### Task 7: 测试

- **`coder_test.go`**: 新增 `TestCoder_AgentMode_UsesRunAgent`（设置 `_agent: true`，验证调用 `RunAgent` 而非 `GenerateModel`）
- **`coder_test.go`**: 新增 `TestCoder_AgentMode_FallbackOnErrNotConfigured`（RunAgent 返回 ErrAgentNotConfigured 时降级）
- **`reviewer_test.go`**: 类似两个测试
- **`engine_test.go`**: 验证 flowContext.RunAgent 委托给 agentEngine

## 依赖关系

```
Task 1 (上游 RunLoop) ──→ Task 2 (flowContext 实现) ──→ Task 3 (mock 修复)
                                                       ├──→ Task 4 (coder)
                                                       ├──→ Task 5 (reviewer)
                                                       └──→ Task 7 (测试)
                                                              ↓
                                                         Task 6 (YAML 简化)
```

Task 1→2→3 是编译修复链（MUST），Task 4-7 是功能增强（MUST 但依赖 1-3）。

## 文件变更清单

| 文件 | 操作 | 行数 |
|------|------|------|
| `inferglow/orchestrator/agent/engine.go` | 修改: 新增 RunLoop 方法 | +10 |
| `inferglow/orchestrator/agent/flow_context_impl.go` | 修改: RunAgent 改用 RunLoop | ~2 |
| `inferflow/runtime/engine/engine.go` | 修改: flowContext 新增字段+方法+注入 | +50 |
| `inferflow/runtime/stage/builtin/mock_fctx_test.go` | 修改: mock 新增两个方法+字段 | +15 |
| `inferflow/runtime/stage/builtin/coder.go` | 修改: agent 模式分支+新 system prompt | +60 |
| `inferflow/runtime/stage/builtin/reviewer.go` | 修改: agent 模式分支+新 system prompt | +50 |
| `inferflow/etc/config/flows/bug_fix_workflow.yaml` | 修改: 13步→7步 | -55/+15 |
| `inferflow/runtime/stage/builtin/coder_test.go` | 修改: 新增 agent 模式测试 | +40 |
| `inferflow/runtime/stage/builtin/reviewer_test.go` | 修改: 新增 agent 模式测试 | +40 |
| **总计** | | **~230** |

## 风险评估

| 风险 | 严重度 | 缓解 |
|------|--------|------|
| Session 污染：agent 循环消息混入 flow session | 中 | coder/reviewer 使用 `SessionIsolation: true`，Phase 2 实现 session fork |
| Token 消耗失控 | 低 | maxRounds 硬限制（coder:5, reviewer:3）+ LoopGuard 检测重复 |
| 工具副作用（bash/file_write 修改文件系统） | 中 | file_write 限制 AllowedDirs，bash_executor 受 isolation context 约束 |
| ErrAgentNotConfigured 降级保证 | 低 | coder/reviewer 显式捕获错误并降级为 GenerateModel |
| 上游 API `RunLoop` 的稳定性承诺 | 低 | 方法简单，签名稳定，是 executeLoop 的直接包装 |

## 被否决的方案

| 方案 | 否决原因 |
|------|---------|
| 仅 stub 不做真实实现 | 用户明确要求引入多轮迭代能力，stub 无法满足 |
| 在 inferflow 中重写 agent 循环 | 过度重复，上游已有完整的 LoopGuard/Cancel/L3-L4 逻辑 |
| 使用反射调用 executeLoop | 不安全，破坏类型检查，且公开方法更干净 |
| RunAgentParallel 返回 error 而非顺序降级 | 上游已采用顺序降级模式，保持一致性 |
| YAML 中保留重试步骤作为备选 | agent 模式的内部迭代更灵活，硬编码重试是反模式 |
