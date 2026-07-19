# V6 L1-L4 实施计划

## 决策：四任务独立，推荐顺序 L1 → L3 → L2 → L4

按 ROI 排序：L1 最小(3d)且直接降成本 → L3 纯新增(5d)扩展接入面 → L2 新模块(5d)质量保障 → L4 最大(8d)且需 Windows 环境。

---

## Phase A: L1 — Prefix Cache Budget 反馈回路 (~3天, ~120行)

**目标**: 将 LLM 响应中的 `cached_tokens` 反馈到 context manager，动态调整 sweet-spot 阈值，最大化前缀缓存命中率。

**策略**: 不修改 `ContextManager` 接口（零回归）。Engine 增加可选回调字段，HybridManager 增加公开方法，Agent 层桥接。

### Step 1: Engine 增加 cacheBudgetHook
- `orchestrator/agent/engine.go` — Engine struct (L66-132) 增加:
  ```go
  cacheBudgetHook func(cachedTokens int)
  ```
- executeLoop 中 lastUsage 消费后 (L568+) 增加 ~5 行:
  ```go
  if e.cacheBudgetHook != nil && lastUsage != nil {
      if cached := lastUsage.PromptTokensDetails["cached_tokens"]; cached > 0 {
          e.cacheBudgetHook(cached)
      }
  }
  ```

### Step 2: HybridManager 增加 UpdateCacheBudget 方法
- `context/hybrid.go` — 新增 ~20 行公开方法（不改接口）:
  ```go
  func (h *HybridManager) UpdateCacheBudget(cachedTokens int) {
      if cachedTokens <= 0 { return }
      h.toleranceMu.Lock()
      defer h.toleranceMu.Unlock()
      effective := cachedTokens + h.estimateTotalTokens()
      cap := int(float64(h.sweetSpotOriginal) * 1.5)
      if effective > cap { effective = cap }
      if effective > h.sweetSpotTokens {
          h.sweetSpotTokens = effective
      }
  }
  ```
- 上限 1.5× 防止窗口压力；`toleranceMu` 已存在保护并发读写

### Step 3: Agent 层桥接
- `orchestrator/agent/agent.go` — Agent struct 增加 `cacheBudgetUpdater` 字段 + `WithContextManager` RunOption
- 定义最小接口避免循环依赖:
  ```go
  type CacheBudgetUpdater interface {
      UpdateCacheBudget(cachedTokens int)
  }
  ```
- Run 中设置 `engine.cacheBudgetHook = func(c int) { a.cacheBudgetUpdater.UpdateCacheBudget(c) }`

### Step 4: 空实现 + 测试
- `context/passthrough.go` + `context/threezone_adapter.go` — 无需改动（不修改接口）
- `orchestrator/agent/engine_test.go` — 验证 hook 被正确调用
- `context/hybrid_cache_test.go` — 验证 UpdateCacheBudget 行为（上调、上限、并发安全）

### 关键文件
1. `orchestrator/agent/engine.go` — hook 字段 + 调用点
2. `context/hybrid.go` — UpdateCacheBudget 方法
3. `orchestrator/agent/agent.go` — 桥接
4. `model/chat.go` — UsageInfo 结构（只读参考）

---

## Phase B: L3 — MCP SSE Transport (~5天, ~400行)

**目标**: 为 MCP server 添加 SSE 传输层，支持多客户端并发连接。

**策略**: 纯新增 `serve_sse.go` + `serve_sse_test.go`，不修改任何现有文件。复用 `Server.HandleMessage()` 处理消息。

### Step 1: SSEFrameTransport 实现
- 新建 `mcpserver/serve_sse.go` (~200行):
  - `SSEFrameTransport` struct: `sendCh chan []byte`, `recvCh chan []byte`, `done chan struct{}`
  - 实现 `FrameTransport` 接口 (Send/Recv/Close)
  - Send: 非阻塞写入 sendCh，满时丢弃最旧
  - Recv: 从 recvCh 读取，ctx 取消返回 err
  - Close: sync.Once 幂等

### Step 2: SSEHandler + Session 管理
- `SSEHandler() http.Handler` 返回处理两个端点的 handler:
  - `GET /sse` — 建立 SSE 连接，设置 `Content-Type: text/event-stream`，goroutine 从 sendCh 写 `data: {...}\n\n`
  - `POST /messages?sessionId=xxx` — 读取 body，调用 `Server.HandleMessage()`，响应写入对应 session 的 sendCh
- `sseSessionManager`: `sync.Map` 存储 sessionID → *SSEFrameTransport
- 心跳: 每 30s 发送 `: keepalive\n\n`
- 最大连接数限制（默认 1000）

### Step 3: ServeSSE 便捷函数
```go
func ServeSSE(s *Server, addr string) error
```

### Step 4: 测试
- 新建 `mcpserver/serve_sse_test.go` (~200行):
  - SSEFrameTransport Send/Recv/Close 基本功能
  - GET 建立 SSE + POST 发送消息端到端流程
  - 100 并发客户端无 race
  - 客户端断开后 session 清理
  - 完整 MCP 握手: initialize → tools/list → tools/call

### 关键文件
1. `mcpserver/serve_sse.go` — 新文件，核心实现
2. `mcpserver/serve_sse_test.go` — 新文件，测试
3. `mcpserver/server.go` — HandleMessage 复用（只读参考）
4. `mcpserver/serve_http.go` — HTTP handler 参考模式
5. `mcpserver/transport.go` — FrameTransport 接口（只读参考）

---

## Phase C: L2 — Eval Runner 评估框架 (~5天, ~700行)

**目标**: 从零构建评估框架，支持场景定义、mock 执行、结果对比、报告输出。

**策略**: 独立 `eval/` 模块，仅依赖现有公开接口，不修改任何现有代码。复用 `replay.go` 的比较逻辑。

### Step 1: 核心类型定义
- 新建 `eval/go.mod` — 独立模块
- 新建 `eval/types.go` (~100行):
  ```go
  type Suite struct { Name string; Cases []Case; Parallelism int }
  type Case struct {
      Name, Input, SystemPrompt string
      Expect Expectation
      Tools []ToolStub
  }
  type Expectation struct {
      Contains, NotContains, ToolSequence []string
      MaxRounds int
  }
  type CaseResult struct { CaseID string; Pass bool; Response string; Latency time.Duration; Usage model.UsageInfo; Diffs []string }
  type Report struct { Suite string; Total, Passed, Failed int; P50, P95 time.Duration; Results []CaseResult }
  ```

### Step 2: MockProvider
- 新建 `eval/mock_provider.go` (~80行):
  - `ScriptedProvider`: 预录响应序列，复用 `engine_test.go` 的 mockModelRequester 模式
  - 实现 `model.StreamRequester` 接口
  - 支持延迟模拟 (`ResponseDelay time.Duration`)

### Step 3: Runner 核心
- 新建 `eval/runner.go` (~200行):
  - `Runner.Run(ctx, suite) (*Report, error)`:
    - 遍历 Cases，为每个 Case 构造独立 Agent（工厂模式，避免状态污染）
    - 默认串行（Parallelism=1），可配置 semaphore 控制并发
    - 收集 CaseResult，计算 P50/P95 延迟
  - 使用 `action.New()` 注册 ToolStub 为 mock action

### Step 4: Replay 集成
- 新建 `eval/replay_adapter.go` (~60行):
  - 从 golden session JSON 自动生成 Case
  - 使用 `replay.ReplayCompare()` 和 `replay.ReplayToolCallSequence()` 验证

### Step 5: 报告输出
- 新建 `eval/report.go` (~80行):
  - 文本表格 + JSON 格式
  - 退出码: 0=全通过, 1=有失败

### Step 6: 测试
- 新建 `eval/eval_test.go` (~150行):
  - 3 个场景: 直接响应、单工具调用、多轮对话
  - 验证 Runner 正确收集结果和 assertion
- 新建 `eval/testdata/` — golden session fixtures

### 关键文件
1. `eval/runner.go` — 核心执行引擎
2. `eval/types.go` — 数据结构
3. `eval/mock_provider.go` — 测试 mock
4. `orchestrator/agent/replay.go` — 复用比较逻辑
5. `orchestrator/agent/agent.go` — Agent.Run 入口（只读参考）

---

## Phase D: L4 — Windows Sandbox 真实实现 (~8天, ~500行)

**目标**: 将三个 Windows 沙箱后端从 stub 升级为真实 syscall 隔离实现。

**策略**: 逐后端递进，每个独立可编译可测试。RestrictedToken(2d) → AppContainer(2.5d) → WindowsSandbox(2d) → 集成测试(1.5d)。全部 `//go:build windows` 保护。

### Step 1: RestrictedToken 后端 (2天)
- `sandbox/windows_restricted_token.go` — 实现 `CreateRestrictedToken()`:
  - `advapi32.dll`: `OpenProcessToken` → `DuplicateTokenEx` → `CreateRestrictedToken`（移除 SeDebugPrivilege 等高权限）
  - `Execute()` 使用 `CreateProcessAsUser` 以受限 token 启动
- 新建 `sandbox/windows_syscall.go` (~80行): advapi32.dll proc 声明封装

### Step 2: AppContainer 后端 (2.5天)
- `sandbox/windows_appcontainer.go` — 实现 `setupAppContainerEnvironment()`:
  - `userenv.dll`: `CreateAppContainerProfile` 创建 SID
  - `SetEntriesInAcl` 配置文件系统 ACL
  - `CreateProcessAsUser` + `SECURITY_CAPABILITIES` 启动
- `configureFilesystemAccess()` / `configureRegistryAccess()` 真实实现
- `Stop()` 清理 AppContainer profile

### Step 3: WindowsSandbox 后端 (2天)
- `sandbox/windows_sandbox.go` — 实现 `Start()`:
  - 调用 `generateWSConfig()`（已有）写入临时 .wsb 文件
  - `exec.Command("WindowsSandbox.exe", configPath)` 启动 VM
  - 就绪文件轮询等待
- `Execute()` 通过共享文件夹传递命令/结果
- `Stop()` 终止进程 + 清理临时文件

### Step 4: 集成测试 (1.5天)
- 增强各后端 `*_test.go`:
  - RestrictedToken: 验证进程无法访问管理员目录
  - AppContainer: 验证文件系统隔离
  - WindowsSandbox: 验证 VM 启动和共享文件夹通信
- 跨后端测试: `selectStrongestAvailable()` 正确选择

### 关键文件
1. `sandbox/windows_restricted_token.go` — 后端 1
2. `sandbox/windows_appcontainer.go` — 后端 2
3. `sandbox/windows_sandbox.go` — 后端 3
4. `sandbox/provider.go` — 接口定义（只读）
5. `sandbox/windows_runtime.go` — Provider 注册（只读）

---

## 依赖关系

```
L1 (3d) ─── 独立
L3 (5d) ─── 独立
L2 (5d) ─── 独立
L4 (8d) ─── 独立（仅 Windows 平台）
```

四任务间**无硬依赖**，可任意顺序或并行实施。

## 总变更量

| Feature | 新文件 | 修改文件 | 新增行数 |
|---------|--------|----------|----------|
| L1 | 1 (test) | 3 | ~120 |
| L3 | 2 | 0 | ~400 |
| L2 | 7 | 0 | ~700 |
| L4 | 1 | 4 | ~500 |
| **合计** | **11** | **7** | **~1720** |

## 风险矩阵

| 风险 | 影响 | 缓解 |
|------|------|------|
| L1: sweetSpotTokens 过度膨胀 | 延迟压缩 | 硬编码 1.5× 上限 + decayTolerance 持续衰减 |
| L1: 循环依赖 (orchestrator ↔ context) | 编译失败 | 最小接口 `CacheBudgetUpdater` 避免导入 |
| L2: mock 行为与真实 provider 差异 | 测试不可靠 | 提供 passthrough 模式转发到真实 provider |
| L3: SSE 连接泄漏 | 内存增长 | done channel + context 取消双重保障 |
| L3: 大量 SSE 长连接资源开销 | 性能 | 可配置最大连接数（默认 1000） |
| L4: 非 Windows 平台无法测试 | 质量 | build tag 保护；公共逻辑可跨平台测试 |
| L4: Windows API 版本兼容性 | 功能 | 每步做 feature detection + fallback |

## 拒绝的替代方案

- **L1 修改 ContextManager 接口**: 需改 3 个实现 + 所有测试，回归风险高。选择 Engine 回调 + HybridManager 新增方法。
- **L2 在 orchestrator/agent/ 内建 eval**: 污染 agent 包。选择独立 eval/ 模块。
- **L3 复用 FrameTransport 接口做 SSE**: SSE 的 GET/POST 分离在两个 HTTP 连接上，不适合 Send/Recv 同步模型。选择直接实现 HTTP handler。
- **L4 提取 windowsBaseHandle 公共基类**: 增加间接层，三个后端 Execute 差异不小（token vs AppContainer vs VM）。选择逐后端独立实现。
- **L4 Token 池化**: 过早优化，先确保正确性，性能优化推迟到有实际 benchmark 数据后。
