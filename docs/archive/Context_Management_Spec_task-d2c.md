# 上下文混合管理 — InferGlow 可执行规格 v2

> 来源：设计考量汇总-窗口划分与压缩经济账-v6
> 修订：v2 — 前置存储/Session 对齐、分级压缩语义明确、小模型降级链路、提示词模板与拼接方式

---

## 一、存储模型与 Session 兼容（前置对齐）

#### 1.1 压缩等级与存储的对应关系

**核心对齐**：压缩等级（L0-L3）是**内容状态**，每个级别有**独立的物理文件**。按 step_id 索引，BuildContext 时根据 refs 中记录的当前 level 选用对应文件的内容。

```
压缩等级        存储文件                    内容语义
─────────────────────────────────────────────────────────────────
L0 (原文)      {uuid}.jsonl               完整未压缩内容（append-only 主文件）
L1 (简单压缩)  {uuid}.l1.jsonl            去噪瘦身后的总结
L2 (事实提取)  {uuid}.l2.jsonl            转化为结构化事实
L3 (行为掩码)  {uuid}.l3.jsonl            行为 mask（做了什么 + 参数摘要）
L4 (丢弃)      从 refs.jsonl 移除该行      不可逆，原文仍在 .jsonl 但不再参与拼接
```

**关键原则**：
- `{uuid}.jsonl`（L0）**永远是完整原文**（append-only，不修改）
- L1/L2/L3 各自独立文件，压缩产物**按级别分存**，不混放
- 某 step 当前处于哪个级别，由 `refs.jsonl` 的 `level` 字段标识
- BuildContext 渲染时：查 refs 得 level → 从对应 `.lN.jsonl` 读取压缩内容
- L4 = 从 refs.jsonl 移除（不再出现在活跃视图），原文仍物理存在于 L0
- **不需要** 单独的 `facts.jsonl`——`.l2.jsonl` 本身就是事实存储

### 1.2 存储文件清单

```
文件                              职责                         读写模式
──────────────────────────────────────────────────────────────────────────
{uuid}.jsonl                     L0 主文件（完整原文）          append-only
{uuid}.refs.jsonl                引用追踪 + 当前级别标识        upsert per step
{uuid}.l1.jsonl                  L1 简单压缩总结               append（按 step_id 索引）
{uuid}.l2.jsonl                  L2 事实提取                   append（按 step_id 索引）
{uuid}.l3.jsonl                  L3 行为掩码                   append（按 step_id 索引）
Redis ctx:{session_id}:*         热数据缓存层（可丢弃可重建）   异步写
```

### 1.3 各级文件格式

**refs.jsonl**（引用追踪 + 级别标识）：

```jsonl
{"step_id":1,"level":0,"ref_count":2,"last_ref_at_step":45,"strength":1.2,"task_group_id":1,"task_boundary":true}
{"step_id":3,"level":2,"ref_count":1,"last_ref_at_step":67,"strength":1.1,"task_group_id":1,"task_boundary":false}
{"step_id":8,"level":3,"ref_count":0,"last_ref_at_step":null,"strength":1.0,"task_group_id":1,"task_boundary":false}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| step_id | int | 全局递增序号 |
| **level** | int (0-3) | **当前压缩级别**（决定从哪个文件读取渲染内容） |
| ref_count | int | 被引用累计次数 |
| last_ref_at_step | int\|null | 最后被引用的 step |
| strength | float | 累积访问强度（初始 1.0，+0.1/ref） |
| task_group_id | int | 所属任务组 |
| task_boundary | bool | 是否为任务组起点 |

**{uuid}.l1.jsonl**（简单压缩总结）：

```jsonl
{"step_id":3,"content":"[call] grep(pattern=redis, path=./)\n[result] config.py:12 REDIS_HOST=localhost, :13 REDIS_PORT=6379","token_count":45,"compressed_at_step":20}
{"step_id":5,"content":"分析完成：需要创建 redis_config.py 并迁移 REDIS_HOST/PORT 两个配置项","token_count":32,"compressed_at_step":20}
```

**{uuid}.l2.jsonl**（事实提取）：

```jsonl
{"step_id":3,"facts":["config.py:12 REDIS_HOST=localhost","config.py:13 REDIS_PORT=6379","grep pattern=redis path=./"],"token_count":28,"compressed_at_step":35}
{"step_id":7,"facts":["redis_config.py 已创建","REDIS_TIMEOUT=30s 为生产值","旧 config.py REDIS_HOST 已废弃"],"token_count":30,"compressed_at_step":35}
```

**{uuid}.l3.jsonl**（行为掩码）：

```jsonl
{"step_id":3,"mask":"[step_3|820t|grep|pattern:redis,path:./] 搜索项目redis配置","token_count":18,"compressed_at_step":50}
{"step_id":8,"mask":"[step_8|8.2K|grep|pattern:redis.*timeout] 搜索redis超时相关行","token_count":20,"compressed_at_step":50}
```

**各级文件共有字段**：

| 字段 | 类型 | 说明 |
|------|------|------|
| step_id | int | 对应 L0 中的 step（索引键） |
| content / facts / mask | string\|[]string | 压缩产物（L1=content, L2=facts, L3=mask） |
| token_count | int | 压缩后 token 数（用于 BuildContext 预算计算） |
| compressed_at_step | int | 压缩发生时的当前 step 编号 |

### 1.4 Session 兼容层

**原则**：现有 `session.Session` / `ThreeZoneSession` 完全不动。`contextmgr` 平行独立，通过双写 + 映射关联。

```
框架每步输出：
  ├─ session.AddMessage(role, content, name)     ← 保持 FullContext 完整（不变）
  └─ ctxManager.Ingest(step)                     ← 写入 L0 jsonl + refs + Redis
```

**映射关系**：

```go
// step_id ↔ Session.FullContext[msg_index] 的对应
// Ingest 时自动记录：
type StepSessionLink struct {
    StepID    int    `json:"step_id"`
    MsgIndex  int    `json:"msg_index"`    // FullContext 下标
    SessionID string `json:"session_id"`
}
```

**BuildContext 输出如何回灌 Session**：

```
若 ctxManager.Mode() != passthrough:
    rendered := ctxManager.BuildContext(windowTokens)
    session.SetChatHistory(rendered)   ← 覆盖 ContextWindow（FullContext 不变）
```

### 1.5 模式切换时的存储行为

| 切换方向 | 行为 |
|---------|------|
| passthrough → hybrid | 从 Session.FullContext 回放生成 L0 jsonl（一次性） |
| hybrid → passthrough | 停止压缩，Session.ContextWindow 恢复为 FullContext 尾部 N 条 |
| hybrid → three_zone | 共享 L0 jsonl，ThreeZone 的 snip/prune/summary 操作 refs + lN 文件 |
| 任意 → 任意 | StepStore 共享，切换只换策略引擎，不迁移数据 |

---

## 二、分级压缩语义定义

### 2.1 四级压缩内容语义

| 级别 | 名称 | 内容语义 | 适用类型 | 可逆性 |
|------|------|---------|---------|--------|
| **L0** | 原文 | 完整未压缩内容 | 所有 | — |
| **L1** | 简单压缩 | 去噪瘦身：去重复日志行、冗余格式、空白、过渡性废话 | 所有 | 可逆（expand） |
| **L2** | 事实提取 | 保留关键事实（路径、配置值、错误消息、决策结论），丢弃推理过程 | 所有 | 可逆（expand） |
| **L3** | 掩码记录 | 仅留「做了什么」的结构体 + 一句意图总结，具体内容需 expand 恢复 | tool_result 上限 | 可逆（expand） |
| **L4** | 丢弃 | 从活跃视图移除，不再参与 BuildContext 拼接 | tool_call/推理链/失败/废弃 | **不可逆** |

### 2.2 类型约束矩阵

| step 类型 | 最高压缩级别 | 理由 |
|-----------|------------|------|
| `tool` 的 `[result]` 段 | **L3 封顶** | 工具输出是事实数据，模型无法自行生成 |
| `tool` 的 `[call]` 段 | L4（可丢弃） | 调用意图已由推理链覆盖 |
| `reasoning` | L4（可丢弃） | 模型推理可重新生成 |
| `plan` / `failed` | L4（可丢弃） | 已废弃/已覆盖 |
| `user` | **L2 封顶** | 用户意图不可丢弃 |

### 2.3 各级别压缩前后示例

**L1 简单压缩**（去噪）：
```
原文 (820 tokens):
  "让我来帮你查看一下这个问题。\n\n\n首先我需要...\n好的，现在我来执行 grep 命令...\n
   [call] grep(pattern=redis, path=./)\n[result] config.py:12: REDIS_HOST=localhost\n
   config.py:13: REDIS_PORT=6379\n\n\n以上就是搜索结果。"

L1 压缩后 (380 tokens):
  "[call] grep(pattern=redis, path=./)\n[result] config.py:12: REDIS_HOST=localhost\n
   config.py:13: REDIS_PORT=6379"
```

**L2 事实提取**：
```
L1 内容 → L2 压缩后 (85 tokens):
  "[事实] grep redis: config.py:12 REDIS_HOST=localhost, :13 REDIS_PORT=6379"
```

**L3 掩码记录**：
```
L2 内容 → L3 压缩后 (42 tokens):
  "[掩码 step_3|原820t|grep|pattern:redis,path:./] 搜索项目redis配置"
```

---

## 三、压缩模型与降级链路

### 3.1 双模型架构

```
┌─────────────────────────────────────────────────────────────┐
│                    压缩模型调用链路                            │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  首选：小模型（Qwen2.5-3B / 本地 80-100 tok/s）              │
│    │                                                        │
│    ├── 正常响应 → 使用压缩结果                                │
│    │                                                        │
│    ├── 超时（>5s）→ 重试 1 次                                 │
│    │     ├── 重试成功 → 使用                                  │
│    │     └── 重试失败 → 降级 ↓                                │
│    │                                                        │
│    ├── 不可用（连接拒绝/模型未加载）→ 降级 ↓                    │
│    │                                                        │
│    └── 质量校验失败（压缩后膨胀/格式错误）→ 降级 ↓              │
│                                                             │
│  降级：主模型（当前对话使用的 LLM）                            │
│    │                                                        │
│    ├── 正常响应 → 使用压缩结果                                │
│    │                                                        │
│    └── 主模型也不可用 → 机械降级 ↓                            │
│                                                             │
│  机械降级（无 LLM）：                                        │
│    ├── L1: 正则去噪（去空行、去重复行、截断超长行）             │
│    ├── L2: 提取 [result] 段首 200 字符 + 关键路径正则          │
│    └── L3: 框架直接生成结构体掩码（无需 LLM 意图总结）         │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

### 3.2 CompressModelClient 接口

```go
// session/contextmgr/compress/model_client.go

// CompressModelClient 压缩模型调用接口
type CompressModelClient interface {
    // Compress 执行压缩，返回压缩后文本。
    // level: 目标压缩级别 (1-3)
    // prompt: 已组装好的压缩提示词（含原文）
    Compress(ctx context.Context, level int, prompt string) (string, error)
    // Available 检查模型是否可用
    Available() bool
}

// CompressModelChain 实现小模型→主模型→机械降级的链路
type CompressModelChain struct {
    small    CompressModelClient   // 小模型（3B）
    main     CompressModelClient   // 主模型（当前对话 LLM）
    timeout  time.Duration         // 小模型超时，默认 5s
    retries  int                   // 小模型重试次数，默认 1
}

func (c *CompressModelChain) Compress(ctx context.Context, level int, prompt string) (string, error) {
    // 1. 尝试小模型
    if c.small != nil && c.small.Available() {
        result, err := c.tryWithRetry(ctx, c.small, level, prompt)
        if err == nil && c.validate(level, prompt, result) {
            return result, nil
        }
    }
    // 2. 降级主模型
    if c.main != nil && c.main.Available() {
        result, err := c.main.Compress(ctx, level, prompt)
        if err == nil && c.validate(level, prompt, result) {
            return result, nil
        }
    }
    // 3. 机械降级
    return c.mechanicalCompress(level, prompt)
}
```

### 3.3 质量校验规则

```go
func (c *CompressModelChain) validate(level int, original, compressed string) bool {
    // 规则 1: 压缩后不能膨胀（长度 ≤ 原文）
    if len(compressed) > len(original) { return false }
    // 规则 2: L3 掩码必须包含 [掩码 step_ 前缀
    if level == 3 && !strings.HasPrefix(compressed, "[掩码") { return false }
    // 规则 3: 不能为空
    if strings.TrimSpace(compressed) == "" { return false }
    return true
}
```

### 3.4 小模型复用 model 包

```go
// 复用 inferglow/model 已有的 Client 接口：
type smallModelClient struct {
    client model.Client   // 复用 model 包
    cfg    model.Config   // 指向本地 3B 端点
}

func (s *smallModelClient) Compress(ctx context.Context, level int, prompt string) (string, error) {
    resp, err := s.client.Generate(ctx, &model.GenerateRequest{
        Messages: []model.Message{{Role: "user", Content: prompt}},
        Config:   s.cfg,  // temperature=0, max_tokens=原文token数
    })
    if err != nil { return "", err }
    return resp.Content, nil
}
```

---

## 四、压缩提示词模板与拼接方式

### 4.1 L1 简单压缩提示词

```
[System]
你是一个文本压缩器。对输入内容执行"去噪瘦身"：
- 删除重复的日志行（保留首次出现）
- 删除纯空白行、分隔线、装饰性格式
- 删除过渡性废话（"让我来看看"、"好的，现在"、"以上就是"等）
- 保留所有代码块、命令输出、路径、配置值的原始格式
- 不改变任何事实性内容的措辞

输出压缩后的文本，不要添加任何解释。

[User]
{step_content}
```

### 4.2 L2 事实提取提示词

```
[System]
你是一个事实提取器。从输入内容中提取关键事实：
- 保留：文件路径、配置键值对、错误消息原文、命令及其关键输出、决策结论
- 丢弃：推理过程、尝试性探索、中间计算、已被后续结论覆盖的假设
- 格式：每行一个事实，前缀 "[事实] "
- 若内容为工具调用结果，保留 [call] 摘要（工具名+关键参数）+ [result] 关键行

输出提取的事实列表，不要添加解释。

[User]
{step_content}
```

### 4.3 L3 掩码记录提示词

```
[System]
你是一个掩码生成器。为以下操作记录生成一行掩码总结：
- 格式：[掩码 step_{id}|原{token_count}t|{tool_name}|{key_params}] {一句话意图}
- 意图总结不超过 20 字
- key_params 提取最关键的 1-2 个参数（如 pattern、path、command）

只输出一行掩码，不要添加解释。

[User]
step_id: {step_id}
token_count: {token_count}
tool_name: {tool_name}
content:
{step_content}
```

### 4.4 机械降级模板（无 LLM）

```go
// L1 机械降级：正则去噪
func mechanicalL1(content string) string {
    // 去连续空行 → 单空行
    // 去 "让我/好的/现在/以上就是" 开头的句子
    // 去重复行（相邻相同行只保留首条）
    // 截断超过 500 字符的单行（保留前 500 + "...[截断]"）
}

// L2 机械降级：正则提取
func mechanicalL2(content string) string {
    // 提取匹配 [\w/]+\.\w+:\d+ 的路径:行号
    // 提取 KEY=VALUE 模式
    // 提取 error/Error/ERROR 开头的行
    // 拼接为 "[事实] " 前缀行
}

// L3 机械降级：结构体生成（无需 LLM）
func mechanicalL3(step StepRecord) string {
    // 框架直接从 step 元数据生成：
    return fmt.Sprintf("[掩码 step_%d|原%dt|%s|%s] (机械掩码)",
        step.StepID, step.TokenCount, step.ToolName, step.KeyParams)
}
```

### 4.5 BuildContext 拼接方式

BuildContext 组装上下文窗口时，按以下顺序拼接：

```
┌─────────────────────────────────────────────────────────────────┐
│ 区域 1 · head_buffer（永不压缩）                                  │
│   宪法准则 + 操作禁止 + 初始目标 + 首条用户消息 + skill_zone        │
│   来源：Session 初始化时设定，不参与压缩流程                        │
├─────────────────────────────────────────────────────────────────┤
│ 区域 2 · 高频事实注入区（从 .l2.jsonl 中筛选）                    │
│   [facts | sources: step_42,55 | ref_count≥3]                    │
│     • config.py:12 REDIS_HOST=localhost                          │
│     • REDIS_TIMEOUT=30s 为生产值                                  │
│   [/facts]                                                       │
│   来源：.l2.jsonl 中 ref_count≥3 且 strength≥1.3 的高频事实       │
├─────────────────────────────────────────────────────────────────┤
│ 区域 3 · 压缩历史区（按 step_id 升序拼接）                         │
│   对每个活跃 step，查 refs.jsonl 的 level 选用对应文件渲染：        │
│                                                                 │
│   level=0 → 从 {uuid}.jsonl 读取原文                              │
│   level=1 → 从 {uuid}.l1.jsonl 读取 content 字段                  │
│   level=2 → 从 {uuid}.l2.jsonl 读取 facts 数组拼接                │
│   level=3 → 从 {uuid}.l3.jsonl 读取 mask 字段                    │
│   (L4 的 step 已从 refs 移除，不出现)                             │
│                                                                 │
│   拼接格式：                                                     │
│   [step_1|user] 帮我把 Redis 配置迁移到独立文件                    │
│   [step_3|tool|L2] [事实] grep redis: config.py:12 REDIS_HOST=.. │
│   [step_5|reasoning|L1] 需要先创建 redis_config.py...             │
│   [step_8|tool|L3] [掩码 step_8|原8.2K|grep|redis.*timeout] ...  │
├─────────────────────────────────────────────────────────────────┤
│ 区域 4 · tail 原文区（最近 N 步保持 L0 原文）                      │
│   最近 tail_keep_steps（默认 5）步始终渲染原文，不做压缩            │
│   来源：L0 jsonl 最后 N 条                                       │
├─────────────────────────────────────────────────────────────────┤
│ 区域 5 · HintBlock 注入区（运行时动态信息）                        │
│   [hint] 上下文压力: 78% | 当前任务组: #3 | 活跃 fact: 2          │
│   来源：框架动态生成，追加在尾部                                   │
└─────────────────────────────────────────────────────────────────┘
```

### 4.6 拼接伪代码

#### 4.6.1 增量拼装加速（JSON list 直传）

**问题**：每步 BuildContext 若全量重新渲染所有 step，对已存在且未变化的 step 是冗余计算。

**方案**：引入 **rendered cache**——对已渲染且 level 未变的 step，直接以标准 JSON list 元素缓存，下次 BuildContext 时跳过重新读取/渲染，直接拼装：

```go
// 缓存结构（Redis 或内存）
type RenderedCache struct {
    StepID  int    `json:"step_id"`
    Level   int    `json:"level"`    // 渲染时的 level
    Block   string `json:"block"`    // 已渲染的完整文本（含 ⟨§N·type·Lx⟩ 标记）
    Hash    string `json:"hash"`     // 内容 hash，用于校验
}

// BuildContext 拼装时的快速路径：
func (h *HybridManager) renderStep(stepID int, ref RefRecord) RenderedBlock {
    cached := h.renderCache.Get(stepID)
    if cached != nil && cached.Level == ref.Level {
        // 快速路径：level 未变，直接复用已渲染结果
        return RenderedBlock{StepID: stepID, Level: ref.Level, Content: cached.Block}
    }
    // 慢路径：重新从 .lN.jsonl 读取 + 渲染 + 写回缓存
    block := h.renderFromStore(stepID, ref)
    h.renderCache.Set(stepID, block)
    return block
}
```

**传给 Agent 的格式**：当多个连续 step 均命中缓存时，以 JSON array 批量传递，agent 侧直接 concat 而非逐条解析：

```json
[
  {"step_id":1,"level":0,"block":"⟨§1·user·L0⟩ 帮我把 Redis 配置迁移..."},
  {"step_id":3,"level":2,"block":"⟨§3·tool·L2⟩ • config.py:12 REDIS_HOST=localhost"},
  {"step_id":5,"level":1,"block":"⟨§5·reasoning·L1⟩ 需要先创建 redis_config.py..."}
]
```

**失效条件**：
- refs.jsonl 中该 step 的 level 变更 → 缓存失效
- head_buffer 版本变更 → 全局缓存清空
- checkpoint 恢复 → 从 checkpoint 重建缓存

#### 4.6.2 完整拼接逻辑

```go
func (h *HybridManager) BuildContext(windowTokens int) ([]RenderedBlock, error) {
    var blocks []RenderedBlock

    // 区域 1: head_buffer（固定）
    blocks = append(blocks, h.headBuffer...)

    // 区域 2: 高频事实注入（从 .l2.jsonl 筛选 ref_count≥3 的条目）
    hotFacts := h.store.HotFacts(minRefCount=3, minStrength=1.3)
    for _, f := range hotFacts {
        blocks = append(blocks, renderFactsBlock(f))  // 拼接 facts 数组
    }

    // 区域 3+4: 压缩历史 + tail 原文（使用增量缓存加速）
    allSteps := h.store.AllActiveStepIDs()  // refs.jsonl 中存在的 step，升序
    tailStart := max(0, len(allSteps) - h.cfg.TailKeepSteps)

    for i, stepID := range allSteps {
        if i >= tailStart {
            // 区域 4: tail 原文（从 .jsonl 读）
            step := h.store.GetStep(stepID)
            blocks = append(blocks, RenderedBlock{StepID: stepID, Level: 0, Content: step.Content})
        } else {
            // 区域 3: 增量缓存快速路径 / 慢路径
            ref := h.store.GetRef(stepID)
            blocks = append(blocks, h.renderStep(stepID, *ref))
        }
    }

    // 区域 5: HintBlock
    blocks = append(blocks, h.buildHintBlock())

    // 裁剪：若总 token 超 windowTokens，从区域 3 头部开始升级压缩
    return h.fitToWindow(blocks, windowTokens)
}
```

### 4.7 Step 分节标记协议（发送给 API 时的格式）

#### 4.7.1 防冲突格式设计

**问题**：框架在 BuildContext 拼接后发给 LLM API 时，每个 step 需要分节标记。但 LLM 生成的内容可能包含类似标记，导致框架解析混淆。

**方案**：使用 Unicode 控制字符 + 小众分隔符组合，确保 LLM 自然生成中几乎不会出现：

```
分节标记格式：

  ⟨§{step_id}·{type}·{level}⟩

实际字节：
  \u27E8 \u00A7 {step_id} \u00B7 {type} \u00B7 {level} \u27E9

示例：
  ⟨§3·tool·L2⟩ [事实] config.py:12 REDIS_HOST=localhost
  ⟨§8·tool·L3⟩ [step_8|8.2K|grep|redis.*timeout] 搜索redis超时
  ⟨§12·user·L0⟩ 帮我把 Redis 配置迁移到独立文件
```

**为什么这个格式安全**：

| 字符 | Unicode | LLM 自然生成概率 | 说明 |
|------|---------|----------------|------|
| ⟨ | U+27E8 | 极低 | 数学括号，非普通 ASCII |
| § | U+00A7 | 极低 | 节符号，几乎不出现在代码/对话中 |
| · | U+00B7 | 低 | 中间点，区别于 ASCII `.` 和 `|` |
| ⟩ | U+27E9 | 极低 | 数学括号 |

**组合碰撞概率**：`⟨§` 这个二元组在自然文本/代码中几乎不可能出现。即使 LLM 生成了 `§` 或 `⟨`，两者紧邻的概率趋近于零。

**框架侧解析正则**：

```go
// 解析分节标记
var stepMarkerRe = regexp.MustCompile(`\u27E8\u00A7(\d+)\u00B7(\w+)\u00B7L(\d)\u27E9`)

// 防注入：框架在 Ingest 时对 LLM 输出做净化
// 若 LLM 输出中意外包含 ⟨§ 模式，替换为 [§]（破坏标记结构）
func sanitizeOutput(text string) string {
    return strings.ReplaceAll(text, "\u27E8\u00A7", "[\u00A7]")
}
```

#### 4.7.2 各级别渲染格式

```
⟨§{id}·{type}·L0⟩ {原文内容}
⟨§{id}·{type}·L1⟩ {l1.jsonl 的 content}
⟨§{id}·{type}·L2⟩ • {facts[0]}\n• {facts[1]}\n...
⟨§{id}·{type}·L3⟩ {l3.jsonl 的 mask}
```

#### 4.7.3 引用符协议（LLM 如何引用历史 step）

**LLM 侧引用格式**（在宪法提示词中约定）：

```
引用格式： §{step_id}

示例：
  "根据 §3 的 grep 结果，REDIS_HOST 在 config.py:12"
  "复用 §7 中确认的 REDIS_TIMEOUT=30s 配置"
```

**框架侧回写机制**：

```go
// 每步 LLM 输出后，解析引用符并更新 refs.jsonl
var refCiteRe = regexp.MustCompile(`\u00A7(\d+)`)

func (h *HybridManager) processCitations(output string, currentStep int) {
    matches := refCiteRe.FindAllStringSubmatch(output, -1)
    for _, m := range matches {
        stepID := atoi(m[1])
        ref := h.store.GetRef(stepID)
        if ref != nil {
            ref.RefCount++
            ref.LastRefAtStep = &currentStep
            ref.Strength += 0.1
            h.store.UpsertRef(*ref)
        }
    }
}
```

**为什么用 `§N` 而不是 `[step_N]`**：
- `[step_N]` 在代码讨论中太容易出现（"step 1", "step 2" 是常见词）
- `§N` 简洁（省 token）、唯一性高、LLM 不会自然生成
- 与分节标记 `⟨§N·type·level⟩` 共享 `§` 语义，LLM 容易学会关联

---

## 四-B、Header 宪法子区：上下文管理提示词模板

### 4B.1 宪法子区结构

head_buffer 中划分一个**上下文管理宪法子区**，位于 skill_zone 之后、历史区之前。该区域永不压缩，享受 prefix cache。

```
head_buffer 布局：
┌────────────────────────────────────────────┐
│ 宪法准则（核心行为约束）          │ ← 永不压缩
│ 操作禁止                            │ ← 永不压缩
│ 初始目标 / session scope             │ ← 永不压缩
│ 首条用户任务声明                    │ ← 永不压缩
│ skill_zone                            │ ← 永不压缩
│ ─── 上下文管理宪法子区 ───          │ ← 永不压缩（本节）
└────────────────────────────────────────────┘
```

### 4B.2 上下文管理宪法提示词模板

```
[CONTEXT_PROTOCOL]
本对话采用分级上下文管理。你看到的历史内容已经过压缩处理。

分节标记：
- 每个历史片段以 ⟨§N·type·level⟩ 开头，N 为 step 编号
- L0=原文, L1=精简, L2=事实, L3=掩码
- L3 掩码仅描述"做了什么"，具体内容不可见

引用规则：
- 当你需要引用历史 step 的内容时，使用 §N 格式（如 "根据 §3 的结果"）
- 引用会自动触发该 step 的活跃度追踪，影响后续压缩策略
- 不要编造不存在的 §N 编号

可用工具：
- context_search(query): 检索历史 step（当 L3 掩码不够用时）
- context_expand(step_id): 展开某个 step 的原文（从 L0 恢复）
- context_surround(step_id): 查看某 step 前后的上下文
- memory_search(query): 检索跨 session 长期记忆（配置值/决策/约束等持久知识）

压缩感知：
- 当你看到 [hint] 中的上下文压力超过 80%，应主动精简输出
- 不要尝试在输出中复制分节标记格式（⟨§...⟩）
- 若发现信息不足（掩码太略），先调用 context_expand 再回答
[/CONTEXT_PROTOCOL]
```

### 4B.3 模板变量与动态注入

```go
// 宪法子区在 session 初始化时一次性写入，后续不变（prefix cache 稳定）
// 仅以下字段在初始化时填充：
const contextProtocolTemplate = `[CONTEXT_PROTOCOL]
本对话采用分级上下文管理。窗口大小: %d tokens。
...(固定内容如上)...
[/CONTEXT_PROTOCOL]`

// HintBlock（区域5）是动态的，每步更新：
// [hint] pressure:78%% | task_group:#3 | active_facts:2 | tail:5steps
```

---

## 四-C、上下文自省与 refs.jsonl 管理

### 4C.1 自省触发场景

| 触发方式 | 场景 | 动作 |
|---------|------|------|
| task 确认节点 | 用户确认任务完成 / 任务阶段切换 | 全局自省：扫描 refs + 批量压缩 + checkpoint |
| 手动触发 | 用户显式请求 "整理上下文" | 同上 |
| 空闲巩固 | 空闲 N 步后自动 | 轻量自省：prune + merge（不做 checkpoint） |
| 窗口压力 | 上下文压力 > 90% | 应急自省：强制压缩 + fitToWindow |

### 4C.2 自省时的 refs.jsonl 操作流程

```
全局自省流程（task 确认 / 手动触发）：

  Step 1 · 快照（副本 + 缓存标记）
    ─────────────────────────────
    复制当前 refs.jsonl → refs.checkpoint.{step_id}.jsonl
    在副本头部写入元数据行：
      {"_checkpoint":true,"at_step":120,"header_ver":"h_v2","cache_valid":true}
    
    意义：
    - 这是压缩前的完整状态快照
    - cache_valid=true 表示与当前 head_buffer 版本匹配
    - 若后续 head_buffer 版本变更（h_v2→h_v3），
      此副本自动标记为 cache_valid=false（stale）

  Step 2 · 批量压缩
    ─────────────────────────────
    对所有活跃 step 执行 effective_decay 计算 + 压缩
    更新 refs.jsonl 中各 step 的 level 字段
    写入对应 .l1/.l2/.l3.jsonl

  Step 3 · L4 清理
    ─────────────────────────────
    将满足 L4 条件的 step 从 refs.jsonl 移除
    （原文仍在 .jsonl，但不再参与 BuildContext）

  Step 4 · task_group 归档检查
    ─────────────────────────────
    若某 task_group 内所有 step 均 ≥L3 且 ref_count=0：
    复制该组 refs 条目 → refs.archive.{task_group_id}.jsonl
    从活跃 refs.jsonl 移除

  Step 5 · 缓存标记更新
    ─────────────────────────────
    若本次自省导致 head_buffer 版本变更（如全局 summary 更新了初始目标）：
    - 所有历史 checkpoint 副本的 cache_valid 标记为 false
    - Redis 缓存层执行 FLUSH ctx:{sid}:* （重建）
    - 当前 refs.jsonl 写入新的 header_ver
```

### 4C.3 checkpoint 副本的缓存标记规则

```go
// refs.checkpoint.{step_id}.jsonl 头部元数据
type CheckpointMeta struct {
    IsCheckpoint bool   `json:"_checkpoint"`  // 恒为 true，标识这是副本
    AtStep       int    `json:"at_step"`      // 快照时的当前 step
    HeaderVer    string `json:"header_ver"`   // 快照时的 head_buffer 版本
    CacheValid   bool   `json:"cache_valid"`  // 缓存有效性标记
}

// 缓存失效规则：
// 1. head_buffer 版本变更 → 所有旧 checkpoint 的 cache_valid = false
// 2. 恢复时若 cache_valid=false → 需要重新计算 decay（不能直接复用旧值）
// 3. cache_valid=true → 可直接从 checkpoint 恢复 refs 状态（快速回滚）
```

### 4C.4 轻量自省（空闲巩固）的区别

```
空闲巩固（不做 checkpoint）：
  - 不复制 refs.jsonl（无副本）
  - 仅做预标记：将候选 L4 的 step 标记 pending_l4=true
  - 等下一个批次边界或全局自省时统一执行删除
  - 不触发 head_buffer 版本变更
  - 不影响 Redis 缓存
```

### 4C.5 refs.jsonl 字段完整定义（v3 修订）

```jsonl
{"step_id":42,"level":2,"ref_count":3,"last_ref_at_step":89,"strength":1.3,"task_group_id":1,"task_boundary":false,"semantic_hold":false,"pending_l4":false,"related_files":["config.py","redis_config.py"]}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| step_id | int | 全局递增序号 |
| level | int (0-3) | 当前压缩级别（决定从哪个 .lN 文件读取） |
| ref_count | int | 被 §N 引用的累计次数 |
| last_ref_at_step | int\|null | 最后被引用的 step |
| strength | float | 累积访问强度（初始 1.0，+0.1/ref） |
| task_group_id | int | 所属任务组 |
| task_boundary | bool | 是否为任务组起点 |
| semantic_hold | bool | Redis 语义安全网暂缓压缩标记 |
| **pending_l4** | bool | **空闲巩固预标记**（等待批次边界统一执行） |
| related_files | []string | 关联文件（活跃编辑时 file_mod=0.3） |

---

## 五、模式切换架构（独立于其他模式）

### 5.1 ContextManager 接口

```go
package contextmgr

type Mode string
const (
    ModePassthrough Mode = "passthrough"
    ModeThreeZone   Mode = "three_zone"
    ModeHybrid      Mode = "hybrid"
)

type ContextManager interface {
    Mode() Mode
    Ingest(step StepRecord) error
    BuildContext(windowTokens int) ([]RenderedBlock, error)
    TriggerCompression(opts CompressOpts) (*CompressResult, error)
    Search(query SearchQuery) ([]SearchHit, error)
    Stats() ContextStats
    Close() error
}
```

### 5.2 与 Agent Loop 集成

```
orchestrator/agent/engine.go executeLoop 每步：
  1. session.AddMessage(...)          ← 不变
  2. ctxManager.Ingest(step)          ← 新增
  3. if mode != passthrough:
       window = ctxManager.BuildContext(cfg.WindowTokens)
       session.SetChatHistory(window)  ← 覆盖 ContextWindow
```

### 5.3 模式注册与热切换

```go
type Registry struct { factories map[Mode]Factory }
func (r *Registry) SwitchMode(from, to Mode, cfg Config) (ContextManager, error)
// 切换时 StepStore 共享，只换策略引擎
```

---

## 六、压缩引擎

### 6.1 五层防线

| 层 | 触发 | 动作 | 阻塞 |
|---|------|------|------|
| 1 · per-step | effective_decay 越阈值 | 单 step 升级 L0→L1→L2→L3 | 否 |
| 2 · 空闲巩固 | 空闲 N 步（默认 10） | prune + strengthen + merge + 归档 | 后台 |
| 3 · 部分拼接 | 特定段膨胀 | 目标 step 集合提升级别 | 否 |
| 4 · 全局 summary | 任务阶段切换 | LLM 全局整理 + checkpoint | 是 |
| 5 · 最后警戒线 | 窗口即将溢出 | 应急压缩（§4.6 fitToWindow） | 是 |

### 6.2 effective_decay 公式

```
effective_decay = raw_decay × ref_mod × file_mod / strength × task_group_mod

raw_decay     = Σ token_count（last_ref_step+1 → 当前步）
ref_mod       = 1.0 / (1.0 + ref_count × 0.2)
file_mod      = 0.3（关联文件活跃编辑）| 1.0（无变化）
strength      = 初始 1.0，每次被引用 +0.1
task_group_mod = 1.5（已完成组）| 1.0（活跃组）
```

### 6.3 阈值表（128K 基准窗口）

| 目标级别 | tool 触发 | reasoning 触发 | 窗口占比(tool) |
|---------|----------|--------------|---------------|
| L1 | decay 16K-48K | decay 32K-96K | 12.5%-37.5% |
| L2 | decay 48K-128K | decay 96K-256K | 37.5%-100% |
| L3 | decay ≥128K | decay 256K-512K | ≥100% |
| L4 | — | decay ≥512K | ≥400% |

### 6.4 批次压缩 8 步

| 步骤 | 执行者 | 动作 |
|------|--------|------|
| 1 扫描 | 框架 | 计算所有活跃 step 的 effective_decay |
| 2 分级 | 框架+Redis | 按阈值表定位目标 level + 语义安全网 |
| 3 预估 | 框架 | 估算节省量，<2K 则跳过 |
| 4 执行 | **CompressModelChain** | 组装提示词 → 小模型/主模型/机械降级 |
| 5 落盘 | 框架 | 写入对应 .lN.jsonl + 更新 refs.jsonl 的 level 字段 |
| 6 验证 | 框架 | 格式校验（level 范围、hash、不膨胀） |
| 7 审计 | 主模型（按需） | context_inspector 抽查 |
| 8 诊断 | 框架 | 更新 CacheShape 指标 |

---

## 七、RAG + Redis 缓存 + 多后端持久化

### 7.1 StepStore 接口

```go
type StepStore interface {
    // --- L0 原文 (.jsonl) ---
    AppendStep(step StepRecord) error
    GetStep(stepID int) (*StepRecord, error)
    RangeSteps(from, to int) ([]StepRecord, error)

    // --- refs (.refs.jsonl) ---
    UpsertRef(ref RefRecord) error
    GetRef(stepID int) (*RefRecord, error)
    AllActiveStepIDs() ([]int, error)   // refs 中存在的 step，升序
    RemoveRef(stepID int) error          // L4 丢弃

    // --- L1 简单压缩 (.l1.jsonl) ---
    AppendL1(rec L1Record) error
    GetL1(stepID int) (*L1Record, error)

    // --- L2 事实提取 (.l2.jsonl) ---
    AppendL2(rec L2Record) error
    GetL2(stepID int) (*L2Record, error)
    HotFacts(minRefCount int, minStrength float64) ([]L2Record, error)

    // --- L3 行为掩码 (.l3.jsonl) ---
    AppendL3(rec L3Record) error
    GetL3(stepID int) (*L3Record, error)

    // --- 长期记忆 (longmem.jsonl / longterm_memories 表) ---
    UpsertLongMem(mem LongMemRecord) error
    GetLongMem(memID string) (*LongMemRecord, error)
    SearchLongMem(query string, category string, limit int) ([]LongMemRecord, error)
    RemoveLongMem(memID string) error

    // --- lifecycle ---
    Close() error
}
```

### 7.2 后端矩阵

| 后端 | 包路径 | 适用 | 对标 |
|------|--------|------|------|
| JSONL（默认） | store/jsonl/ | 单机/CLI/开发 | ReasonX, Pi |
| SQLite | store/sqlite/ | 轻量+全文搜索 | Letta 本地 |
| PostgreSQL | store/postgres/ | 生产多 session | MemGPT, LangChain |
| Redis 缓存 | store/redis/ | 检索加速（可丢弃） | Knox-MS |

### 7.3 三路融合召回

```go
type FusionRetriever struct {
    semantic  SemanticSearcher   // Redis VSS / pgvector
    keyword   KeywordSearcher    // Redis FT / SQLite FTS5 / BM25
    recency   RecencySearcher    // Redis ZSet / SQL ORDER BY
    weights   [3]float64         // {0.50, 0.30, 0.20}
    threshold float64            // 0.35
}
```

---

## 八、包结构

```
session/contextmgr/
├── manager.go           (ContextManager 接口 + Mode + RenderedBlock)
├── registry.go          (模式注册/切换)
├── config.go            (Config 全量配置)
├── compat.go            (Session 兼容：双写 + 映射 + 回灌)
├── passthrough.go       (ModePassthrough)
├── threezone_adapter.go (ModeThreeZone)
├── hybrid.go            (ModeHybrid 主实现 + BuildContext 拼接)
├── step.go              (StepRecord, RefRecord, L1Record, L2Record, L3Record)
├── decay.go             (effective_decay)
├── compress/
│   ├── engine.go        (五层防线 + 8 步流程)
│   ├── levels.go        (阈值表 + 类型约束)
│   ├── prompts.go       (L1/L2/L3 提示词模板)
│   ├── model_chain.go   (小模型→主模型→机械降级)
│   ├── mechanical.go    (机械降级实现)
│   └── idle.go          (空闲巩固)
├── retrieval/
│   ├── fusion.go        (三路融合)
│   ├── embed.go         (Embedder 接口)
│   ├── bm25.go          (BM25 现建索引)
│   └── semantic.go      (向量检索)
├── store/
│   ├── store.go         (StepStore 接口)
│   ├── jsonl/           (本地文件)
│   ├── sqlite/          (SQLite)
│   ├── postgres/        (PostgreSQL)
│   └── redis/           (Redis 缓存层)
└── tools/
    ├── search_tool.go
    ├── expand_tool.go
    ├── surround_tool.go
    └── memory_search_tool.go
```

---

## 八-B、配套工具实现规格

### 8B.1 工具注册与集成方式

inferglow 的 agent 工具系统通过 `orchestrator/tool` 包注册。上下文管理工具作为**内置工具**（非 MCP），在 `contextmgr` 初始化时自动注册到当前 session 的 tool registry：

```go
// session/contextmgr/tools/register.go

func RegisterContextTools(reg tool.Registry, mgr ContextManager) {
    reg.Register(&ContextSearchTool{mgr: mgr})
    reg.Register(&ContextExpandTool{mgr: mgr})
    reg.Register(&ContextSurroundTool{mgr: mgr})
    reg.Register(&MemorySearchTool{mgr: mgr})   // 长期记忆检索
}

// 工具元数据（所有上下文工具共享）
// - ReadOnly: true（不修改 refs/level，只读取）
// - ConcurrencySafe: true（可并行调用）
// - 权限: 自动允许（框架内部工具，无需用户确认）
// - 仅在 mode != passthrough 时注册
```

### 8B.2 context_search — 语义检索历史 step

```go
// 输入 Schema
 type ContextSearchInput struct {
    Query     string   `json:"query" jsonschema:"description=检索关键词或语义描述"`
    LevelMax  int      `json:"level_max,omitempty" jsonschema:"description=最高检索级别(0-3),默认3"`
    TaskGroup int      `json:"task_group,omitempty" jsonschema:"description=限定任务组"`
    Limit     int      `json:"limit,omitempty" jsonschema:"description=返回条数,默认5"`
}

// 输出
type ContextSearchOutput struct {
    Hits []SearchHit `json:"hits"`
}
type SearchHit struct {
    StepID    int     `json:"step_id"`
    Level     int     `json:"level"`      // 当前压缩级别
    Score     float64 `json:"score"`      // 融合得分
    Snippet   string  `json:"snippet"`    // 匹配片段（≤200字）
    Type      string  `json:"type"`       // user/tool/reasoning
}
```

**实现路径**：
```
query → FusionRetriever.Search(query, limit)
      → 三路融合（VSS 0.50 + BM25 0.30 + Recency 0.20）
      → 过滤 level ≤ LevelMax
      → 返回 hits（snippet 从对应 .lN 文件截取）
```

**错误处理**：
- Redis 不可用 → 降级为纯 BM25（从 .lN.jsonl 现建索引）
- 无命中 → 返回空 hits + 提示 "无匹配，尝试 context_expand 直接查看"

### 8B.3 context_expand — 展开原文

```go
// 输入 Schema
type ContextExpandInput struct {
    StepID    int  `json:"step_id" jsonschema:"description=要展开的step编号"`
    Full      bool `json:"full,omitempty" jsonschema:"description=true=完整原文,false=仅L1"`
}

// 输出
type ContextExpandOutput struct {
    StepID   int    `json:"step_id"`
    Level    int    `json:"current_level"`   // 当前压缩级别
    Content  string `json:"content"`         // 展开的内容
    Tokens   int    `json:"token_count"`     // 内容 token 数
    Warning  string `json:"warning,omitempty"` // 如 "原文 8.2K tokens，注意窗口压力"
}
```

**实现路径**：
```
step_id → store.GetStep(stepID)  // 从 L0 .jsonl 读取原文
        → 若 Full=false，先尝试 GetL1（更短）
        → 副作用：ref_count++ / strength+=0.1 / last_ref_at_step=当前
        → 返回内容 + token 警告
```

**副作用说明**：expand 会触发 refs 更新（等同于 §N 引用），这是设计意图——展开意味着该 step 有活跃价值，应延缓压缩。

### 8B.4 context_surround — 查看前后上下文

```go
// 输入 Schema
type ContextSurroundInput struct {
    StepID int `json:"step_id" jsonschema:"description=中心step编号"`
    Before int `json:"before,omitempty" jsonschema:"description=向前看N步,默认2"`
    After  int `json:"after,omitempty" jsonschema:"description=向后看N步,默认2"`
}

// 输出
type ContextSurroundOutput struct {
    Steps []SurroundStep `json:"steps"`
}
type SurroundStep struct {
    StepID  int    `json:"step_id"`
    Type    string `json:"type"`
    Level   int    `json:"level"`
    Content string `json:"content"`  // 按当前 level 渲染（同 BuildContext 逻辑）
    IsCenter bool  `json:"is_center"`
}
```

**实现路径**：
```
step_id ± N → store.AllActiveStepIDs() 中取范围
            → 对每个 step 按 ref.Level 从对应文件读取
            → 标记 is_center
```

### 8B.5 工具调用时的 HintBlock 反馈

每次工具调用结果返回给 LLM 时，附加一行 hint：

```
[context_tool] expand §42 → 原文 3.2K tokens | 当前窗口压力: 72% → 78%
```

这让 LLM 感知到展开操作对窗口压力的影响，引导其决策。

### 8B.6 工具与中间件钩子的集成点

| 集成点 | 时机 | 动作 |
|--------|------|------|
| Ingest 后 | 每步 LLM 输出完成 | processCitations 解析 §N 引用 → 更新 refs |
| BuildContext 前 | 每次 model call 准备 | 检查是否有 pending expand 请求（工具异步结果） |
| compress_context 触发 | 窗口压力 > 阈值 | 五层防线启动（不经过工具，框架内部） |
| on_system_prompt | session 初始化 | 注入 CONTEXT_PROTOCOL 宪法子区 |
| tool_result 回写 | 工具返回后 | 工具结果本身也作为新 step Ingest 进 L0 |

### 8B.7 实施阶段代码理解工具

> **开发约定**：本规格实施（coding）过程中，允许使用 **graphify** 理解 inferglow 现有代码结构（调用链、模块依赖、接口关系）。
> graphify 是开发辅助工具，**不是框架运行时组件**，不进入 contextmgr 包依赖。
>
> 后续框架完成后，代码 agent 可考量通过 skill 管理集成 graphify，但那属于 skill 管理范畴，不在本规格范围内。

### 8B.8 memory_search — 长期记忆检索

```go
// 输入 Schema
type MemorySearchInput struct {
    Query    string   `json:"query" jsonschema:"description=检索关键词或语义描述"`
    Category string   `json:"category,omitempty" jsonschema:"description=限定类别: config/decision/constraint/pattern"`
    Limit    int      `json:"limit,omitempty" jsonschema:"description=返回条数,默认5"`
}

// 输出
type MemorySearchOutput struct {
    Hits []MemoryHit `json:"hits"`
}
type MemoryHit struct {
    MemID      string   `json:"mem_id"`
    Facts      []string `json:"facts"`        // 记忆内容
    Category   string   `json:"category"`
    Confidence float64  `json:"confidence"`
    Sources    []int    `json:"source_steps"` // 可溯源 step
    Sessions   []string `json:"source_sessions"`
}
```

**实现路径**：
```
query → FusionRetriever.SearchLongMem(query, category, limit)
      → 三路融合（VSS 0.50 + BM25 0.30 + Recency 0.20）
      → 过滤 confidence ≥ 0.5（低于此值的记忆不参与检索）
      → 返回 hits
```

**与 context_search 的区别**：

| 维度 | context_search | memory_search |
|------|---------------|---------------|
| 范围 | 当前 session 内的 step | 跨 session 长期记忆 |
| 数据源 | .lN.jsonl + refs.jsonl | longmem.jsonl / PostgreSQL longterm_memories |
| 副作用 | 无 | 命中记忆的 last_validated_step 更新 + confidence += 0.02 |
| 注册条件 | mode ≠ passthrough | mode ≠ passthrough 且 longmem 已启用 |

**错误处理**：
- longmem 存储不可用 → 返回空 hits + 提示 "长期记忆未启用"
- 无命中 → 返回空 hits + 提示 "无匹配长期记忆，尝试 context_search 检索当前 session"

**副作用说明**：memory_search 命中会轻微提升 confidence（+0.02），表示该记忆仍被活跃使用。这与 context_expand 更新 refs 的设计一致——被检索 = 有价值。

---

## 八-C、L2 事实 → 长期记忆提取

### 8C.1 动机

.l2.jsonl 中的高频事实（ref_count≥3, strength≥1.3）事实上承担了"session 内长期记忆"的角色。但这些事实随 session 结束而沉寂。需要一条**提升管道**将跨 session 有价值的事实提取为持久化长期记忆。

### 8C.2 提取管道

```
.l2.jsonl 事实 ──→ 提升评估 ──→ 长期记忆存储
                    │
                    ├─ 条件 1: ref_count ≥ 5（跨多个 task_group 被引用）
                    ├─ 条件 2: 关联 ≥ 2 个不同 task_group
                    ├─ 条件 3: 事实类型为"配置值/路径/决策结论"（非临时状态）
                    └─ 条件 4: 未被后续事实覆盖/否定
```

### 8C.3 长期记忆存储格式

```jsonl
// {uuid}.longmem.jsonl（或 PostgreSQL longterm_memories 表）
{"mem_id":"m_001","facts":["REDIS_HOST=localhost","REDIS_PORT=6379"],"source_steps":[3,7,42],"source_sessions":["sess_abc"],"category":"config","created_at_step":120,"last_validated_step":350,"confidence":0.92}
{"mem_id":"m_002","facts":["redis_config.py 为独立配置文件","旧 config.py REDIS_HOST 已废弃"],"source_steps":[7,55],"source_sessions":["sess_abc","sess_def"],"category":"decision","created_at_step":200,"last_validated_step":400,"confidence":0.88}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| mem_id | string | 全局唯一记忆 ID |
| facts | []string | 记忆内容（从 .l2.jsonl 提升） |
| source_steps | []int | 来源 step（可溯源） |
| source_sessions | []string | 来源 session（跨 session 标记） |
| category | string | config / decision / constraint / pattern |
| created_at_step | int | 提升时的 step |
| last_validated_step | int | 最后被验证仍有效的 step |
| confidence | float | 置信度（初始 0.8，每次被引用 +0.04，被否定 → 0） |

### 8C.4 长期记忆的使用

```
BuildContext 区域 2（高频事实注入）扩展：

  来源 1: 当前 session .l2.jsonl 中 ref_count≥3 的事实（原有）
  来源 2: longmem 中 category 匹配当前任务 + confidence≥0.7 的记忆（新增）

  渲染格式：
  [facts | session | sources: step_42,55]
    • config.py:12 REDIS_HOST=localhost
  [/facts]
  [facts | longterm | mem:m_001 | conf:0.92]
    • REDIS_HOST=localhost (跨 session 验证)
  [/facts]
```

### 8C.5 提升触发时机

| 触发 | 条件 | 动作 |
|------|------|------|
| 全局自省（§4C.2） | 事实满足 8C.2 全部条件 | 写入 longmem + 标记 source_steps |
| session 结束 | session close 时扫描 | 将当前 session 中 ref_count≥5 的事实候选提升 |
| 新 session 启动 | 加载 longmem | 按 category + 当前任务关键词筛选注入区域 2 |
| 事实验证 | 长期记忆被 §N 引用 | last_validated_step 更新 + confidence += 0.04 |
| 记忆否定 | LLM 输出"§N 已过期/不正确" | confidence = 0 → 下次自省时移除 |

### 8C.6 与 AgentScope 长期记忆的对标

| 维度 | AgentScope (Mem0/ReMe/Agentic) | 本方案 (longmem) |
|------|------|------|
| 写入方式 | LLM 主动 add_memory / auto_memory | 框架自动提升（无需 LLM 额外调用） |
| 来源 | 对话原文 | .l2.jsonl 压缩事实（已去噪） |
| 检索 | 向量/关键词 | 三路融合（复用 FusionRetriever） |
| 跨 session | ✓ | ✓（source_sessions 多值） |
| 可溯源 | 弱（mem0 无 source step） | 强（source_steps → L0 原文可 expand） |
| 置信度衰减 | 无 | confidence 机制 + 否定清零 |

---

## 九、实现里程碑

| 阶段 | 内容 | 依赖 |
|------|------|------|
| **M1** | 数据模型(StepRecord/RefRecord/L1-L3Record) + StepStore 接口 + JSONL 后端(.jsonl/.refs/.l1/.l2/.l3) | 无 |
| **M2** | ContextManager 接口 + Registry + Passthrough + Config | M1 |
| **M3** | Session 兼容层（双写 + 映射 + 回灌 SetChatHistory） | M1+M2 |
| **M4** | Hybrid 骨架：Ingest + BuildContext 五区拼接 + effective_decay | M3 |
| **M5** | CompressModelChain（小模型+降级+机械）+ 提示词模板 + 8 步流程 | M4 |
| **M6** | Redis 缓存层 + 三路融合 + Embedder | M4 |
| **M7** | context_search/expand/surround 工具（含 schema 注册 + 错误降级 + hint 反馈） | M6 |
| **M8** | 高频事实注入（.l2.jsonl 筛选 + BuildContext 区域2） | M5 |
| **M9** | 空闲巩固 + task_group 归档 + 五层完整 | M5+M6 |
| **M10** | SQLite / PostgreSQL 后端 | M1 |
| **M11** | L2 事实 → 长期记忆提升管道（longmem.jsonl + 区域2 扩展 + 置信度）+ memory_search 工具 | M8+M9 |
| **M12** | BuildContext 增量缓存（rendered cache + JSON list 直传加速） | M4 |

---

## 十、待评审确认

| # | 决策点 | 当前方案 | 备选 |
|---|--------|---------|------|
| 1 | 小模型超时阈值 | 5s | 可配置 |
| 2 | 机械降级 L3 无意图总结 | 只生成结构体 | 用模板填充 |
| 3 | tail_keep_steps 默认值 | 5 步 | 按 token 预算动态 |
| 4 | BuildContext 渲染前缀 [step_N\|type\|Lx] | 有前缀（方便引用追踪） | 无前缀（省 token） |
| 5 | L4 丢弃后原文是否保留 | 保留（L0 .jsonl 不删） | 物理删除（不可恢复） |
| 6 | 模式切换时 FullContext 回放 | 一次性生成 L0 .jsonl | 懒加载（按需） |
| 7 | .l2.jsonl 的 facts 字段格式 | []string 数组 | 单字符串（\n 分隔） |
 8 | 高频事实注入阈值 | ref_count≥3 且 strength≥1.3 | 可配置 |
| 9 | 工具注册方式 | 内置 tool.Registry（mode≠passthrough 时自动注册） | 按需手动注册 |
| 10 | expand 副作用是否更新 refs | 是（等同 §N 引用） | 只读不更新 |

---

## 十一、与 AgentScope 完善度对比

### 11.1 架构层面对比

| 维度 | AgentScope（当前） | InferGlow contextmgr（本规格） | 评价 |
|------|-------------------|-------------------------------|------|
| **压缩粒度** | 整个 context → 单次 structured summary（5 字段） | 每 step 独立压缩，L0→L1→L2→L3→L4 五级 | 本方案粒度更细，信息保留更精确 |
| **压缩模型** | 仅主模型（generate_structured_output） | 小模型→主模型→机械降级 三级链路 | 本方案成本更低、鲁棒性更强 |
| **触发机制** | 单一阈值（trigger_ratio=0.8） | 五层防线（per-step decay + 空闲巩固 + 部分拼接 + 全局 + 警戒线） | 本方案更渐进、更少突变 |
| **存储持久化** | 纯内存（state.context）+ offloader 文件 | JSONL/SQLite/PostgreSQL + Redis 缓存 | 本方案支持跨 session 恢复 |
| **检索能力** | RAG: 纯向量（KnowledgeBase.search） | 三路融合（VSS + BM25 + Recency） | 本方案召回率更高 |
| **工具暴露** | search_knowledge / memory_search / add_memory | context_search / context_expand / context_surround | 功能对等，本方案多了"展开"和"环顾" |
| **中间件钩子** | 7 个生命周期钩子（onion pattern） | 5 个集成点（Ingest/BuildContext/compress/system_prompt/tool_result） | AgentScope 钩子更通用；本方案专用但足够 |
| **LLM 感知** | HintBlock 注入（static 模式） | CONTEXT_PROTOCOL 宪法 + HintBlock 动态 + §N 引用协议 | 本方案 LLM 对压缩状态的感知更完整 |
| **长期记忆** | Agentic/Mem0/ReMe 三选一 | L2 事实自动提升 → longmem（§八-C） | 本方案无需 LLM 额外调用，自动且可溯源 |

### 11.2 本方案超越 AgentScope 的关键点

1. **渐进压缩 vs 断崖压缩**：AgentScope 一旦触发就把旧 context 全部压成一段 summary，信息损失大且不可逆。本方案按 step 逐级降，高频引用的 step 可长期保持 L0/L1。
2. **成本敏感**：小模型（3B）做 90% 的压缩工作，主模型只在降级时介入。AgentScope 每次压缩都消耗主模型 structured output。
3. **可审计性**：每级压缩产物独立文件存储，可回溯任意 step 的任意级别。AgentScope 压缩后原文仅通过 offloader 文件保留。
4. **LLM 主动参与**：§N 引用协议让 LLM 主动标记重要信息，影响后续压缩决策。AgentScope 的 LLM 对压缩完全无感知。

### 11.3 AgentScope 优于本方案的点（需关注）

1. **中间件通用性**：AgentScope 的 7 钩子 onion pattern 允许任意第三方扩展。本方案的集成点是硬编码的，扩展性弱。
   - **缓解**：M2 的 Registry 设计预留 `Hook` 接口，后续可演进为 middleware chain。
2. **长期记忆写入方式**：AgentScope 依赖 LLM 主动调用 add_memory 或 auto_memory 提取。本方案由框架根据 refs 统计自动提升，无需额外 LLM 调用。
   - **注意**：自动提升可能遗漏"仅出现一次但极重要"的事实。缓解：LLM 可通过 §N 引用 + 宪法提示词中的指引主动标记"此事实需长期保留"。
3. **工具权限模型**：AgentScope 有 `check_permissions` + `PermissionDecision`。本方案的工具自动允许。
   - **缓解**：上下文工具只读（expand 有 refs 副作用但无破坏性），自动允许合理。若后续加入写操作工具再引入权限。
4. **结构化输出保障**：AgentScope 用 `generate_structured_output` + pydantic schema 确保压缩结果格式。本方案用正则校验。
   - **缓解**：机械降级不依赖 LLM 格式；小模型/主模型的 validate() 可扩展为 structured output。
