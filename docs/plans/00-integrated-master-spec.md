# InferGlow 总体执行脉络 — 整合 Spec

> 生成时间：2026-08-02
> 来源：`15-final-report.md`（上下文管理主线）+ `inferglow-flowenhance-with-comfy.md`（Flow 增强）+ `plan-glow-agentscope.md`（管理后台）+ `其他长尾任务.txt`（长尾项）
> 冲突裁决：上下文管理以 `15-final-report.md` 为唯一权威，CM-6/CM-7 已吸收。

---

## 零、总体架构：四条线

```
                    ┌─────────────────────────────────┐
                    │      InferGlow 总体演进          │
                    └─────────────────────────────────┘
                                   │
          ┌────────────────────────┼────────────────────────┐
          │                        │                        │
   ┌──────▼──────┐    ┌───────────▼───────────┐    ┌───────▼───────┐
   │ 线A: 上下文  │    │ 线B: Flow 增强         │    │ 线C: 管理后台  │
   │ 管理 (主线)  │    │ (flow 包端口模型)       │    │ (AgentScope    │
   │             │    │                        │    │  集成)         │
   └──────┬──────┘    └───────────┬───────────┘    └───────┬───────┘
          │                       │                        │
          │           ┌───────────┴───────────┐            │
          │           │ 线D: 长尾独立项        │            │
          │           │ OT-1~OT-6, OT-15, DC-6│            │
          │           └───────────────────────┘            │
          │                                                │
          └──────────── 互不阻塞，可并行 ──────────────────┘
```

**关键约束**：
- 线A 是上下文管理唯一权威，线D 中的 CM-6/CM-7 退场
- 线A/B/C 互不依赖，可完全并行推进
- 线D 各子项独立，按自身优先级逐个推进

---

## 线A：上下文管理（主线，P0）

> 来源：`15-final-report.md`，吸收 CM-6/CM-7

### A.1 核心交付物总览

| 序号 | 交付物 | 类型 | 说明 |
|------|--------|------|------|
| A-1 | 9层装配模型 | 架构 | L1-L9，每层语义独立，缓存友好 |
| A-2 | 两步装配 (Setup+Execute) | 架构 | Cleanup 由快系统异步替代 |
| A-3 | Rebackground 语义收窄 | 重构 | 仅调整 L4，不再全局重建 L1-L5 |
| A-4 | 前缀缓存策略 | 新增 | 稳定层/半稳定层/不稳定层三级分类 + 定期重建 |
| A-5 | 跨组老化调制 | 重构 | 二值 groupMod → 距离+跨组引用调制 |
| A-6 | DecayTrace | 新增 | 衰减链路可追溯，所有系数外置到 Config |
| A-7 | 检索 2+1 | 重构 | 语义+关键词+recency bias，替代 4 路 |
| A-8 | 数值隔离 | 重构 | Layer 6 按 Strength/RefCount 原始数值排序 |
| A-9 | 同组回溯展开 | 新增 | Layer 8 注入 task_group 历史 top-K |
| A-10 | 快慢系统 | 新增 | 异步小模型，P0压缩泄压 > P1状态总结 |
| A-11 | ModeAssembly 接口 | 新增 | AssemblyManager 接口 + AssemblyAudit |
| A-12 | C 轨 (Phase 1) | 新增 | 调用级临时上下文，自动清理碎片 |
| A-13 | H 维度 (Phase 2) | 新增 | 热度分区衰减，三区（显著/未定/衰减） |

### A.2 分层架构细节

#### 9 层定义与属性

| 层 | 名称 | 稳定性 | 压缩策略 | 缓存策略 |
|----|------|--------|----------|----------|
| L1 | 系统安全策略 | 永久 | 不可压缩 | 永远缓存 |
| L2 | 数字员工身份 | 永久 | 不可压缩 | 永远缓存 |
| L3 | 协议 | 永久 | 不可压缩 | 永远缓存 |
| L4 | 长任务背景 | 任务级 | 可更新 | 缓存+SHA256指纹 |
| L5 | 禁止项/行为边界 | 宪法级 | 不可压缩 | 缓存+SHA256指纹 |
| L6 | 长期记忆(LTM) | 半稳定 | 可压缩 | 缓存+SHA256指纹 |
| L7 | 压缩历史 | 可变 | 可压缩 | 每轮重建 |
| L8 | 高频索引+增强RAG | 可变 | 每轮重建 | 每轮重建 |
| L9 | 动态提示 | 可变 | 每轮重建 | 每轮重建 |

**LLM 阅读顺序**：L1 → L2 → L3 → L4 → L5 → L6 → L7 → L8 → L9

#### L4/L5 层序可配置

```go
type LayerOrder int
const (
    OrderA L4ThenL5 LayerOrder = iota  // 默认：先任务后边界
    OrderB L5ThenL4                     // 高风险场景：先边界后任务
    OrderC Merged                       // 简单任务：合并
)
```

### A.3 实现方向细化

#### A-1: 9层装配模型

**目标文件**：`context/assembly.go` + `context/layer.go`（已落地）

**关键类型**：
```go
type LayerID int
const (
    LayerSystemSafety  LayerID = 1
    LayerIdentity      LayerID = 2
    LayerProtocol      LayerID = 3
    LayerTaskBackground LayerID = 4
    LayerProhibitions  LayerID = 5
    LayerLTMFacts      LayerID = 6
    LayerCompressedHist LayerID = 7
    LayerHotIndex      LayerID = 8
    LayerHintBlock     LayerID = 9
)

type LayerContent struct {
    ID       LayerID
    Content  string
    Sha256   string
    Version  int64   // 单调递增版本号
    Stable   bool
}
```

**验收标准**：每层独立构建，修改某一层不影响其他层内容。

#### A-2: 两步装配

**目标文件**：`context/assembly.go`（已落地）

```go
type AssemblyManager interface {
    ContextManager
    AssembleSetup(ctx context.Context, req *SetupRequest) (*AssemblyOutput, error)
    AssembleExecute(ctx context.Context, req *ExecuteRequest) (*AssemblyOutput, error)
    Rebackground(ctx context.Context, req *RebackgroundRequest) error
    GetLayer(layer LayerID) (*LayerContent, error)
    InvalidateLayer(layer LayerID)
    GetLayerCacheStats() map[LayerID]LayerCacheStat
}
```

**Setup 输入**：`SetupRequest{SafetyPolicy, Identity, Protocol, TaskDescription, Prohibitions, LayerOrder}`
**Execute 输入**：`ExecuteRequest{RoundID, LTMQuery, CompressedHistory, HotIndexQuery, Hints}`

**验收标准**：Setup 输出 L1-L5 拼接结果，Execute 输出 L1-L9 完整 prompt。

#### A-3: Rebackground 语义收窄

**触发场景**：
| 场景 | 操作 |
|------|------|
| 会话恢复 | Setup → Execute |
| 上下文压缩后 | 仅 Setup |
| 任务切换 | Setup → Execute |
| 模式切换 | Setup → Execute |

**实现**：`RebackgroundRequest{NewTaskDescription, CheckProhibitionChange}` → 只重算 L4，L5 仅在 `CheckProhibitionChange=true` 时重算，L1/L2/L3 完全不动。

**与 CM-7 的关系**：CM-7（背景版本管理）的需求——`/rebackground` 对接 RewriteHeadBuffer 实现替换语义 + 历史版本链——已在此处吸收。L4 变更时保留历史版本链（version 递增），旧版本通过 `GetLayerVersion(layer, version)` 可回溯。

#### A-4: 前缀缓存策略

**目标文件**：`context/render_cache.go`（已落地）

**三级缓存**：
```go
type LayerCacheEntry struct {
    Content     string
    Sha256      string
    CreatedAt   time.Time
    TTL         time.Duration
    Fingerprint string
}

type CacheConfig struct {
    RebuildInterval     int     // 定期重建间隔（步数），如 20 for 128K, 50 for 1M
    RebuildTriggerRatio float64 // 应急触发比例（窗口利用率），如 0.85
}
```

**触发逻辑**：`stepsSinceLastRebuild >= RebuildInterval || tokenUtilization >= RebuildTriggerRatio`

**与 CM-6 的关系**：CM-6（Zone 0.5 + Zone 1 token 预算，总上限=窗口5%）已在此处吸收。`RebuildTriggerRatio` 本质上就是窗口利用率预算。Zone 0.5/1 的 token 上限通过 `CacheConfig.MaxBackgroundTokens`（默认窗口5%）控制。

**缓存命中率目标**：~67%（L8 后置后从 ~44% 提升）。

#### A-5: 跨组老化调制

**目标文件**：`context/decay.go`（已落地）

**替换逻辑**：
```go
// 旧：二值 groupMod
func oldGroupMod(sameGroup bool) float64 {
    if sameGroup { return 1.0 }
    return 1.5
}

// 新：距离+跨组引用调制
func crossGroupMod(ref RefRecord, currentGroupID int) float64 {
    if ref.TaskGroupID == currentGroupID {
        return 1.0
    }
    distance := currentGroupID - ref.TaskGroupID
    crossRefs := ref.CrossGroupRefs
    return 1.0 + 0.3*float64(distance)/(1.0+0.2*float64(crossRefs))
}
```

**验收标准**：同组=1.0，前1组被引5次=1.15，前1组未引=1.3，前3组被引10次=1.3，前3组未引=1.9。

#### A-6: DecayTrace

**目标文件**：`context/decay.go`（已落地）

```go
type DecayTrace struct {
    StepID      int
    RawDecay    int
    RefMod      float64
    FileMod     float64
    Strength    float64
    GroupMod    float64
    HeatMod     float64
    Effective   float64
    TargetLevel int
    Reason      string
}
```

**所有衰减系数外置到 Config**：`DecayConfig{RawDecayBase, RefModWeight, FileModWeight, StrengthDivisor, GroupModConfig, HeatModConfig}`

#### A-7: 检索 2+1

**目标文件**：`context/retrieval/`（已落地）

**三路**：
1. 语义相似度（embedding cosine）
2. 关键词匹配（BM25 或 TF-IDF）
3. Recency bias（LastRefAtStep + Strength 加权）

**融合策略**：路1和路2的结果取并集，路3作为加权因子调整排序。

#### A-8: 数值隔离

Layer 6 注入时按 `Strength` 或 `RefCount` 降序排列，直接输出数字排名，不转换为自然语言标签。

```
// 输出格式（示例）
[LTM Facts - ranked by strength]
#1 (strength: 95) 用户偏好使用 SSH 进行 Git 操作
#2 (strength: 87) 项目使用 Go 1.22+
#3 (strength: 72) 禁止使用 HTTPS 克隆仓库
```

#### A-9: 同组回溯展开

**触发条件**：当前 step 的 TaskGroupID 与前一步相同 → 自动展开

**数据流**：
1. 检索 `task_group_X` 内所有 steps
2. 按 Recency + Strength 排序，取 top-K
3. 展开为"回溯上下文块"（最近 N 步的关键动作 + 结果，非完整对话）
4. 注入 Layer 8

**配置**：`BacktrackConfig{TopK: 5, MaxCharsPerStep: 500}`

#### A-10: 快慢系统

**目标文件**：`context/compress/`（已落地，快慢系统为未来实现）

| 维度 | 慢系统（主LLM） | 快系统（小模型） |
|------|----------------|-----------------|
| 模型 | 全尺寸 | 轻量（如 1.5B） |
| 时机 | 每轮 LLM 调用 | 异步，按需/定期 |
| 输出 | 注入 LLM 上下文 | 仅 UI 展示 |
| 优先级 | P0 压缩泄压 > P1 状态总结 | 同左 |

**Kill Switch**：`FastSlowConfig{Enabled: true, AutoKillOnPressure: true, MaxConcurrent: 1}`

#### A-11: ModeAssembly 接口

**集成方式**：ModeAssembly 作为新增模式，不影响 ModeHybrid/ModePassthrough，热切换零中断。

**AssemblyAudit**：
```go
type AssemblyAudit struct {
    entries    []AuditEntry
    maxEntries int
}
type AuditEntry struct {
    RoundID     string
    Phase       string
    Layers      []LayerAudit
    TotalTokens int
    CacheHits   int
    CacheMisses int
    Duration    time.Duration
}
```

#### A-12: C 轨（Phase 1，P0）

**问题**：工具调用结果永久留在 store，5工具×50轮=250碎片污染检索。

**方案**：`StepRecord` 新增 `Transient`/`TransientScope`/`TransientRound` 字段，`BuildContext` 遍历时跳过过期条目。

```go
func (h *HybridManager) MarkTransient(stepID int, scope string, round int) error
func (h *HybridManager) ClearStaleTransients(currentRound int) (int, error)
```

**难度**：中。纯过滤，无算法改动。

#### A-13: H 维度（Phase 2，P1）

**问题**：EffectiveDecay 缺少独立时间维度信号。

**方案**：`RefRecord` 新增 `Heat`（0-100），三区衰减：
- 显著区（70+）：衰减减速30%（heatMod=0.7）
- 未定区（40-70）：正常（heatMod=1.0）
- 衰减区（<40）：加速30%（heatMod=1.3）

每次引用 `RecallBoost(+20)`，`EffectiveDecay` 新增 `heatMod` 乘数。

**难度**：中高。调参风险高，需生产数据校准。

---

## 线B：Flow 增强（端口驱动声明式编排）

> 来源：`inferglow-flowenhance-with-comfy.md`

### B.1 核心交付物

| 序号 | 交付物 | 类型 | 说明 |
|------|--------|------|------|
| B-1 | 端口模型 | 新增 | PortType/PortDef/EdgePort 类型体系 |
| B-2 | 端口解析器 | 新增 | PortResolver 运行时解析 + 编译期校验 |
| B-3 | Step 增强 | 增强 | InputPorts/OutputPorts 声明 |
| B-4 | Flow/Edge 增强 | 增强 | 端口级连接 + Connect() API |
| B-5 | Stage 注册表 | 增强 | StageMeta 携带端口声明 |
| B-6 | FlowDef 原生加载 | 新增 | YAML v2 格式 → *Flow 直接转换 |
| B-7 | Operator 端口化 | 增强 | 13 种算子增加端口声明 |
| B-8 | v1→v2 降级 | 兼容 | 旧格式 YAML 自动降级为 PortAny |

### B.2 实现方向细化

#### B-1: 端口模型

**目标文件**：`inferglow/flow/port.go`（新建）

```go
type PortType string
const (
    PortString PortType = "string"
    PortInt    PortType = "int"
    PortFloat  PortType = "float"
    PortBool   PortType = "bool"
    PortJSON   PortType = "json"
    PortFile   PortType = "file"
    PortCode   PortType = "code"
    PortModel  PortType = "model"
    PortAny    PortType = "any"
)

type PortDef struct {
    Name        string
    Type        PortType
    Required    bool
    Default     any
    Description string
    Enum        []string
    Min, Max    *float64
    Children    map[string]*PortDef  // 嵌套结构
}

type EdgePort struct {
    FromStep string
    FromPort string
    ToStep   string
    ToPort   string
}
```

#### B-2: 端口解析器

**目标文件**：`inferglow/flow/port_resolver.go`（新建）

**校验规则**（5条）：
1. 源 step 存在且有同名输出端口
2. 目标 step 存在且有同名输入端口
3. 端口类型兼容（PortAny 兼容所有，其余精确匹配）
4. 无悬空连接
5. Required 输入端口必须有来源

```go
func ValidatePortConnections(steps map[string]*Step, portEdges []EdgePort) error
```

#### B-3: Step 增强

**目标文件**：`inferglow/flow/step.go`（修改）

```go
type Step struct {
    Name        string
    Func        StepFunc
    Schema      *schema.OutputSchema
    InputPorts  []PortDef   // 新增
    OutputPorts []PortDef   // 新增
}
```

**向后兼容**：不声明端口 → 走旧 `any→any` 路径。

#### B-4: Flow/Edge 增强

**目标文件**：`inferglow/flow/flow.go`（修改）

```go
type Edge struct {
    From         string
    To           string
    PortMappings []EdgePort  // 新增：空=nil=全量传递
}

func (fb *FlowBuilder) Connect(fromStep, fromPort, toStep, toPort string) *FlowBuilder
```

**Build() 增强**：调用 `ValidatePortConnections`，失败返回 error。

#### B-5: Stage 注册表

**目标文件**：`inferglow/flow/stage.go`（新建）

```go
type StageMeta struct {
    Name        string
    Description string
    InputPorts  []PortDef
    OutputPorts []PortDef
}

type StageRegistry struct {
    mu sync.RWMutex
    m  map[string]StageEntry
}
```

**内置 Stage 迁移**：triage、coder、reviewer、tester、committer、quality_gate 全部携带端口声明。

#### B-6: FlowDef 原生加载

**目标文件**：`inferglow/flow/flowdef.go`（新建）

```go
type FlowDefLoader struct {
    stages *StageRegistry
}

func (l *FlowDefLoader) LoadYAML(path string) (*Flow, error)
func (l *FlowDefLoader) LoadYAMLBytes(data []byte) (*Flow, error)
```

**YAML 格式**：`api_version: flowdef/v2`，用 `source: step.port` 替代 `{{.step.field}}` 模板引用。

#### B-7: Operator 端口化

**目标文件**：`inferglow/flow/operator.go`（修改）

```go
type Operator struct {
    // ... 现有字段 ...
    InputPorts  []PortDef  // 新增
    OutputPorts []PortDef  // 新增
}
```

13 种算子全部增加端口声明（BatchFanout, MatchCase, ParallelFanout 等）。

#### B-8: v1→v2 降级

`FlowDefLoader` 自动检测版本：有 `input_ports`/`output_ports` → v2，无 → v1（降级为 PortAny）。

### B.3 实施阶段

| 阶段 | 内容 | 文件 |
|------|------|------|
| Phase 0 | 端口模型基础设施 | `port.go`, `port_resolver.go`, `step.go`, `flow.go`, `engine.go` |
| Phase 1 | Stage 元数据 + FlowDef | `stage.go`, `flowdef.go`, `operator.go` |

---

## 线C：管理后台集成

> 来源：`plan-glow-agentscope.md`

### C.1 核心交付物

| 序号 | 交付物 | 优先级 | 说明 |
|------|--------|--------|------|
| C-1 | 统一存储抽象 | P0 | StorageBackend 接口 + InMemory/Redis/SQL 实现 |
| C-2 | 多租户增强 | P0 | UserIdentity + ResourceAccessPolicy + 与 TenantManager 协同 |
| C-3 | 消息总线 | P0 | MessageBus 接口 + InMemory/Redis 实现 |
| C-4 | Session 管理 | P1 | CRUD + 级联清理 + SSE 流 |
| C-5 | Scheduler 管理 | P1 | Cron + Stateful/Stateless + 集成 Trigger |
| C-6 | 凭据管理 | P1 | CRUD + 数据屏蔽 |
| C-7 | Workspace 管理 API | P1 | 暴露 workspace 模块能力 |
| C-8 | Knowledge Base 管理 | P2 | 复用 rag 模块 |
| C-9 | MCP Hub 管理 | P2 | 市场浏览 + 安装 |
| C-10 | Skill Hub 管理 | P2 | 市场浏览 + 安装 |
| C-11 | 前端集成 | P2 | 复用 AgentScope 前端 + API 适配器 |

> **前置依赖提示**：线D 的 OT-1（Streaming/SSE 输出，P0）对 C-4（Session SSE 流）和 C-11（前端实时更新）有直接复用价值。C-3（消息总线 Pub/Sub）也可对接 OT-1 的 WebSocket 基础设施。建议 OT-1 与 C Phase 0 并行启动，到 C-4 时 streaming 基础设施已就绪，可省 1-2 天。

### C.2 实现方向细化

#### C-1: 存储抽象

**目标**：`inferglow/storage/` 目录（新建）

```go
type StorageBackend interface {
    // Agent CRUD
    CreateAgent(ctx context.Context, tenantID string, agent *Agent) error
    GetAgent(ctx context.Context, tenantID, agentID string) (*Agent, error)
    ListAgents(ctx context.Context, tenantID string) ([]*Agent, error)
    UpdateAgent(ctx context.Context, tenantID string, agent *Agent) error
    DeleteAgent(ctx context.Context, tenantID, agentID string) error

    // Credential, KB, Schedule, Session, Skill, Team, User 同理
}
```

**实现**：InMemoryStorage（channel+map）、RedisStorage（go-redis）、SQLStorage（GORM+migrate）

**与 MemoryStore 适配**：`StorageBackendAdapter` 实现 `MemoryStore` 接口，桥接到 `StorageBackend`。

#### C-2: 多租户

**与现有 TenantManager 协同**：
- `TenantManager`（API Key 认证）→ 确定租户身份
- `ResourceAccessPolicy`（资源授权）→ 控制租户内用户间数据可见性
- 两者分离，独立演进

```go
type ResourceAccessPolicy interface {
    CanAccess(ctx context.Context, userID string, resourceType string, resourceID string, action string) bool
}

// 扩展点注入
s.SetResourceAccessPolicy(policy)
```

#### C-3: 消息总线

**目标**：`inferglow/messagebus/` 目录（新建）

```go
type MessageBus interface {
    // Drain Queue
    DrainSession(ctx context.Context, sessionID string) (<-chan Message, error)
    DrainGlobal(ctx context.Context) (<-chan Message, error)

    // Replay Log
    ReplaySession(ctx context.Context, sessionID string, fromOffset int64) (<-chan Message, error)
    ReplayGlobal(ctx context.Context, fromOffset int64) (<-chan Message, error)

    // Pub/Sub
    Publish(ctx context.Context, topic string, msg Message) error
    Subscribe(ctx context.Context, topic string) (<-chan Message, error)

    // 分布式锁
    AcquireLock(ctx context.Context, key string, ttl time.Duration) (bool, error)
    ReleaseLock(ctx context.Context, key string) error
}
```

**默认实现**：InMemoryMessageBus（基于 channel），零配置可用。

#### C-4~C-7: 核心业务模块

均在 `inferglow/server/` 中新增路由和 handler，遵循现有模式（`writeJSON`/`writeError`/`writeSSEEvent`）。

| 模块 | 新增路由前缀 | 依赖 |
|------|-------------|------|
| Session | `/v1/sessions/` | C-1, C-3 |
| Scheduler | `/v1/schedules/` | C-1, C-3, trigger.CronTrigger |
| Credential | `/v1/credentials/` | C-1, C-2 |
| Workspace | `/v1/workspaces/` | C-1, workspace.Provider |

#### C-8~C-10: 扩展业务模块

| 模块 | 新增路由前缀 | 复用模块 |
|------|-------------|---------|
| KB | `/v1/knowledge-bases/` | rag.Loader, rag.Splitter, rag.Store |
| MCP Hub | `/v1/mcp-hub/` | mcpserver |
| Skill Hub | `/v1/skill-hub/` | action.ActionRegistry |

#### C-11: 前端集成

1. 复用 AgentScope 前端（React 18 + shadcn/ui + Tailwind）
2. 修改 `baseURL` 指向 Inferglow 后端
3. 创建 API 适配器层处理差异
4. 通过 `embed` 嵌入 Go 二进制或独立 Nginx 部署

### C.3 实施阶段

| 阶段 | 内容 | 预估工时 |
|------|------|---------|
| Phase 0 | 存储抽象 + 多租户 + 消息总线 | 8-11 天 |
| Phase 1 | Session + Scheduler + 凭据 + Workspace | 8-12 天 |
| Phase 2 | KB + MCP Hub + Skill Hub | 7-10 天 |
| Phase 3 | 前端集成 | 5-7 天 |
| Phase 4 | 融合架构 + 文档 | 2-3 天 |

### C.4 实施状态与挂起任务（G3 追加）

> 本小节由 G3 分支 `feat/g3-server-admin` 追加，记录路线C在 G3 白名单（`server/**`）内的推进进度与跨分支挂起项。

**已完成（G3 内无风险落地）**：

| 编号 | 交付物 | 落地文件 | 状态 |
|------|--------|---------|------|
| C-4 | Session 管理 | `server/handlers_session.go` + `server/session_store.go` | ✅ 完成 |
| C-5 | Scheduler 管理 | `server/handlers_schedule.go` + `server/schedule_store.go` | ✅ 完成 |
| C-6 | 凭据管理 | `server/handlers_credential.go` + `server/credential_store.go` | ✅ 完成 |
| C-7 | Workspace 管理 API | `server/handlers_workspace.go` | ✅ 完成 |
| C-8 | Knowledge Base 管理 | `server/handlers_kb.go` + `server/kb_store.go` | ✅ 完成 |
| C-9 | MCP Hub 管理 | `server/handlers_mcphub.go` + `server/mcphub_store.go` | ✅ 完成 |
| C-10 | Skill Hub 管理 | `server/handlers_skill_hub.go` + `server/skill_store.go` | ✅ 完成（依赖 action.ActionRegistry，只读复用） |

**挂起任务（有风险，暂缓到 G3 白名单外条件满足）**：

| 编号 | 交付物 | 挂起原因 | 解锁条件 |
|------|--------|---------|---------|
| C-11 | 前端集成 | 超出 G3 白名单（`server/**` 之外），需前端目录 + embed 构建，跨分支冲突风险高 | 前端目录纳入可写范围，或由持有前端分支的 lane 承接 |

> 说明：C-10 为本次 G3 追加实现。SkillStore 对 action 包保持只读依赖（action 归 G1 领地），删除采用软删除（shadow set）实现，因为 action.ActionRegistry 无 unregister 方法。

---

## 线D：长尾独立项

> 来源：`其他长尾任务.txt`，CM-6/CM-7 已吸收到线A，不再独立列出

| 编号 | 方向 | 优先级 | 实现方向 |
|------|------|--------|---------|
| OT-1 | Streaming/SSE 输出 | P0 | 实时流式聊天输出，支持 SSE + WebSocket。`server/stream.go`，与现有 SSE 事件流复用底层 |
| OT-2 | Multi-Agent 协作 | P0 | Host-Specialist 路由 + 任务委派。`orchestrator/multiagent/`，Host 解析任务 → 路由到 Specialist → 聚合结果 |
| OT-3 | A2A Protocol | P1 | Agent-to-Agent 跨进程/跨网络通信。`protocol/a2a/`，基于 gRPC 或 HTTP/2，消息格式 JSON |
| OT-4 | 向量检索 | P1 | Embedding-based 语义检索。`rag/vector/`，集成现有 embedding 模块，支持多种向量存储（Milvus/Qdrant/内存） |
| OT-5 | Prompt 管理 | P1 | Prompt 版本控制、模板仓库、动态组合。`prompt/manager.go`，Go template + 版本号 + 热加载 |
| OT-6 | Eval 框架 | P1 | Agent 离线评估自动化。已实现 ~750 LOC，需补充示例和文档。`eval/` 目录 |
| OT-15 | 插件系统 | P3 | 约定优先插件 + 两级权限。`plugin/` 目录，Go plugin 或 HashiCorp go-plugin |
| DC-6 | 配置热重载 | P3 | TUI 中 `/config reload` 命令。`config/watcher.go`，fsnotify 监听文件变更 + 原子替换 |

---

## 五、全局执行顺序建议

```
Phase 1（并行启动）
├── 线A: A-1~A-4（9层模型 + 装配 + Rebackground + 缓存）  ← 上下文管理地基
├── 线B: Phase 0（端口模型基础设施）                        ← Flow 增强地基
├── 线C: Phase 0（存储抽象 + 多租户 + 消息总线）            ← 管理后台地基
├── 线D: OT-1（Streaming）+ OT-2（Multi-Agent）            ← 长尾 P0
└── 线D: OT-6（Eval 框架文档）

Phase 2（线A 收尾 + 其余线继续）
├── 线A: A-5~A-9（衰减改进 + 检索 + 回溯 + 快慢系统）
├── 线A: A-12（C 轨）
├── 线B: Phase 1（Stage 元数据 + FlowDef 加载）
├── 线C: Phase 1（Session + Scheduler + 凭据 + Workspace）
└── 线D: OT-3（A2A）+ OT-4（向量检索）+ OT-5（Prompt 管理）

Phase 3（长尾收尾 + 前端）
├── 线A: A-13（H 维度）
├── 线C: Phase 2（KB + Hub）+ Phase 3（前端）
└── 线D: DC-6（配置热重载）+ OT-15（插件系统）

Phase 4：联调 + 文档

Phase 5：远期 backlog
├── 跨 step 语义融合压缩
├── 遗忘分流（三路）
└── LTM 分级降格
```

**关键原则**：
- 线A/B/C 的 Phase 0 可以同时启动，互不依赖
- 线A 的 A-12（C 轨）和 A-13（H 维度）也可独立交付，不分先后
- 线D 各子项完全独立，按 P0→P1→P3 顺序逐个推进，无相互依赖

---

## 六、冲突裁决表

| 来源 | 原内容 | 裁决 |
|------|--------|------|
| 长尾 CM-6 | Zone 0.5+1 token 预算 | 吸收到线A A-4（前缀缓存策略），不再独立追踪 |
| 长尾 CM-7 | 背景版本管理 | 吸收到线A A-3（Rebbackground 语义收窄），不再独立追踪 |
| 15-final Knox-MS | 时频衰减 | 拒绝吸收，自研 EffectiveDecay 已覆盖 |
| 15-final 三步装配 | Setup+Cleanup+Execute | 改为两步，Cleanup 由快系统替代 |
| 15-final 4路检索 | 语义+关键词+时效+重要性 | 改为 2+1（语义+关键词+recency） |
| 15-final 21档数值隔离 | 查表转换 | 改为原始数值排序 |
| 15-final OPC 扩展预留 | 多个扩展接口 | 仅保留 AssemblyAudit，其余 YAGNI |

---

## 七、风险评估

| 风险 | 影响线 | 缓解 |
|------|--------|------|
| 9层模型 prompt 膨胀 | 线A | DecayTrace 监控 + 压缩阈值调优需生产数据 |
| H 维度调参不准 | 线A | 三区阈值/衰减速率/boost 均为经验值，需生产数据校准 |
| Flow 端口模型与现有 Operator 兼容 | 线B | PortAny 降级 + 混合 Flow 单测 |
| 存储层接口过于宽泛 | 线C | 先定义 3-4 种核心资源验证，再扩展至 8 种 |
| 前端 API 适配成本过高 | 线C | 先做兼容性分析，差异过大则新建轻量前端 |
| 多线并行导致集成冲突 | 全局 | 各线 Phase 0 独立，集成点在 Phase 4 统一处理 |