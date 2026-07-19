# InferGlow v10 架构增强 Spec

> 基于 v9 成熟度审计提出的 10 项架构改进方向
> 创建日期：2026-07-31
> 状态：P0 实现中 / P1-P3 待排期

---

## 优先级总览

| 优先级 | 编号 | 改进项 | 涉及模块 | 状态 |
|:------:|:----:|--------|---------|:----:|
| **P0** | #2 | ContentBlock 多模态统一类型 | model/ | ✅ 完成 |
| **P0** | #8 | ToolFilter 动态工具过滤 | action/ | ✅ 完成 |
| **P1** | #1 | EmbeddingRequester 批量接口提升到 model 层 | model/ | ✅ 完成 |
| **P1** | #9 | 审计链打通 Server/CLI（可选开关） | server/, audit/ | ✅ 完成 |
| **P1** | #3 | 内置定价表 pricing_table.go | model/ | ✅ 完成 |
| P2 | #4 | 泛型探索 ActionResult[T] / MemoryStore[T] | action/, context/ | 待排期 |
| P2 | #5 | 跨会话存储上升到记忆编排 | flow/, context/ | ✅ 完成 |
| P2 | #6 | 线程管理加强（命名执行线程） | flow/ | 待排期 |
| P2 | #7 | 多模态工具（生图/TTS） | builtins/ | ✅ 完成 |
| P3 | #10 | 带 UI 的 Agent Server | server/ + 前端 | 待排期 |

---

## #2 ContentBlock 多模态统一类型 [P0]

### 背景
当前 `model.Attachment{Type string, Data any}` 是弱类型预留桩，无 Provider 实际处理。作为 CLI/GUI 底座，多模态优先级提升。

### 设计
在 `model/` 层引入 `ContentBlock` 替代 `Attachment`：

```go
type ContentType string
const (
    ContentText  ContentType = "text"
    ContentImage ContentType = "image"
    ContentAudio ContentType = "audio"
    ContentVideo ContentType = "video"
    ContentFile  ContentType = "file"
)

type ContentBlock struct {
    Type     ContentType
    MIMEType string
    Data     []byte       // 内联数据
    URL      string       // 远程引用
    Meta     map[string]any
}
```

### 集成点
- `ModelRequest.ContentBlocks []ContentBlock` 替代 `Attachment`
- `ActionResult.ContentBlocks []ContentBlock` 支持多模态工具输出
- `StreamChunk.ContentBlocks []ContentBlock` 支持流式多模态
- Provider 层：各 Provider 实现 `buildContentBlocks()` 适配

### 文件
- `model/content_block.go` — 类型定义 + 辅助构造函数

---

## #8 ToolFilter 动态工具过滤 [P0]

### 背景
当前有三层工具限制（ActionSpec / Policy / Skill 白名单），但缺少动态 per-request 过滤。Plan mode 需要"只读工具可用、写工具禁用"。

### 设计
```go
type ToolFilter struct {
    Allowed    []string        // 白名单（空=不过滤）
    Forbidden  []string        // 黑名单
    MaxLevel   SideEffectLevel // 最大副作用级别
}

func (f *ToolFilter) Apply(registry *ActionRegistry) []string
func (f *ToolFilter) IsAllowed(action *Action, spec *ActionSpec) bool
```

### 集成点
- Server：`X-Tool-Profile` 请求头 → 自动构建 ToolFilter
- CLI：`--tool-profile` 参数
- Plan Mode：`MaxLevel = SideEffectRead`
- Skill：复用 `Skill.AllowedTools`

### 文件
- `action/tool_filter.go` — ToolFilter 定义 + Apply 逻辑

---

## #1 EmbeddingRequester 批量接口 [P1]

### 背景
`context/retrieval.Embedder` 是单条签名 `Embed(ctx, string) → []float32`，`rag.EmbeddingModel` 是批量签名 `Embed(ctx, []string) → [][]float32`。两者不兼容。需要在 `model/` 层统一。

### 设计
```go
type EmbeddingRequester interface {
    Embed(ctx context.Context, texts []string) ([][]float32, error)
    Dim() int
    ModelName() string
}
```

### 集成点
- `EmbeddingCache` 可选使用 `EmbeddingRequester` 替代 `Embedder`
- `rag.EmbeddingModel` 可实现 `model.EmbeddingRequester`
- Provider 层：OpenAI `text-embedding-3-small` 等

### 文件
- `model/embedding.go` — 接口定义 + NoopEmbeddingRequester

---

## #9 审计链打通 Server/CLI [P1]

### 背景
`audit.AuditChain` 在 orchestrator 层工作，但 Server/CLI 不直接调用。`SecurityConfig` 有布尔开关但未接线。

### 设计
- `Config` 新增 `Audit AuditConfig` 段（`enabled`, `storage_path`, `signature_key`）
- Server 启动时根据 `cfg.Audit.Enabled` 创建 `audit.AuditChain`
- Server handler 在入口/出口写审计记录
- CLI 同理
- 保留 `GET /v1/audit/verify` 端点验证链完整性

### 文件
- `server/config/config.go` — 新增 AuditConfig 段
- `server/handlers_audit.go` — 审计相关端点
- Server 启动逻辑 — 条件创建 AuditChain

---

## #3 内置定价表 [P1]

### 背景
`model.Pricing` 已有成本计算基础设施，但无内置定价表。需手动 `SetPricing()`。

### 设计
```go
var DefaultPricingTable = map[string]Pricing{
    "gpt-4o":          {Input: 2.50, CacheHit: 1.25, Output: 10.00, Currency: "USD"},
    "gpt-4o-mini":     {Input: 0.15, CacheHit: 0.075, Output: 0.60, Currency: "USD"},
    "claude-sonnet-4": {Input: 3.00, CacheHit: 0.30, Output: 15.00, Currency: "USD"},
    "deepseek-v3":     {Input: 0.27, CacheHit: 0.07, Output: 1.10, Currency: "USD"},
    // ...
}

func LookupPricing(modelName string) (*Pricing, bool)
```

### 文件
- `model/pricing_table.go` — 定价表 + LookupPricing

---

## #4 泛型探索 [P2]

### 方向
- `ActionResult[T]`：编译期类型安全，改造成本最低
- `MemoryStore[T]`：记忆条目类型约束
- `TypedAgent[M]`：参考 Eino，终态目标但改造成本最高

### 建议路径
`ActionResult[T]` → `MemoryStore[T]` → `CheckpointStore[T]` → `TypedAgent[M]`

---

## #5 跨会话存储上升到记忆编排 [P2]

### 方向
- `ExecutionSnapshot` 中的关键信息自动提炼为长期记忆
- Intervention 记录 → 用户偏好记忆
- 统一 `PersistableStore` 接口

---

## #6 线程管理加强 [P2]

### 方向
- 命名执行线程 `ExecutionThread`
- Per-tenant WorkerPool
- Team 并行执行

### 当前评估
短期不急——CLI 单用户 + Server 小规模并发压力不大。等 Server 多租户实际使用后再优化。

---

## #7 多模态工具 [P2]

### 前置
依赖 #2 ContentBlock 统一类型。

### 工具清单
| 工具 | API | 输出 |
|------|-----|------|
| `image_generate` | DALL-E 3 / SD | ContentBlock{image} |
| `text_to_speech` | OpenAI TTS / ElevenLabs | ContentBlock{audio} |
| `speech_to_text` | Whisper / Deepgram | ContentBlock{text} |

---

## #10 带 UI 的 Agent Server [P3]

### 路线图
| 阶段 | 内容 | 成本 |
|------|------|------|
| Phase 1 | REST API + 简单 Web UI | ~2 周 |
| Phase 2 | Agent 可视化编排 | ~3 周 |
| Phase 3 | 多租户管理面板 | ~2 周 |

### 前置
Agent 持久化实例管理 + 认证/授权接入 Server
