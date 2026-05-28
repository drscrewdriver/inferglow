# 08 · 关键调用链

> 本文档汇总跨模块的端到端函数调用链，每条链路标注**源码文件:行号**，便于跳转核对。
> 所有路径相对于仓库根目录 `inferglow/`。截至 2026-07-22 源码状态。

## 一、调用链索引

| 编号 | 名称 | 入口 | 触发场景 |
|:----:|------|------|---------|
| C1 | 用户请求端到端主链 | `Agent.Run` | 一次完整的 PLAN→EXECUTE 循环 |
| C2 | 单轮 LLM 规划子链 | `Engine.executeLoop` L163-L475 | 每轮 PLAN 阶段 |
| C3 | Action 并发执行子链 | `ActionDispatcher.Execute` | 每轮 EXECUTE 阶段 |
| C4 | MCP 工具发现注册链 | `DiscoverMCPTools` | 启动期一次性 |
| C5 | MCP 工具调用链 | `MCPExecutor.Execute` | 运行期 Action 调用 |
| C6 | 审计链写入链 | `AuditChain.Append` | 每次决策/Action 后 |
| C7 | 审计链验证链 | `AuditChain.VerifyChain` | 离线取证/启动校验 |
| C8 | PII 输入侧脱敏链 | `Session.AddMessageChecked` | 每次 AddMessage |
| C9 | PII 输出侧脱敏链 | `Agent.Run` 输出阶段 | 最终响应返回前 |
| C10 | 死循环检测链 | `LoopGuard.Check` | 每轮循环开头 |
| C11 | 取消/抢占链 | `CancelManager.Cancel` | 外部请求取消 |
| C12 | 会话 resize 链 | `Session.PreparePrompt` | 上下文超长时 |
| C13 | Sandbox 执行审批链 | `ApprovalService.Submit` | 高危 Action 执行前 |

---

## 二、C1 · 用户请求端到端主链

最关键的链路：用户输入一条消息，最终拿到 Agent 的最终响应。

```
[用户]
  │  Agent.Run(ctx, userMessage, opts)
  ▼
[orchestrator/agent/agent.go:240] Agent.Run
  │
  ├──[L247-254] 合并 runConfig（Agent 默认 + per-call opts）
  │       maxRounds / systemPrompt / streamTimeout / outputHook / piiMasker / outputSchema
  │
  ├──[L268] a.engine.streamTimeout = c.streamTimeout  ← 透传流超时
  ├──[L272] a.engine.outputSchema = c.outputSchema    ← 透传 L4 schema (NEW)
  │
  ├──[L?] if piiMasker != nil:
  │       a.session.SetMessageMasker(piiMasker)
  │         → [session/session.go:245] Session.SetMessageMasker
  │         → 后续所有 AddMessage 都会先 MaskInput
  │
  ├──[L184] decision, err := a.engine.executeLoop(...)
  │         ↓ 进入 C2 主循环
  │
  ├──[L185-187] if err != nil: return "", err
  │
  ├──[L189] if decision.NextAction == "response":
  │   │
  │   ├──[L196-200] if outputHook != nil:
  │   │       outputHook.CheckOutput(decision.FinalResponse)
  │   │         → prompt_injection.Detector 检测
  │   │         → 返回 error 时阻断响应（不写入 session）
  │   │
  │   ├──[L201] response := decision.FinalResponse
  │   │
  │   ├──[L206-208] if piiMasker != nil:
  │   │       response = piiMasker.MaskOutput(response)
  │   │         → [security/pii/mask.go:134] Masker.MaskOutput
  │   │         → 进入 C9 输出侧脱敏链
  │   │
  │   ├──[L209] a.session.AddAssistantMessage(response)
  │   │         → [orchestrator/agent/session_ext.go:53] SessionExtension.AddAssistantMessage
  │   │         → [session/session.go:194] Session.AddMessage
  │   │         → 触发 masker.MaskInput（若设置）
  │   │
  │   └──[L210] return response, nil  ← 主链终点
  │
  └──[L214] return "", ErrNoFinalResponse
```

---

## 三、C2 · 单轮 LLM 规划子链（executeLoop 核心）

[orchestrator/agent/engine.go:163] `Engine.executeLoop` 是整个框架的「心脏」。

```
[engine.go:163] executeLoop(ctx, userMessage, maxRounds, systemPrompt)
  │
  ├──[L165] e.session.AddUserMessage(userMessage)
  │         → SessionExtension.AddUserMessage → Session.AddMessage
  │         → 触发 PII MaskInput（C8）
  │
  ├──[L174] defer: 确保 TurnLoop 回到 Idle
  │
  ├──[L180] round := 0
  ├──[L185] for {
        │
        ├──[L187-210] ── LoopGuard 检查 ──→ C10
        │     if e.loopGuard != nil:
        │       verdict, _ := e.loopGuard.Check(state)
        │       switch verdict.Action:
        │         VerdictBreak   → return ErrLoopDetected
        │         VerdictDegrade → systemPrompt += degrade 提示
        │         VerdictContinue → 继续
        │
        ├──[L218-225] ── CancelManager 立即取消检查 ──→ C11
        │     CheckTimeoutEscalation()
        │     if HasPendingCancel && CheckCancel(CancelImmediate):
        │       CompleteCancel(nil); return "agent cancelled"
        │
        ├──[L231] ── TurnLoop 进入 Planning ──
        │     preemptCh = e.turnLoop.EnterPlanning()
        │
        ├──[L235] tools := e.buildToolDefinitions()
        │
        ├──[L235-275] 构造 model.ModelRequest
        │     System / ChatHistory=session.PreparePrompt() / Tools / Output
        │     Options: {"force_json": true}
        │
        ├──[L277] ── L3 兜底 prompt 注入 (NEW) ──
        │     if e.outputSchema != nil && shouldInjectSchemaPrompt(req):
        │       req.System += formatSchemaInstruction(req.Output)
        │     ← 仅当 provider 不支持 json_schema 级 response_format 时启用
        │
        ├──[L282] data, err := e.modelReq.GenerateRequestData(ctx, req)
        │         → Provider.GenerateRequestData
        │         → 当 force_json:true + Properties 非空时发 json_schema 模式 (L1/L2)
        │
        ├──[L297] timeoutCtx 默认 5min
        ├──[L298] stream, err := e.modelReq.RequestModel(timeoutCtx, data)
        │
        ├──[L302-335] streamLoop: 收集 content
        │     select {
        │       case chunk := <-stream: content.WriteString(chunk.Delta)
        │       case <-timeoutCtx.Done(): return timeoutCtx.Err()
        │       case <-preemptCh: return "agent preempted"
        │     }
        │
        ├──[L340-375] ── L4 后置校验 + 重试 (NEW) ──
        │     if e.outputSchema != nil:
        │       validator := model.NewOutputValidator(e.outputSchema)
        │       validator.MaxRetries = 2
        │       validatedResp, err := validator.ValidateAndRetryWithFetch(ctx, fetcher)
        │       ← fetcher 首次返回已收集 content，重试时重新调用模型
        │       ← 失败 → return "L4 output validation failed after retries"
        │       ← 成功 → content 替换为 validatedResp.Content
        │
        ├──[L379] totalTokens += len(content.String())
        │
        ├──[L382] decision, err := actionruntime.ParseDecision(content.String())
        │         → ParseDecision → RepairLLMJSON → json.Unmarshal → Decision
        │
        ├──[L383-395] ParseDecision 失败降级策略:
        │     decision = {NextAction:"response", FinalResponse:rawContent}
        │
        ├──[L400-415] ── 审计链写入（仅 IsEnabled 时）──→ C6
        │
        ├──[L417] prevDecision = decision; prevOutput = content.String()
        │
        ├──[L419] if !actionruntime.ShouldContinue(*decision, round, maxRounds):
        │           return decision, nil
        │
        ├──[L429] ── CancelAfterChatModel 安全点 ──→ C11
        │
        ├──[L439] TurnLoop.EnterActive()
        │
        ├──[L443-445] dispatcher := NewActionDispatcherWithAudit(registry, auditHook)
        │             results := dispatcher.Execute(ctx, decision.ActionCalls)
        │             ↓ 进入 C3
        │
        ├──[L449-453] for i, call := range decision.ActionCalls:
        │       e.session.AddActionResult(call.Name, results[i])
        │
        ├──[L461] ── CancelAfterToolCalls 安全点 ──→ C11
        │
        ├──[L470] TurnLoop.EnterIdle()
        │
        └──[L473] round++
      }
```

---

## 四、C3 · Action 并发执行子链

[orchestrator/actionruntime/dispatcher.go:58] `ActionDispatcher.Execute`

```
[dispatcher.go:58] Execute(ctx, calls []ActionCall) []*ActionResult
  │
  ├── results := make([]*ActionResult, len(calls))
  ├── var wg sync.WaitGroup
  │
  ├── for i, call := range calls:
  │     wg.Add(1)
  │     go func(idx, c) {
  │       defer wg.Done()
  │       defer recover()  ← panic 恢复：合成 error 形态的 ActionResult
  │                          避免单 Action 崩溃拖垮整个 dispatcher
  │
  │       result, err := registry.Execute(ctx, c.Name, c.Params)
  │         → [action/action.go:152] ActionRegistry.Execute
  │            ├──[action.go:125] Get(name) ← 查表
  │            │     未找到 → return error（不会进 executor）
  │            └──[action.go:60] ActionExecutor.Execute(ctx, input)
  │                  三种实现 ↓
  │
  │       if err != nil:
  │         result = &ActionResult{Status:"error", Error: err.Error()}
  │       results[idx] = result
  │
  │       ── 审计写入（即使失败也记录）──→ C6
  │       if auditHook != nil:
  │         auditHook.Append(&AuditEntry{
  │           Source: "action", Action: "execute",
  │           Input: c.Params, Output: result,
  │           Metadata: {"action_name": c.Name},
  │         })
  │         ← _, _ = auditHook.Append(entry)
  │         ← 审计失败不阻断 Action（关键设计）
  │     }(i, call)
  │
  └── wg.Wait()
      return results  ← 顺序与输入 calls 对齐
```

### ActionExecutor 三种实现的入口

| Executor | 文件:行号 | 内部分支 |
|---------|----------|---------|
| `LocalFunctionExecutor` | action/executor_local.go | 直接调用 Go func |
| `SandboxExecutor` | action/executor_sandbox.go（需 `with_sandbox`） | 调用 sandbox.Manager → Provider（7 种后端） |
| `MCPExecutor` | [action/executor_mcp.go:78] | 调用 mcp.Client.CallTool → C5 |

> **可插拔架构改进（v2）：build tag 对调用链的影响**
>
> `SandboxExecutor` 通过 `//go:build with_sandbox` 隔离，对调用链的影响如下：
>
> - **默认模式（`go build ./...`）**：使用 `action/executor_sandbox_stub.go` 的占位 `SandboxExecutor`。C3 调用链中 `SandboxExecutor.Execute` 直接返回 `ActionResult{Status:"error", Error:"sandbox executor requires building with -tags with_sandbox"}`，不进入 sandbox.Manager / Provider 分支，也不引入 `github.com/inferglow/sandbox` 依赖。
> - **沙箱模式（`go build -tags with_sandbox ./...`）**：使用 `action/executor_sandbox.go` 的真实 `SandboxExecutor`。C3 调用链完整走通 sandbox.Manager → Provider（7 种后端）→ Handle.Execute，并可能触发 C13 审批链。
>
> 两种模式下 `SandboxExecutor` 都满足 `action.ActionExecutor` 接口（编译期断言 `var _ ActionExecutor = (*SandboxExecutor)(nil)`），注册逻辑无需区分模式。

---

## 五、C4 · MCP 工具发现注册链

启动期一次性：连接 MCP server，拉取工具列表，注册为 Action。

```
[application startup]
  │
  ▼
[action/executor_mcp.go:196] DiscoverMCPTools(ctx, client, registry)
  │
  ├──[mcp/client.go:313] client.ListTools(ctx)
  │     → 发送 JSON-RPC 2.0 "tools/list"
  │     → readLoop 多路分解响应
  │     → 返回 []Tool
  │
  └── for each tool:
        [executor_mcp.go:151] NewMCPAction(client, tool)
          │
          ├──[executor_mcp.go:60] NewMCPExecutor(client, tool.Name)
          │     → MCPExecutor{client, toolName}
          │
          ├── 构造 action.Action{
          │     Name: tool.Name,
          │     Description: tool.Description,
          │     Schema: tool.InputSchema,
          │     Executor: MCPExecutor,
          │   }
          │
          └──[action/action.go:103] registry.Register(a)
                → 名字冲突 → return error
                → 否则存入 map
```

### MCP 初始化子链（首次连接）

```
[action/mcp/client.go:97] NewClient(transport)
  │
  ├──[L?] 启动 readLoop goroutine
  │     for { 解析 JSON-RPC 响应 → 按 reqID 分发到 pending map }
  │
[client.go:274] client.Initialize(ctx)
  │
  ├── 发送 "initialize" 请求（protocolVersion = "2024-11-05"）
  ├── 接收 ServerInfo{ProtocolVersion, ServerInfo, Capabilities}
  └── 返回 *ServerInfo（含 capabilities.tools 是否支持）
```

---

## 六、C5 · MCP 工具调用链

运行期：LLM 决定调用某 MCP 工具，由 MCPExecutor 转发到 MCP server。

```
[来自 C3 dispatcher 的 ActionExecutor.Execute 调用]
  │
  ▼
[action/executor_mcp.go:78] MCPExecutor.Execute(ctx, input)
  │
  ├──[executor_mcp.go:?] 构造 arguments map[string]any
  │
  ├──[mcp/client.go:330] client.CallTool(ctx, toolName, arguments)
  │     │
  │     ├── atomic 加 reqID
  │     ├── pending[reqID] = make(chan *jsonRPCResponse)
  │     ├── 序列化 JSON-RPC 2.0 请求：
  │     │     {"jsonrpc":"2.0","id":N,"method":"tools/call",
  │     │      "params":{"name":toolName,"arguments":args}}
  │     ├── transport.Send(rawBytes)
  │     │     → stdio: 写到 stdin
  │     │     → 其他 transport 各自实现
  │     ├── select {
  │     │     case resp := <-pending[reqID]:
  │     │       if resp.Error != nil: return error
  │     │       return parseContents(resp.Result)
  │     │     case <-ctx.Done():
  │     │       return ctx.Err()  ← 请求在途时也可取消
  │     │   }
  │     └── defer delete(pending, reqID)
  │
  └──[executor_mcp.go:?] 将 []Content 映射为 ActionResult.Result
        Content 三种类型：
          text         → string
          image        → map{type, data, mimeType}
          resource_link → map{type, uri, name}
        返回 &ActionResult{Status:"success", Result: contents}
```

---

## 七、C6 · 审计链写入链

每次 Decision / Action 执行后写入。`NoOpHook` 时为空操作。

```
[caller in engine.go L405 / dispatcher.go L?]
  │  auditHook.Append(&AuditEntry{...})
  ▼
[audit/chain.go:100] AuditChain.Append(entry)
  │
  ├──[L?] if !c.enabled: return "", nil  ← 默认关闭，零开销
  │
  ├──[L?] c.mu.Lock(); defer c.mu.Unlock()
  │
  ├──[L?] 计算 prevHash = c.lastHash（首条为 genesisHash）
  │
  ├──[L?] entry.PrevHash = prevHash
  ├──[L?] entry.Timestamp = c.clock()
  │
  ├──[L?] 计算 entry.Hash：
  │     payload := prevHash + timestamp + source + action + serialize(input/output)
  │     entry.Hash = sha256(payload)
  │
  ├──[L?] 计算 entry.Signature（若配置 HMAC key）：
  │     entry.Signature = hmac.sha256(key, entry.Hash)
  │
  ├──[L?] c.entries = append(c.entries, entry)
  │
  ├──[L?] c.lastHash = entry.Hash  ← 推进哈希指针
  │
  ├──[L?] MaxEntries 软上限检查：
  │     if len(entries) > MaxEntries:
  │       滚动淘汰最旧条目（保留最新）
  │
  └──[L?] return entry.Hash, nil  ← 返回新条目哈希
```

---

## 八、C7 · 审计链验证链

离线取证或启动期校验，三重检查确保链未被篡改。

```
[audit/verify.go:39] AuditChain.VerifyChain()
  │
  ├── c.mu.RLock(); defer c.mu.RUnlock()
  │
  ├── 检查 1: prev_hash 连续性
  │     for i := 1; i < len(entries); i++:
  │       if entries[i].PrevHash != entries[i-1].Hash:
  │         return ErrChainBroken{Index: i, Reason: "prev_hash mismatch"}
  │
  ├── 检查 2: hash 重计算
  │     for i, entry := range entries:
  │       recomputed := sha256(entry.PrevHash + entry.Timestamp + ... + serialize)
  │       if recomputed != entry.Hash:
  │         return ErrChainBroken{Index: i, Reason: "hash mismatch"}
  │     ← 任何字段被改动都会暴露
  │
  └── 检查 3: HMAC 签名验证（若配置 key）
        for i, entry := range entries:
          expected := hmac.sha256(key, entry.Hash)
          if !hmac.Equal(entry.Signature, expected):
            return ErrChainBroken{Index: i, Reason: "signature mismatch"}
        ← 防止有人替换整个链文件
```

> **设计要点**：三重检查冗余但各有用途——prev_hash 检测顺序/插入/删除；hash 检测字段篡改；signature 检测整链替换。

---

## 九、C8 · PII 输入侧脱敏链

每次 `Session.AddMessage` 都会经过 masker 链。

```
[caller in session_ext.go L49/L53/L82]
  │  s.AddMessage("user", content, "")
  ▼
[session/session.go:194] Session.AddMessage(role, content, name)
  │
  └──[L?] 调用 AddMessageChecked(role, content, name)  ← 忽略 error
        ▼
[session/session.go:208] AddMessageChecked(role, content, name)
  │
  ├──[L?] c.mu.Lock(); defer c.mu.Unlock()
  │
  ├──[L?] contentStr := ContentToString(content)
  │
  ├──[L?] if c.masker != nil:
  │       contentStr = c.masker.MaskInput(contentStr)
  │         → [security/pii/mask.go:124] Masker.MaskInput
  │            → if cfg.ApplyOn & MaskOnInput:
  │                 return m.Mask(text)
  │                   → [mask.go:108] Mask(text)
  │                      5 种正则替换：
  │                        email    → ***@***.***
  │                        phone    → 1**********
  │                        idCard   → ******************
  │                        bankCard → **** **** **** ****
  │                        credit   → **** **** **** ****
  │               else:
  │                 return text  ← MaskOnInput 未启用时原样返回
  │
  ├──[L?] 触发 security hook（若有）
  │
  ├──[L?] 追加到 FullContext（永不裁剪）
  │
  ├──[L?] 追加到 ContextWindow
  │
  └──[L?] if len(ContextWindow) > maxLength:
          resize handler 触发 → C12
```

---

## 十、C9 · PII 输出侧脱敏链

仅在 `Agent.Run` 返回最终响应前触发，与输入侧独立。

```
[agent.go:206-208]
  │  if c.piiMasker != nil:
  │    response = c.piiMasker.MaskOutput(response)
  ▼
[security/pii/mask.go:134] Masker.MaskOutput(text)
  │
  ├── if cfg.ApplyOn & MaskOnOutput:
  │     return m.Mask(text)
  │       → 同 C8 的 5 种正则替换
  │
  └── else:
        return text  ← MaskOnOutput 未启用时原样返回
```

> **关键约定**：`MaskInput` 和 `MaskOutput` 通过 `ApplyOn` 位标志独立控制，可以只输入脱敏、只输出脱敏、两侧都脱敏或不脱敏。

---

## 十一、C10 · 死循环检测链

每轮循环开头检查，避免 Agent 陷入无限规划/执行。

```
[engine.go:187-210]
  │  if e.loopGuard != nil:
  │    state := LoopGuardState{Round, ActionCalls, LastOutput, TotalTokens, StartedAt}
  │    verdict, _ := e.loopGuard.Check(state)
  ▼
[orchestrator/agent/loop_guard.go:112] LoopGuard.Check(state)
  │
  ├── 策略 1: RepeatAction（优先级最高）
  │     若连续 RepeatActionWindow 轮（默认 3）的 ActionCalls 完全相同
  │     → return {Action: VerdictBreak, Reason: "repeat_action: ..."}
  │
  ├── 策略 2: OutputStagnation
  │     若连续 OutputStagnationWindow 轮（默认 3）的 LLM 输出
  │     Jaccard 相似度 > OutputSimilarityThreshold（默认 0.9）
  │     → return {Action: VerdictBreak, Reason: "output_stagnation: ..."}
  │
  ├── 策略 3: TimeBudget
  │     if time.Since(StartedAt) > TimeBudget（默认 5min）:
  │     → return {Action: VerdictBreak, Reason: "time_budget_exceeded"}
  │
  └── 策略 4: TokenBudget
        if TotalTokens > TokenBudget（默认 100000）:
        → return {Action: VerdictBreak, Reason: "token_budget_exceeded"}

  首个命中的策略决定 verdict，后续不再检查。
  全部未命中 → return {Action: VerdictContinue}, nil
```

### Verdict 在 executeLoop 中的处理

```
[engine.go:200-210]
  switch verdict.Action {
    case VerdictBreak:                                    ← L197
      return nil, fmt.Errorf("%w: %s", ErrLoopDetected, verdict.Reason)
    case VerdictDegrade:                                  ← L199
      systemPrompt = systemPrompt + "\n" + verdict.Reason  ← 软提示，继续循环
    // VerdictContinue: 继续
  }
```

---

## 十二、C11 · 取消/抢占链

外部调用方可通过 `Engine.CancelManager()` 请求取消，三种安全点。

### 取消请求发起

```
[external caller]
  │  handle := cancelManager.Cancel(mode, opts...)
  ▼
[cancel.go:158] CancelManager.Cancel(mode, opts)
  │
  ├──[L?] atomic 存储 pendingCancel{mode, opts.Timeout, opts.Recursive}
  ├──[L?] 通知 TurnLoop.Preempt（如果 mode == CancelImmediate）
  │         → [turn_loop.go:140] TurnLoop.Preempt(reason)
  │            → 关闭 preemptCh（解阻塞 streamLoop 的 select）
  │            → 设置 phase = Idle, preempted = true
  │
  └──[L?] return &CancelHandle{done: make(chan error)}
       ← 调用方 handle.Wait() 阻塞等待完成
```

### 取消生效（在 executeLoop 的安全点检查）

```
[engine.go 三处安全点]
  │
  ├──[L218-225] Point 1: CancelImmediate
  │     每轮开头 + 所有安全点都匹配
  │     CheckTimeoutEscalation()  ← 超时的安全点取消升级为立即
  │     if CheckCancel(CancelImmediate):
  │       CompleteCancel(nil); return "agent cancelled"
  │
  ├──[L429] Point 4: CancelAfterChatModel
  │     LLM 返回后、工具执行前
  │     if CheckCancel(CancelAfterChatModel):  ← CancelImmediate 也匹配
  │       CompleteCancel(nil); return decision, nil  ← 让 LLM 输出能返回
  │
  └──[L461] Point 6: CancelAfterToolCalls
        工具执行后、下一轮前
        if CheckCancel(CancelAfterToolCalls):
          CompleteCancel(nil); return "agent cancelled after tool calls"
```

### 超时升级

```
[cancel.go:267] CheckTimeoutEscalation()
  │
  ├── if pendingCancel.timeout 已到期：
  │     pendingCancel.mode = CancelImmediate  ← 升级
  │     TurnLoop.Preempt("timeout")
  │     return true
  └── else: return false
```

---

## 十三、C12 · 会话 resize 链

ContextWindow 超长时触发的上下文裁剪（FullContext 永不裁剪）。

```
[session/session.go:208] AddMessageChecked 内部
  │  if len(ContextWindow) > maxLength:
  │    → 触发 resize
  ▼
[session.go:147-159] ListResizeHandlers / SetDefaultResizeHandler
  │
  ├── 默认 resize 策略（三选一）：
  │   1. TailWindow：保留最近 N 条
  │   2. SummarizeWindow：调用 LLM 总结旧消息，替换为 summary
  │   3. HybridWindow：先总结再裁剪
  │
  └── ResizeHandler(fullContext, contextWindow) → newContextWindow
        输入：完整历史 + 当前窗口
        输出：新的窗口切片
        ← 可自定义 handler（RegisterResizeHandler）

[session.go:365] PreparePrompt()
  │
  └── return ContextWindow  ← 返回裁剪后的窗口作为 LLM 输入
```

> **设计要点**：FullContext 永不裁剪，保证审计/取证的完整性；ContextWindow 可裁剪，控制 LLM token 成本。

---

## 十四、C13 · Sandbox 执行审批链

高危 Action（SandboxRequired=true）执行前的审批流程。

```
[action/executor_sandbox.go]
  │  SandboxExecutor.Execute(ctx, input)
  │    在调用 sandbox.Provider.Run 之前：
  ▼
[sandbox/approval.go:95] ApprovalService.Submit(req)
  │
  ├──[L?] 检查 ApprovalPolicy：
  │     if action 在 blocklist:
  │       return record{Status: Rejected}
  │     if action 在 allowlist 或 policy == AutoApprove:
  │       return record{Status: Approved}
  │     else:
  │       record{Status: Pending}  ← 等待人工/自动审批
  │
  ├──[L?] 调用 ApprovalHandler.Resolve(ctx, req)
  │     ├── InputTimeoutFailHandler：等待 ctx 超时 → Rejected
  │     ├── FailClosedHandler：直接 Rejected
  │     ├── AutoApproveHandler：直接 Approved
  │     └── AutoAllowHandler：allowlist 内 → Approved
  │
  └──[L?] return record, nil

  根据 record.Status：
    Approved → 继续 sandbox.Provider.Run
    Rejected → 返回 &ActionResult{Status:"rejected", Error:"..."}
    Pending  → 阻塞等待 Resolve 调用
```

### 审批解析（外部触发）

```
[sandbox/approval.go:123] Resolve(recordID, decision, approver)
  │
  ├──[L?] 查找 record
  ├──[L?] if record.Status != Pending: error
  ├──[L?] record.Status = Approved 或 Rejected
  └──[L?] 通知等待方
```

---

## 十五、Flow 引擎调用链（独立编排能力）

Flow 被 orchestrator 直接依赖（orchestrator/go.mod require flow）。orchestrator 通过 `FlowContext` 接口注入横切能力，支持 `WithFlow` RunOption 切换到 flow 编排模式（与默认的 executeLoop oneshot 模式互斥）。

```
[flow/engine.go:76] Flow.Execute(ctx, input) *Execution
  │
  ├──[L?] findStartStep()
  │     ← 若多个候选：按 name 排序（确定性）
  │
  ├──[L?] state := ExecutionState{Status: Running, StepLogs: []}
  │
  └── for each step in topological order:
        │
        ├──[callable_ref.go:160] CallableRef.ResolveHandler()
        │     → 从 HandlerRegistry 查找
        │     → GlobalHandlerRegistry 兜底
        │
        ├── 调用 handler(input) → output
        │
        ├── 记录 StepLogEntry
        │
        ├── 分支处理（if/else）：
        │     [branch_test.go 验证] condition 为 false 时跳过 true 分支
        │
        └── Checkpoint（若启用）：
              CheckpointManager.Save(state)
              ← 支持 Pause/Resume
```

### TriggerFlow 算子调度（事件驱动变体）

```
[flow/triggerflow/*.go]
  │  TriggerFlowEngine.OnSignal(signal)
  ▼
  for each operator in pipeline:
    OperatorHandler.Handle(ctx, signal, state)
      13 种算子各自实现：
        chunk / signal_gate / batch_fanout / for_each_split /
        match_route / intervention_point / sub_flow / ...
```

---

## 十六、模块间调用关系全景

```
┌─────────────────────────────────────────────────────────────────┐
│                      用户代码                                    │
│                         │                                        │
│                         ▼                                        │
│              ┌──────────────────────┐                            │
│              │  orchestrator/agent  │  ← 用户唯一入口            │
│              │   Agent.Run (C1)     │                            │
│              └──────────┬───────────┘                            │
│                         │                                        │
│           ┌─────────────┼─────────────┐                          │
│           ▼             ▼             ▼                          │
│     ┌──────────┐  ┌──────────┐  ┌──────────┐                    │
│     │ session  │  │  model   │  │  action  │                    │
│     │  (C8,C12)│  │ Provider │  │Registry  │                    │
│     └────┬─────┘  └────┬─────┘  └────┬─────┘                    │
│          │             │             │                           │
│          │             │      ┌──────┼──────┐                    │
│          │             │      ▼      ▼      ▼                    │
│          │             │  Local  Sandbox  MCP                    │
│          │             │         │        │                      │
│          │             │         ▼        ▼                      │
│          │             │   ┌─────────┐ ┌──────────┐             │
│          │             │   │ sandbox │ │action/mcp│             │
│          │             │   │Manager  │ │ Client   │             │
│          │             │   └─────────┘ └──────────┘             │
│          │             │            (C5)                         │
│          │             │                                          │
│          ▼             ▼                                          │
│     ┌──────────┐  ┌──────────┐                                    │
│     │security  │  │  audit   │  ← 横切关注点                      │
│     │pii/rbac/ │  │ AuditChain│                                   │
│     │inject/   │  │ (C6,C7)  │                                   │
│     │ratelimit │  └──────────┘                                    │
│     └──────────┘                                                  │
│                                                                   │
│     ┌──────────────┐  ┌──────────────┐  ┌──────────────┐         │
│     │observability │  │  workspace   │  │    flow      │         │
│     │   otel       │  │  (path 安全) │  │ (独立编排)   │         │
│     └──────────────┘  └──────────────┘  └──────────────┘         │
└─────────────────────────────────────────────────────────────────┘
```

---

## 十七、关键错误传播路径

| 错误来源 | 抛出位置 | 传播路径 | 用户可见 |
|---------|---------|---------|---------|
| LLM 超时 | `engine.go:335` | executeLoop → Run | ✅ "context deadline exceeded" |
| LoopGuard break | `engine.go:203` | executeLoop → Run | ✅ ErrLoopDetected |
| Cancel immediate | `engine.go:225` | executeLoop → Run | ✅ "agent cancelled" |
| Cancel after tool | `engine.go:465` | executeLoop → Run | ✅ "agent cancelled after tool calls" |
| Preempt | `engine.go:333` | executeLoop → Run | ✅ "agent preempted: <reason>" |
| Action panic | `dispatcher.go:?` | recover → 合成 ActionResult | ❌ 体现在 result.Status="error" |
| Action 未找到 | `action.go:?` | Registry.Execute → Dispatcher | ❌ 体现在 result.Status="error" |
| 审计写入失败 | `chain.go:?` | `_, _ = Append()` 吞掉 | ❌ 不影响主流程 |
| 输出注入检测 | `agent.go:197` | Run → return err | ✅ error 阻断响应 |
| PII masker panic | `mask.go:?` | 无 recover，会上抛 | ✅ 直接 panic（设计如此） |
| ParseDecision 失败 | `engine.go:326` | 降级为 response 决策 | ❌ 返回原文 |
| L4 校验失败 | `engine.go:375` | executeLoop → Run | ✅ "L4 output validation failed after retries" |
| MCP 调用超时 | `client.go:?` | ctx.Done() → return err | ❌ 体现在 result.Status="error" |

---

## 十八、阅读导航

| 想了解 | 跳转 |
|-------|------|
| 整体架构与模块依赖 | [01-architecture-overview.md](./01-architecture-overview.md) |
| LLM Provider 细节 | [02-model-and-schema.md](./02-model-and-schema.md) |
| Flow 编排引擎 | [03-flow.md](./03-flow.md) |
| Action Runtime 细节 | [04-action-and-mcp.md](./04-action-and-mcp.md) |
| Session/Sandbox/Audit 内部 | [05-session-sandbox-audit.md](./05-session-sandbox-audit.md) |
| Security/OTel/Workspace | [06-security-observability-workspace.md](./06-security-observability-workspace.md) |
| Orchestrator 内部机制 | [07-orchestrator.md](./07-orchestrator.md) |
| **端到端调用链（本文档）** | 08-call-chains.md |
