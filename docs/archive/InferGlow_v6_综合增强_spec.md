# InferGlow v6 综合增强 Spec — 七领域 + 上下文管理校准

> 基准版本：InferGlow v5（对比覆盖率 72%）
> 目标版本：v6（目标覆盖率 ≥ 82%，超越 AgentScope 70%）
> 分析日期：2026-07-29
> 参考源：
> - `agent-framework-comparison.md`（四框架 87 项对标）
> - Reasonix `上下文管理增强_task-44b_chp.md`（甜点区/分级压缩/宪法区）
> - Reasonix `maybeSliceCompact_实现_task-44b.md`（三问重组/预热排期/容忍上调）
> - Agently v4.1.4.3 源码（Python 参考实现）
> - OTel GenAI Semantic Conventions（最新 Development 规范）

---

## 一、总体策略与依赖关系

```
Phase 0: 上下文管理校准 ←── 从 Reasonix 移植甜点区+预热+三问重组
    │
    ├── Phase 1: REST API Server（必选）←── 独立。**server/ 已是独立 Go module，v6 做多租户+流式增强**
    │
    ├── Phase 2: OTel GenAI 全集成 ←── 独立，可并行。**telemetry/ 已有 OTel 基础**
    │
    ├── Phase 3: Multi-Agent 能力 ←── 依赖新增 `team/` 包（P0-3），可并行
    │
    ├── Phase 4: RAG 文档加载+分割 ←── **rag/ 已是独立 Go module（6 loader + 3 splitter），v6 增强 PDF loader + embedding 注册表**
    │
    ├── Phase 5: 可选 Rerank 支持 ←── **rerank/ 已是独立 Go module（Cohere + LLM + fallback），v6 增加 cross-encoder ONNX**
    │
    ├── Phase 6: 沙箱增强 ←── 独立。7+1 沙箱，v6 补齐 Windows 三子后端
    │
    └── Phase 7: MCP 三传输优化 ←── 独立，可并行。已有基础 consumer，v6 补齐 stdio+SSE+StreamableHTTP + server 端
```

Phase 0 必须先做（其他 Phase 可能引用 contextmgr 接口），Phase 1-7 可并行开发。

---

## Phase 0: 上下文管理校准（从 Reasonix 移植）

### 0.1 现状分析

**InferGlow v5 contextmgr 已有**（对比文件 §模块5）：
- Session.FullContext、ContextWindow+MaxLength、SummaryMemory（L1/L2 压缩）
- contextmgr 五层防线（per-step → 空闲巩固 → 部分拼接 → 全局 → 警戒线）
- contextmgr 三路融合召回（VSS 0.50 + BM25 0.30 + Recency 0.20）
- contextmgr longmem 自动提升（L2 事实 → 长期记忆，无需 LLM）
- 4 存储后端（JSONL/SQLite/PostgreSQL/Redis）

**InferGlow v5 contextmgr 缺失**（Reasonix 创新点）：
1. ❌ **甜点区阈值**（SweetSpotTokens）——无"低于阈值不启动分级压缩"的 passthrough 模式
2. ❌ **预热排期**（≥ 80% 甜点区时异步预压缩老 step）
3. ❌ **sliceCompact 三问决策**（Q1 宪法追加 / Q2 任务摘要转移 / Q3 step 保留决策）
4. ❌ **Zone 0.5 宪法区**（可动态追加的操作禁止/约束/提示词更新）
5. ❌ **头部区改写**（task_group 转移时重写 Zone 1）
6. ❌ **context_reorganize 工具**（LLM 主动触发三问重组）
7. ❌ **动态甜点区容忍上调**（LLM 引用 ≥5 个不同 step_id 时自动扩大甜点区 10%）
8. ❌ **System Prompt 上下文管理模板按条件注入**

### 0.2 移植设计

#### 0.2.1 甜点区阈值 + Passthrough 模式

**配置入口** — `internal/config/config.go`：

```go
// AgentConfig 新增
type AgentConfig struct {
    // ... 现有字段 ...

    // SweetSpotTokens 甜点区阈值（token 数）。
    // 当 prompt tokens 低于此值时，保持原始压缩逻辑，不启动分级改写。
    // 目的：保护 prefix cache 命中率。
    // 0 或负值 = 始终启用分级压缩（当前行为）。
    // 推荐默认值：256000（256K tokens）
    SweetSpotTokens int `toml:"sweet_spot_tokens"`
}
```

**甜点区判断逻辑** — `internal/contextmgr/hybrid.go`：

```go
func (h *HybridManager) perStepDecay(stepID int) error {
    // 甜点区判断：低于阈值时不触发分级压缩升级
    if h.sweetSpotTokens > 0 && h.estimateTotalTokens() < h.sweetSpotTokens {
        // Passthrough 模式：仅更新 ref_count，不提升 level
        // 保持所有 step 在 L0（原文），最大化 prefix cache 命中
        return nil
    }
    // 超出甜点区：启用 effective_decay 分级升级
    return h.effectiveDecayUpgrade(stepID)
}
```

**甜点区语义说明**：
- 甜点区 ≠ 降低 thinking effort
- 甜点区 = 控制是否启动分级历史改写
- 甜点区内 → 原有压缩逻辑（高缓存命中）
- 甜点区外 → 启用分级压缩（L1-L4）

#### 0.2.2 预热排期

**新增字段** — `internal/contextmgr/hybrid.go`：

```go
type HybridManager struct {
    // ... 现有字段 ...
    sweetSpotTokens     int
    sweetSpotOriginal   int           // 原始值，用于容忍上调上限
    sweetSpotTolerance  float64       // 当前上调比例
    warmupPending       atomic.Bool   // 预热排期互斥
    warmupRatio         float64       // 默认 0.8
}
```

**预热触发** — `Ingest()` 末尾：

```go
func (h *HybridManager) Ingest(step StepRecord) error {
    // ... 现有 Ingest 逻辑 ...

    // 预热排期：≥ 80% 甜点区时异步预压缩
    if h.sweetSpotTokens > 0 && !h.warmupPending.Load() {
        ratio := float64(h.estimateTotalTokens()) / float64(h.sweetSpotTokens)
        if ratio >= h.warmupRatio {
            h.warmupPending.Store(true)
            go h.warmupCompress(context.Background())
        }
    }
    return nil
}

func (h *HybridManager) warmupCompress(ctx context.Context) {
    defer h.warmupPending.Store(false)

    ids, _ := h.store.AllActiveStepIDs()
    // 跳过 tail 区（最近 N 步保持 L0），压缩 Process 区的老 step
    tailStart := len(ids) - h.cfg.TailKeepSteps
    if tailStart < 0 {
        tailStart = 0
    }

    // 第一轮：最老的 step → L1（去噪）
    for _, id := range ids[:tailStart] {
        ref, _ := h.store.GetRef(id)
        if ref.Level >= 1 {
            continue
        }
        if err := h.compressEngine.CompressStep(ctx, id, 1); err != nil {
            continue // 单个失败不阻塞
        }
    }

    // 第二轮：再检查是否还需要→ L2（事实提取）
    // 仅在 L1 完成后 token 仍超阈值时才继续
    if float64(h.estimateTotalTokens())/float64(h.sweetSpotTokens) >= h.warmupRatio {
        for _, id := range ids[:tailStart] {
            ref, _ := h.store.GetRef(id)
            if ref.Level >= 2 {
                continue
            }
            h.compressEngine.CompressStep(ctx, id, 2)
        }
    }
}
```

#### 0.2.3 三问合一重组决策引擎（合并为单次 prefill）

> **设计决策**：独立三次 LLM 调用浪费 prefill（三次都要传宪法区 + step 状态表），合并为一次调用可节省 ~58% 输入 tokens。Q3 的 step_decisions 即使 200 个 step 也仅 ~12K chars，远在 flash 模型 8K 默认输出内。

**新建文件** — `internal/contextmgr/reorganize.go`：

```go
// ReorganizeDecision 三问合并的一次输出
type ReorganizeDecision struct {
    // Q1: 宪法区追加条目
    ConstitutionalAppend []string `json:"q1_constitutional_append"`

    // Q2: 新头部简述（空字符串 = 不需要改写）
    NewHeadSummary string `json:"q2_new_head_summary"`

    // Q3: step 保留决策
    StepDecisions []StepLevelDecision `json:"q3_step_decisions"`
}

type StepLevelDecision struct {
    StepID      int    `json:"step_id"`
    TargetLevel int    `json:"target_level"` // 0-3, -1=丢弃
    Reason      string `json:"reason"`       // 决策理由（≤60字）
}

// Reorganize 执行三问合并重组（单次 LLM 调用）
func (h *HybridManager) Reorganize(ctx context.Context, focus string) (*ReorganizeResult, error) {
    prompt := h.buildMergedReorganizePrompt(focus)

    // 单次调用压缩模型
    resp, err := h.compressEngine.Call(ctx, prompt)
    if err != nil {
        return nil, fmt.Errorf("reorganize call failed: %w", err)
    }

    decision, err := parseMergedResponse(resp)
    if err != nil {
        return nil, fmt.Errorf("parse merged response: %w", err)
    }

    var result ReorganizeResult
    if len(decision.ConstitutionalAppend) > 0 {
        result.ConstitutionalAdded = len(decision.ConstitutionalAppend)
        h.AppendConstitutional(decision.ConstitutionalAppend)
    }
    if decision.NewHeadSummary != "" {
        result.HeadRewritten = true
        h.RewriteHeadBuffer(decision.NewHeadSummary, fmt.Sprintf("reorg-%d", time.Now().Unix()))
    }
    if len(decision.StepDecisions) > 0 {
        n, _ := h.SetStepLevels(ctx, decision.StepDecisions)
        result.StepsAdjusted = n
    }
    return &result, nil
}

// buildMergedReorganizePrompt 构建合并三问重组 prompt（单次 prefill）
func (h *HybridManager) buildMergedReorganizePrompt(focus string) string {
    var sb strings.Builder
    sb.WriteString("你是上下文重组管理器。基于以下信息，一次性回答三个决策问题。\n\n")

    sb.WriteString("## 当前状态\n")
    sb.WriteString("### 宪法区（Zone 0.5）\n")
    sb.WriteString(h.getConstitutionalContent())
    sb.WriteString("\n### 头部简述（Zone 1）\n")
    sb.WriteString(h.getHeadBuffer())
    sb.WriteString("\n### Step 状态表\n")
    sb.WriteString(h.Status())
    if focus != "" {
        sb.WriteString(fmt.Sprintf("\n### 重组焦点\n%s\n", focus))
    }

    sb.WriteString(`
## 三问决策（请按顺序回答，以 JSON 格式输出全部三个答案）

Q1: 宪法区是否需要追加？
分析：当前任务是否产生了新的操作禁止、约束或提示词更新？
行动：若有，列出追加条目；若无，返回空数组。

Q2: 头部简述是否需要转移？
分析：当前任务焦点相比现有头部简述是否已发生显著转移？
行动：若有变化，输出新头部简述；若无变化，输出空字符串。

Q3: 每个 step 的保留/压缩等级？
对每个 step，根据其与当前焦点的相关性决策目标等级：
  L0=保留原文, L1=去噪, L2=事实提取, L3=掩码, -1=丢弃

输出格式（严格 JSON）：
{
  "q1_constitutional_append": ["条目1", "条目2"],
  "q2_new_head_summary": "新头部简述（无变化则为空字符串）",
  "q3_step_decisions": [
    {"step_id": 3, "target_level": 2, "reason": "已完成且不再相关"},
    {"step_id": 7, "target_level": 0, "reason": "当前焦点依赖此步骤结果"}
  ]
}

约束：
- q1_constitutional_append 必须是字符串数组，每个条目用中文简洁描述（≤80字）
- q2_new_head_summary 不得超过 200 字
- q3_step_decisions 必须覆盖所有活跃 step_id，不得遗漏
- 仅输出 JSON，不要包裹在 markdown 代码块中`)
    return sb.String()
}

// parseMergedResponse 解析合并的三问响应（含严格校验）
func parseMergedResponse(resp string) (*ReorganizeDecision, error) {
    // 1. 清理 markdown 代码块包裹（模型可能不遵守"不包裹"约束）
    cleaned := cleanJSONBlock(resp) // 去掉 ```json ... ``` 或 ``` ... ```

    // 2. JSON 解析
    var raw struct {
        Q1Append    []string `json:"q1_constitutional_append"`
        Q2Summary   string   `json:"q2_new_head_summary"`
        Q3Decisions []struct {
            StepID      int    `json:"step_id"`
            TargetLevel int    `json:"target_level"`
            Reason      string `json:"reason"`
        } `json:"q3_step_decisions"`
    }
    if err := json.Unmarshal([]byte(cleaned), &raw); err != nil {
        return nil, fmt.Errorf("invalid JSON: %w", err)
    }

    decision := &ReorganizeDecision{}

    // Q1 校验：每个条目 ≤80 字
    for i, entry := range raw.Q1Append {
        if len([]rune(entry)) > 80 {
            return nil, fmt.Errorf("q1 entry %d too long: %d chars (max 80)", i, len([]rune(entry)))
        }
    }
    decision.ConstitutionalAppend = raw.Q1Append

    // Q2 校验：≤200 字
    if len([]rune(raw.Q2Summary)) > 200 {
        return nil, fmt.Errorf("q2 summary too long: %d chars (max 200)", len([]rune(raw.Q2Summary)))
    }
    decision.NewHeadSummary = raw.Q2Summary

    // Q3 校验：每个 target_level 合法（范围 -1..3）
    for _, d := range raw.Q3Decisions {
        if d.TargetLevel < -1 || d.TargetLevel > 3 {
            return nil, fmt.Errorf("step %d: invalid target_level %d (range: -1..3)", d.StepID, d.TargetLevel)
        }
        if len([]rune(d.Reason)) > 60 {
            return nil, fmt.Errorf("step %d: reason too long: %d chars (max 60)", d.StepID, len([]rune(d.Reason)))
        }
        decision.StepDecisions = append(decision.StepDecisions, StepLevelDecision{
            StepID:      d.StepID,
            TargetLevel: d.TargetLevel,
            Reason:      d.Reason,
        })
    }
    return decision, nil
}
```

**Prefill 节省计算：**

| 场景 | 独立三问 | 合并一问 | 节省 |
|------|:------:|:------:|:----:|
| 输入 tokens（宪法区 500 + 状态表 800 + 指令 300） | 3 × 1600 = 4800 | 1 × 2000 = 2000 | **58%** |
| Prefill 耗时（flash, ~100K tok/s） | ~48ms | ~20ms | **58%** |
| LLM 调用次数 | 3 | 1 | **67%** |
| 思维连贯性 | 相互独立，可能矛盾 | 整体推理，一致性好 | ✅ |

#### 0.2.4 Zone 0.5 宪法区

**修改文件** — `internal/contextmgr/hybrid.go`：

```go
type HybridManager struct {
    // ... 现有字段 ...
    constitutionalEntries []string      // 宪法区动态条目
    constitutionalMu      sync.RWMutex
}

// AppendConstitutional 追加宪法区条目（线程安全）
func (h *HybridManager) AppendConstitutional(entries []string) {
    h.constitutionalMu.Lock()
    defer h.constitutionalMu.Unlock()
    h.constitutionalEntries = append(h.constitutionalEntries, entries...)
    // 触发缓存失效
    atomic.AddInt64(&h.renderVersion, 1)
}

// BuildContext 五区拼接中插入 Zone 0.5
func (h *HybridManager) BuildContext(windowTokens int) []ContextBlock {
    var blocks []ContextBlock

    // Zone 0: System Prompt（从外部注入，不在这里构建）
    // Zone 0.5: 宪法区（动态追加）
    h.constitutionalMu.RLock()
    entries := make([]string, len(h.constitutionalEntries))
    copy(entries, h.constitutionalEntries)
    h.constitutionalMu.RUnlock()

    if len(entries) > 0 {
        content := "<constitutional>\n"
        for _, e := range entries {
            content += "- " + e + "\n"
        }
        content += staticContextProtocol + "\n" // 静态模板
        content += "</constitutional>"
        blocks = append(blocks, ContextBlock{
            Zone:    "0.5-constitutional",
            Content: content,
            TokenEst: estimateTokens(content),
        })
    }

    // Zone 1: 头部简述
    // Zone 2: 压缩历史区
    // Zone 3: Tail 原文区
    // ... 现有逻辑 ...
    return blocks
}
```

**静态模板注入** — `internal/boot/boot.go`：

```go
// 在 memory.Compose() 之后、skill index 注入之前
if cfg.Agent.SweetSpotTokens > 0 || cfg.Agent.CompressModel != "" {
    sysPrompt += "\n\n" + contextmgr.SystemPromptHint()
}
```

**`internal/contextmgr/hint.go`**：

```go
func SystemPromptHint() string {
    return `<context-management>
当前上下文采用分级压缩管理。历史消息可能包含以下标记：
- [step_N|role|Lx] 表示第 N 步、角色 role、压缩级别 Lx（L0=原文, L1=去噪, L2=事实, L3=行为掩码）
- <compaction-summary> 包裹早期对话的单级摘要（甜点区内模式）
使用 context_search 工具在压缩历史中搜索关键词。
使用 context_expand 工具展开某个被压缩的 step 回原文。
压缩感知：
- 当上下文压力较高时，应主动精简输出
- 不要尝试在输出中复制分节标记格式
- 若发现信息不足（掩码太略），先调用 context_expand 再回答
</context-management>`
}
```

#### 0.2.5 头部区改写

```go
// RewriteHeadBuffer 替换 Zone 1 头部简述
func (h *HybridManager) RewriteHeadBuffer(newContent, newVersion string) {
    h.headMu.Lock()
    defer h.headMu.Unlock()

    // 归档旧头部
    h.archivedHeads = append(h.archivedHeads, ArchivedHead{
        Content:   h.headBuffer,
        Version:   h.headVersion,
        ArchivedAt: time.Now(),
    })

    // 替换
    h.headBuffer = newContent
    h.headVersion = newVersion
    atomic.AddInt64(&h.renderVersion, 1)
}
```

#### 0.2.6 context_reorganize 工具

**`internal/contextmgr/tools.go`** 新增：

```go
// ReorganizeTool LLM 主动触发上下文重组的工具
type ReorganizeTool struct {
    mgr *HybridManager
}

func (t ReorganizeTool) Name() string        { return "context_reorganize" }
func (t ReorganizeTool) Description() string {
    return "当上下文增长过大或任务焦点发生显著转移时，调用此工具重组上下文。" +
        "参数 focus 指定重组焦点（如 'redis config migration'），" +
        "aggressive 控制压缩激进程度（true=更激进压缩旧内容）。"
}
func (t ReorganizeTool) ReadOnly() bool      { return false }

func (t ReorganizeTool) Call(ctx context.Context, args map[string]any) (string, error) {
    focus := ""
    if f, ok := args["focus"].(string); ok {
        focus = f
    }
    aggressive := false
    if a, ok := args["aggressive"].(bool); ok {
        aggressive = a
    }

    result, err := t.mgr.Reorganize(ctx, focus)
    if err != nil {
        return "", fmt.Errorf("reorganize failed: %w", err)
    }

    return fmt.Sprintf(
        "上下文重组完成：宪法区追加 %d 条，头部改写=%v，step 调整 %d 个",
        result.ConstitutionalAdded,
        result.HeadRewritten,
        result.StepsAdjusted,
    ), nil
}
```

**boot.go 注册**：

```go
addContextTools := func() string {
    if contextToolsAdded {
        return "context tools are already enabled."
    }
    contextToolsAdded = true
    reg.Add(contextmgr.NewSearchTool(a.ctxManager))
    reg.Add(contextmgr.NewExpandTool(a.ctxManager))
    reg.Add(contextmgr.NewReorganizeTool(a.ctxManager)) // 新增
    return "enabled context_search, context_expand, context_reorganize."
}
```

#### 0.2.7 动态甜点区容忍上调（滑动比例 + 衰减回退）

> **设计变更**：原方案用绝对引用数（≥5）触发上调，在 1M 上下文窗口中阈值太低——高密度推理任务经常引用 10-20 个 step，导致频繁上调到上限。改为基于滑动窗口的引用率（引用数/活跃 step 数），并加入自然衰减机制。

```go
// HybridManager 新增字段
type HybridManager struct {
    // ... 现有字段 ...
    sweetSpotTokens     int
    sweetSpotOriginal   int           // 不变：原始配置值
    sweetSpotTolerance  float64       // 当前乘数（1.0 = 原始值）
    toleranceDecayRate  float64       // 每步衰减率（默认 0.98）

    // 滑动窗口追踪
    recentRefCounts     []int         // 最近 N 轮的引用计数（环形缓冲，N=10）
    recentRefIdx        int
}

// ProcessCitations 改进版——基于滑动引用率
func (h *HybridManager) ProcessCitations(outputText string) {
    // ... 现有 §N 解析逻辑 ...

    uniqueRefs := h.countUniqueStepRefs(outputText)
    totalActive := len(h.store.AllActiveStepIDs())
    if totalActive == 0 {
        return
    }

    // 记录到滑动窗口
    h.recentRefCounts[h.recentRefIdx] = uniqueRefs
    h.recentRefIdx = (h.recentRefIdx + 1) % len(h.recentRefCounts)

    // 计算滑动平均引用率
    avgRefs := h.averageRecentRefs()
    refRate := float64(avgRefs) / float64(totalActive)

    h.toleranceMu.Lock()
    defer h.toleranceMu.Unlock()

    switch {
    case refRate >= 0.30:
        // 高引用率：模型需要跨越大量历史，扩大甜点区 15%
        h.adjustTolerance(1.15)
    case refRate >= 0.15:
        // 中等引用率：温和扩大 8%
        h.adjustTolerance(1.08)
    case refRate < 0.05:
        // 低引用率：衰减回退 5%
        h.adjustTolerance(0.95)
    }
    // 0.05 ≤ refRate < 0.15：保持当前容忍度不变（稳定区）
}

// adjustTolerance 调整甜点区容忍度（确保在 [1.0, 1.5] × original 范围内）
func (h *HybridManager) adjustTolerance(factor float64) {
    newTolerance := h.sweetSpotTolerance * factor
    if newTolerance < 1.0 {
        newTolerance = 1.0
    }
    if newTolerance > 1.5 {
        newTolerance = 1.5
    }
    h.sweetSpotTolerance = newTolerance
    h.sweetSpotTokens = int(float64(h.sweetSpotOriginal) * newTolerance)
}

// decayTolerance 每步自动衰减（在 Ingest 末尾调用）
func (h *HybridManager) decayTolerance() {
    h.toleranceMu.Lock()
    defer h.toleranceMu.Unlock()

    if h.sweetSpotTolerance > 1.05 {
        h.sweetSpotTolerance *= h.toleranceDecayRate // 0.98 → 每步衰减 2%
        h.sweetSpotTokens = int(float64(h.sweetSpotOriginal) * h.sweetSpotTolerance)
    } else if h.sweetSpotTolerance > 1.0 {
        h.sweetSpotTolerance = 1.0 // 快速归位
        h.sweetSpotTokens = h.sweetSpotOriginal
    }
}

// averageRecentRefs 计算滑动窗口平均引用数
func (h *HybridManager) averageRecentRefs() float64 {
    sum := 0
    count := 0
    for _, v := range h.recentRefCounts {
        if v > 0 || count > 0 { // 初始化阶段部分填充
            sum += v
            count++
        }
    }
    if count == 0 {
        return 0
    }
    return float64(sum) / float64(count)
}
```

**为什么滑动比例优于绝对数量：**

```
场景 A: 100 个活跃 step，引用 10 个 → refRate=0.10 → 稳定区，不调整
场景 B: 10 个活跃 step，引用 5 个  → refRate=0.50 → 高引用，扩大 15%

原方案: A 和 B 都 ≥5 → 都上调 10%（不合理——A 只是正常引用密度）
新方案: A 不调整，B 扩大（符合直觉——小窗口密集引用 = 模型需要更大视野）
```

**推荐阈值（DeepSeek v4 pro/flash，1M 上下文窗口）：**

| 参数 | 推荐值 | 理由 |
|------|:------:|------|
| **sweet_spot_tokens** | **256,000** | 25% of 1M，足够覆盖绝大多数会话（约 170K 英文词 / 85K 中文词） |
| **prepare_ratio**（预热） | **0.80** | tokens ≥ 204,800 → 异步预热，预留 20% 余量 |
| **reorganize_ratio**（重组） | **1.00** | tokens > 256,000 → 触发三问重组 |
| **emergency_ratio**（紧急） | **1.20** | tokens > 307,200 → 激进压缩 |
| **tolerance_max** | **384K (1.5×)** | 上限不超 1.5× 原始值，避免压缩完全失效 |
| **tolerance_decay_rate** | **0.98/step** | 约 35 步无高引用后归位 |
| **compress_model** | **flash 首选，pro 降级** | flash 便宜 3×（$0.14 vs $0.435/M），pro 仅做 fallback |

### 0.3 Phase 0 文件清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `internal/config/config.go` | 修改 | 添加 `SweetSpotTokens` 字段 |
| `internal/contextmgr/hybrid.go` | 修改 | 甜点区判断 + 预热排期 + 宪法区字段 + 容忍上调字段 |
| `internal/contextmgr/tolerance.go` | **新建** | 滑动引用率计算 + `adjustTolerance` + `decayTolerance` |
| `internal/contextmgr/reorganize.go` | **新建** | 三问合一重组引擎 + `buildMergedReorganizePrompt` + `parseMergedResponse` |
| `internal/contextmgr/hint.go` | **新建** | `SystemPromptHint()` 静态模板 |
| `internal/contextmgr/head.go` | **新建** | 头部区读写 + `RewriteHeadBuffer` + 归档 |
| `internal/contextmgr/tools.go` | 修改 | 新增 `ReorganizeTool` |
| `internal/boot/boot.go` | 修改 | 按条件注入 `<context-management>` 模板；注册 `context_reorganize` 工具 |
| `internal/agent/compact.go` | 修改 | `maybeCompact` 拆分 Legacy/Tiered（已有 `maybeCompactTiered` 骨架） |

---

## Phase 1: REST API Server（必选）

### 1.1 现状（已审计修正）

- ✅ **`server/` 已是独立 Go module**（`github.com/inferglow/server`）
- ✅ `Server` struct + `NewServer()` + `AgentStore` 接口 + `AgentLike` 接口 已实现
- ✅ `cmd/inferglow-server/main.go` 入口已存在
- ✅ `handler/` + `middleware/` + `router.go` 已实现
- ✅ `TenantManager` 多租户基础已实现
- ⚠️ 缺少：SSE 流式响应、gRPC、WebSocket、OpenAPI spec 自动生成

### 1.2 目标

生产级 HTTP API 服务，对标 AgentScope 的 FastAPI 9 路由 + 多租户能力，但以 Go 惯用方式实现。

### 1.3 架构设计

```
cmd/inferglow-server/
└── main.go                     # 服务入口

internal/server/
├── server.go                   # Server 结构体 + 生命周期
├── router.go                   # 路由注册（基于 net/http 或轻量 router）
├── middleware/
│   ├── auth.go                 # API Key / Bearer Token 认证
│   ├── ratelimit.go            # 每租户速率限制
│   ├── cors.go                 # CORS 中间件
│   ├── logging.go              # 请求日志 + 审计
│   └── recovery.go             # Panic 恢复
├── handler/
│   ├── agent.go                # Agent CRUD + 调用
│   ├── session.go              # Session 管理
│   ├── tool.go                 # 工具列表/调用
│   ├── workflow.go             # TriggerFlow / DAG 编排
│   ├── memory.go               # 记忆 CRUD
│   ├── health.go               # 健康检查
│   └── streaming.go            # SSE 流式响应
├── tenant/
│   └── manager.go              # 多租户管理（租户隔离、配额）
└── openapi/
    └── spec.go                 # OpenAPI 3.0 规范生成
```

### 1.4 核心 API 路由

```go
// GET  /health                        — 健康检查
// GET  /v1/agents                     — 列出 Agent
// POST /v1/agents                     — 创建 Agent
// GET  /v1/agents/{id}                — 获取 Agent 详情
// POST /v1/agents/{id}/chat           — Agent 对话（非流式）
// POST /v1/agents/{id}/stream         — Agent 对话（SSE 流式）
// POST /v1/agents/{id}/chat/async     — Agent 异步对话（返回 task_id）
// GET  /v1/agents/{id}/tasks/{tid}    — 查询异步任务状态
// GET  /v1/sessions/{id}              — 获取 Session
// GET  /v1/sessions/{id}/messages     — 获取 Session 消息列表
// POST /v1/workflows/{name}/run       — 触发工作流
// GET  /v1/tools                      — 列出注册工具
// POST /v1/memories                   — 创建记忆
// GET  /v1/memories                   — 搜索记忆
// GET  /v1/tenants/{id}/usage         — 租户用量查询
// GET  /openapi.json                  — OpenAPI 规范
```

### 1.5 关键伪代码

```go
// server.go
type Server struct {
    cfg        *config.ServerConfig
    agentStore AgentStore           // Agent 持久化存储
    sessionMgr SessionManager       // Session 生命周期管理
    tenantMgr  *tenant.Manager      // 多租户
    router     *http.ServeMux       // 或 chi/gin
    otelTracer trace.Tracer         // OTel 集成
}

func NewServer(cfg *config.ServerConfig, deps ...any) (*Server, error) {
    s := &Server{cfg: cfg}
    s.setupMiddleware()
    s.registerRoutes()
    return s, nil
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
    // 1. 租户认证 + 速率限制
    tenant, err := s.tenantMgr.Authenticate(r)
    if err != nil { /* 401 */ }

    // 2. 解析请求
    var req ChatRequest
    json.NewDecoder(r.Body).Decode(&req)

    // 3. 创建 OTel span
    ctx, span := s.otelTracer.Start(r.Context(), "chat "+req.AgentID,
        trace.WithAttributes(
            attribute.String("gen_ai.operation.name", "chat"),
            attribute.String("gen_ai.agent.id", req.AgentID),
            attribute.String("gen_ai.agent.name", req.AgentName),
        ),
    )
    defer span.End()

    // 4. 执行 Agent
    agent := s.agentStore.Get(req.AgentID)
    resp, err := agent.Chat(ctx, req.Messages, req.Tools...)

    // 5. 记录用量
    s.tenantMgr.RecordUsage(tenant.ID, resp.Usage)

    // 6. 响应
    json.NewEncoder(w).Encode(resp)
}
```

### 1.6 验证标准

- [ ] `go build ./cmd/inferglow-server/` 编译通过
- [ ] `curl localhost:8080/health` 返回 200
- [ ] Agent 创建/对话/流式 SSE 全链路可用
- [ ] 多租户隔离：租户 A 不可见租户 B 的 Agent/Session
- [ ] OpenAPI spec 自动生成正确

---

## Phase 2: 完整 OpenTelemetry GenAI 语义属性集成

### 2.1 现状（已审计修正）

- ✅ `observability/otel/` 已有基础 OTel 集成（`Tracer`, `CallbacksTracer`）
- ✅ `agent.go` 已注入 `*otel.Tracer`
- ⚠️ 问题：**7 处直接 import `observability/otel`**——违反编排层不依赖能力层的原则（**P0-2 必须修**）
- ❌ 无 GenAI 语义属性（`gen_ai.operation.name`、`gen_ai.request.model` 等）
- ❌ 无 Agent span（`create_agent`、`invoke_agent`、`plan`）

### 2.2 目标

完整实现 OTel GenAI Semantic Conventions（Development），覆盖以下 span 类型：

| Span 类型 | `gen_ai.operation.name` | Span Kind | 优先级 |
|-----------|------------------------|-----------|--------|
| LLM 推理调用 | `chat` / `text_completion` / `generate_content` | CLIENT | P0 |
| Embedding 生成 | `embeddings` | CLIENT | P1 |
| 检索操作 | `retrieval` | INTERNAL | P1 |
| Agent 创建 | `create_agent` | CLIENT | P1 |
| Agent 调用 | `invoke_agent` | CLIENT/INTERNAL | P0 |
| 工作流调用 | `invoke_workflow` | INTERNAL | P1 |
| 规划阶段 | `plan` | INTERNAL | P1 |
| 工具执行 | `execute_tool` | INTERNAL | P0 |
| 记忆搜索 | `search_memory` | INTERNAL | P2 |
| 记忆 CRUD | `create_memory` / `update_memory` / `delete_memory` | INTERNAL | P2 |
| MCP 操作 | MCP 语义属性 | CLIENT | P2 |

### 2.3 包结构

```
internal/telemetry/
├── otel.go                  # OTel 初始化 + TracerProvider + OTLP exporter
├── genai/
│   ├── attributes.go        # gen_ai.* 属性常量 + 辅助函数
│   ├── inference.go         # LLM 推理 span 包装器
│   ├── agent.go             # Agent span（create/invoke/plan）
│   ├── tool.go              # Tool span（execute_tool）
│   ├── retrieval.go         # 检索 span
│   ├── memory.go            # 记忆 span
│   └── workflow.go          # 工作流 span
├── metrics/
│   ├── tokens.go            # Token 用量计数器和直方图
│   ├── latency.go           # 延迟直方图
│   └── requests.go          # 请求计数 + 错误率
└── events/
    └── genai_events.go      # gen_ai.system 事件（模型选择、fallback 等）
```

### 2.4 完整集成示例

#### 2.4.1 初始化

```go
// internal/telemetry/otel.go
package telemetry

import (
    "context"
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
    "go.opentelemetry.io/otel/sdk/resource"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"
    semconv "go.opentelemetry.io/otel/semconv/v1.30.0"
)

func InitTracerProvider(ctx context.Context, cfg TelemetryConfig) (*sdktrace.TracerProvider, error) {
    // OTLP gRPC exporter
    exporter, err := otlptracegrpc.New(ctx,
        otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint),
        otlptracegrpc.WithInsecure(),
    )
    if err != nil {
        return nil, err
    }

    // Resource 标识服务
    res, err := resource.New(ctx,
        resource.WithAttributes(
            semconv.ServiceName("inferglow"),
            semconv.ServiceVersion("6.0.0"),
        ),
    )

    tp := sdktrace.NewTracerProvider(
        sdktrace.WithBatcher(exporter),
        sdktrace.WithResource(res),
        sdktrace.WithSampler(sdktrace.AlwaysSample()), // 或基于 gen_ai 属性的采样
    )
    otel.SetTracerProvider(tp)
    return tp, nil
}
```

#### 2.4.2 LLM 推理 span（核心 P0）

```go
// internal/telemetry/genai/inference.go
package genai

import (
    "context"
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/attribute"
    "go.opentelemetry.io/otel/codes"
    "go.opentelemetry.io/otel/trace"
)

// InferenceSpan 封装一次 LLM 推理调用的完整 OTel 语义
type InferenceSpan struct {
    span  trace.Span
    start time.Time
}

// StartInferenceSpan 在发起 LLM 调用前创建 span
//
// 对应 OTel GenAI spec: gen_ai.inference.client span
// Span name: "chat {model}" 或 "text_completion {model}"
func StartInferenceSpan(ctx context.Context, opts InferenceOpts) (context.Context, *InferenceSpan) {
    tracer := otel.Tracer("inferglow/model")

    spanName := fmt.Sprintf("%s %s", opts.OperationName, opts.Model)
    ctx, span := tracer.Start(ctx, spanName,
        trace.WithSpanKind(trace.SpanKindClient),
        trace.WithAttributes(
            // Required
            attribute.String("gen_ai.operation.name", opts.OperationName),    // "chat"
            attribute.String("gen_ai.provider.name", opts.ProviderName),      // "openai"

            // Conditionally Required
            attribute.String("gen_ai.conversation.id", opts.ConversationID),
            attribute.String("gen_ai.request.model", opts.Model),
            attribute.Bool("gen_ai.request.stream", opts.Streaming),
            attribute.Bool("gen_ai.conversation.compacted", opts.IsCompacted),

            // Recommended
            attribute.Int("gen_ai.request.max_tokens", opts.MaxTokens),
            attribute.Float64("gen_ai.request.temperature", opts.Temperature),
            attribute.Float64("gen_ai.request.top_p", opts.TopP),
            attribute.String("gen_ai.request.reasoning.level", opts.ReasoningLevel),
        ),
    )

    return ctx, &InferenceSpan{span: span, start: time.Now()}
}

// RecordSuccess 记录成功响应，填充所有 Recommended 属性
func (s *InferenceSpan) RecordSuccess(resp InferenceResponse) {
    s.span.SetAttributes(
        // Response 属性
        attribute.String("gen_ai.response.id", resp.ID),
        attribute.String("gen_ai.response.model", resp.Model),
        attribute.StringSlice("gen_ai.response.finish_reasons", resp.FinishReasons),

        // Usage 属性
        attribute.Int("gen_ai.usage.input_tokens", resp.Usage.InputTokens),
        attribute.Int("gen_ai.usage.output_tokens", resp.Usage.OutputTokens),
        attribute.Int("gen_ai.usage.reasoning.output_tokens", resp.Usage.ReasoningTokens),
        attribute.Int("gen_ai.usage.cache_read.input_tokens", resp.Usage.CacheReadTokens),
        attribute.Int("gen_ai.usage.cache_creation.input_tokens", resp.Usage.CacheCreationTokens),

        // Streaming 指标
        attribute.Float64("gen_ai.response.time_to_first_chunk",
            resp.TimeToFirstChunk.Seconds()),
    )

    // Opt-In 属性（仅在启用时记录）
    if resp.InputMessages != nil {
        inputJSON, _ := json.Marshal(resp.InputMessages)
        s.span.SetAttributes(attribute.String("gen_ai.input.messages", string(inputJSON)))
    }
    if resp.OutputMessages != nil {
        outputJSON, _ := json.Marshal(resp.OutputMessages)
        s.span.SetAttributes(attribute.String("gen_ai.output.messages", string(outputJSON)))
    }
    if resp.ToolDefinitions != nil {
        toolsJSON, _ := json.Marshal(resp.ToolDefinitions)
        s.span.SetAttributes(attribute.String("gen_ai.tool.definitions", string(toolsJSON)))
    }

    s.span.SetStatus(codes.Ok, "")
    s.span.End()
}

// RecordError 记录错误，包含 gen_ai 特定错误信息
func (s *InferenceSpan) RecordError(err error) {
    s.span.SetAttributes(
        attribute.String("error.type", classifyGenAIError(err)),
    )
    s.span.SetStatus(codes.Error, err.Error())
    s.span.End()
}
```

#### 2.4.3 Agent span

```go
// internal/telemetry/genai/agent.go

// StartInvokeAgentSpan 创建 invoke_agent span
//
// 对应 OTel spec: gen_ai.invoke_agent.client / gen_ai.invoke_agent.internal
// Span name: "invoke_agent {agent_name}"
func StartInvokeAgentSpan(ctx context.Context, opts AgentOpts) (context.Context, *InvokeAgentSpan) {
    tracer := otel.Tracer("inferglow/agent")

    spanName := fmt.Sprintf("invoke_agent %s", opts.AgentName)
    ctx, span := tracer.Start(ctx, spanName,
        trace.WithSpanKind(trace.SpanKindInternal),
        trace.WithAttributes(
            attribute.String("gen_ai.operation.name", "invoke_agent"),
            attribute.String("gen_ai.agent.name", opts.AgentName),
            attribute.String("gen_ai.agent.id", opts.AgentID),
            attribute.String("gen_ai.agent.description", opts.Description),
            attribute.String("gen_ai.conversation.id", opts.SessionID),
        ),
    )
    return ctx, &InvokeAgentSpan{span: span}
}

// StartPlanSpan 创建 plan span（规划阶段）
// Span name: "plan {agent_name}" 或 "plan"
func StartPlanSpan(ctx context.Context, agentName string) (context.Context, trace.Span) {
    tracer := otel.Tracer("inferglow/agent")
    spanName := "plan"
    if agentName != "" {
        spanName = "plan " + agentName
    }
    ctx, span := tracer.Start(ctx, spanName,
        trace.WithSpanKind(trace.SpanKindInternal),
        trace.WithAttributes(
            attribute.String("gen_ai.operation.name", "plan"),
            attribute.String("gen_ai.agent.name", agentName),
        ),
    )
    return ctx, span
}
```

#### 2.4.4 Tool span

```go
// internal/telemetry/genai/tool.go

// StartExecuteToolSpan 创建 execute_tool span
//
// Span name: "execute_tool {tool_name}"
func StartExecuteToolSpan(ctx context.Context, opts ToolOpts) (context.Context, *ExecuteToolSpan) {
    tracer := otel.Tracer("inferglow/tool")

    spanName := fmt.Sprintf("execute_tool %s", opts.ToolName)
    ctx, span := tracer.Start(ctx, spanName,
        trace.WithSpanKind(trace.SpanKindInternal),
        trace.WithAttributes(
            // Required
            attribute.String("gen_ai.operation.name", "execute_tool"),
            attribute.String("gen_ai.tool.name", opts.ToolName),

            // Conditionally Required
            attribute.String("gen_ai.agent.name", opts.AgentName),

            // Recommended
            attribute.String("gen_ai.tool.call.id", opts.CallID),
            attribute.String("gen_ai.tool.description", opts.Description),
            attribute.String("gen_ai.tool.type", opts.ToolType), // "function" | "extension" | "datastore"
        ),
    )
    return ctx, &ExecuteToolSpan{span: span, start: time.Now()}
}

func (s *ExecuteToolSpan) RecordResult(resultJSON string, err error) {
    if err != nil {
        s.span.SetAttributes(attribute.String("error.type", classifyError(err)))
        s.span.SetStatus(codes.Error, err.Error())
    } else {
        // Opt-In: 记录工具调用结果
        s.span.SetAttributes(attribute.String("gen_ai.tool.call.result", resultJSON))
        s.span.SetStatus(codes.Ok, "")
    }
    s.span.End()
}
```

#### 2.4.5 在 ModelRequester 中集成

```go
// internal/model/requester.go — 实际集成点

func (r *ModelRequester) Stream(ctx context.Context, req ModelRequest) (<-chan StreamChunk, error) {
    // 创建 OTel inference span
    ctx, inferSpan := genai.StartInferenceSpan(ctx, genai.InferenceOpts{
        OperationName:  "chat",
        ProviderName:   r.providerName,    // "deepseek"
        Model:          req.Model,
        ConversationID: req.SessionID,
        Streaming:      true,
        IsCompacted:    req.IsCompacted,
        MaxTokens:      req.MaxTokens,
        Temperature:    req.Temperature,
        TopP:           req.TopP,
        ReasoningLevel: req.ReasoningEffort,
    })

    // 执行实际调用
    ch, err := r.provider.Stream(ctx, req)
    if err != nil {
        inferSpan.RecordError(err)
        return nil, err
    }

    // 包装 channel 以在结束时记录 span
    return wrapStreamWithTelemetry(ctx, ch, inferSpan), nil
}

// wrapped channel 在收到 EOF 或 error 时记录 span
func wrapStreamWithTelemetry(ctx context.Context, ch <-chan StreamChunk, span *genai.InferenceSpan) <-chan StreamChunk {
    out := make(chan StreamChunk)
    var (
        firstChunk   bool
        firstChunkAt time.Time
        totalInput   int
        totalOutput  int
        totalReason  int
        finishReasons []string
    )
    startTime := time.Now()

    go func() {
        defer close(out)
        for chunk := range ch {
            // 首 chunk 延迟
            if !firstChunk {
                firstChunk = true
                firstChunkAt = time.Now()
            }
            // 累积 token 计数
            if chunk.Usage != nil {
                totalInput = chunk.Usage.InputTokens
                totalOutput = chunk.Usage.OutputTokens
                totalReason = chunk.Usage.ReasoningTokens
            }
            if chunk.FinishReason != "" {
                finishReasons = append(finishReasons, chunk.FinishReason)
            }
            out <- chunk
        }
        // Channel 关闭 = 流结束，记录 span
        span.RecordSuccess(genai.InferenceResponse{
            Usage: genai.Usage{
                InputTokens:     totalInput,
                OutputTokens:    totalOutput,
                ReasoningTokens: totalReason,
            },
            FinishReasons:      finishReasons,
            TimeToFirstChunk:   firstChunkAt.Sub(startTime),
        })
    }()
    return out
}
```

#### 2.4.6 Metrics 示例

```go
// internal/telemetry/metrics/tokens.go
package metrics

import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/metric"
)

var (
    meter = otel.Meter("inferglow")

    // TokenUsage 计数器
    TokenUsage, _ = meter.Int64Counter(
        "gen_ai.usage.input_tokens",
        metric.WithDescription("Number of input tokens used"),
    )

    // LLM 调用延迟直方图
    LLMLatency, _ = meter.Float64Histogram(
        "gen_ai.client.operation.duration",
        metric.WithDescription("GenAI operation duration in seconds"),
        metric.WithUnit("s"),
    )

    // 工具调用计数
    ToolCalls, _ = meter.Int64Counter(
        "gen_ai.tool.call.count",
        metric.WithDescription("Number of tool calls"),
    )
)
```

#### 2.4.7 完整 trace 示例（Jaeger 视角）

一次 Agent 推理在 Jaeger 中看到的 trace 树：

```
invoke_agent "Math Tutor"                           [INTERNAL, 5.2s]
├── plan "Math Tutor"                               [INTERNAL, 0.3s]
│   └── chat deepseek-v4                            [CLIENT, 0.25s]
│       ├── gen_ai.operation.name = "chat"
│       ├── gen_ai.provider.name = "deepseek"
│       ├── gen_ai.request.model = "deepseek-v4"
│       ├── gen_ai.usage.input_tokens = 1200
│       ├── gen_ai.usage.output_tokens = 80
│       └── gen_ai.request.reasoning.level = "high"
├── chat deepseek-v4                                [CLIENT, 2.1s]
│   ├── gen_ai.operation.name = "chat"
│   ├── gen_ai.conversation.compacted = true       ← 上下文已压缩
│   ├── gen_ai.usage.input_tokens = 8500
│   ├── gen_ai.usage.output_tokens = 150
│   └── gen_ai.tool.definitions = [...]             ← 工具定义
├── execute_tool "calculator"                       [INTERNAL, 0.05s]
│   ├── gen_ai.operation.name = "execute_tool"
│   ├── gen_ai.tool.name = "calculator"
│   ├── gen_ai.tool.type = "function"
│   ├── gen_ai.tool.call.id = "call_abc123"
│   └── gen_ai.tool.call.arguments = {"expr":"2+3*4"}
├── chat deepseek-v4                                [CLIENT, 1.5s]
│   ├── gen_ai.operation.name = "chat"
│   ├── gen_ai.usage.input_tokens = 3800
│   ├── gen_ai.usage.output_tokens = 200
│   └── gen_ai.response.finish_reasons = ["stop"]
└── execute_tool "read_file"                        [INTERNAL, 0.02s]
    ├── gen_ai.tool.name = "read_file"
    ├── gen_ai.tool.type = "function"
    └── error.type = "file_not_found"
```

### 2.5 配置入口

```toml
# inferglow.example.toml
[telemetry]
enabled = true
otlp_endpoint = "localhost:4317"          # OTLP gRPC
# otlp_endpoint = "localhost:4318"        # OTLP HTTP
protocol = "grpc"
sample_rate = 1.0                        # 采样率
record_input_messages = false            # Opt-In: 记录输入消息
record_output_messages = false           # Opt-In: 记录输出消息
record_tool_results = false              # Opt-In: 记录工具调用结果
max_attribute_length = 8192              # 属性值最大长度（截断）

[telemetry.metrics]
enabled = true
export_interval = "15s"
```

### 2.6 验证标准

- [ ] `go.opentelemetry.io/otel` 依赖正确引入
- [ ] 启动 Jaeger/OTLP collector → 所有 span 类型可见
- [ ] Agent 对话一次 → trace 树包含 `invoke_agent` → `plan` → `chat` → `execute_tool` 完整链路
- [ ] `gen_ai.operation.name`、`gen_ai.request.model`、`gen_ai.usage.*` 属性正确填充
- [ ] streaming 模式下 `gen_ai.response.time_to_first_chunk` 正确计量
- [ ] 错误情况下 `error.type` 属性正确设置
- [ ] metrics exporter 输出 token 用量、延迟、工具调用计数

---

## Phase 3: Multi-Agent 能力

### 3.1 现状

- ❌ 无 Multi-Agent 支持
- ⚠️ SubFlow 轻量子流程（非完整子 Agent）

### 3.2 目标

对标 AgentScope 的 Team 工具 + SubAgentTemplate 模式，实现：
- **Team 编排**：多 Agent 协作，消息传递，角色分工
- **Sub-Agent 委托**：Agent 内嵌子 Agent，任务委托 + 结果回传
- **Agent 间通信**：共享消息总线，支持直连和广播

### 3.3 架构设计

```
internal/multiagent/
├── team.go                   # Team 编排器
├── team_config.go            # Team 配置（角色、拓扑）
├── subagent.go               # SubAgent 委托
├── subagent_template.go      # SubAgent 模板（动态派生）
├── bus.go                    # Agent 间消息总线
├── topology.go               # 拓扑结构（star/mesh/chain/dag）
├── round.go                  # 回合控制 + 发言人选择
├── handoff.go                # Agent 间任务移交
└── team_test.go
```

### 3.4 Team 编排伪代码

```go
// Team 代表一组协作 Agent
type Team struct {
    Name        string
    Members     []TeamMember
    Topology    Topology        // Star | Mesh | Chain | DAG
    Coordinator *Agent          // 协调者 Agent
    Bus         *MessageBus     // 消息总线
    MaxRounds   int             // 最大回合数
}

type TeamMember struct {
    Agent      *Agent
    Role       string           // "researcher", "critic", "executor"
    AutoReply  bool             // 是否自动回复
    Handoff    []string         // 可移交的 Agent 名称列表
}

// Round 执行一轮 Team 协作
func (t *Team) Round(ctx context.Context, task string) (*TeamResult, error) {
    // 1. Coordinator 制定计划
    plan, err := t.Coordinator.Plan(ctx, task)
    if err != nil {
        return nil, err
    }

    // 2. 按拓扑调度 Agent
    var results []AgentResult
    for _, step := range plan.Steps {
        member := t.FindMember(step.AssignedTo)
        if member == nil {
            continue
        }

        // 3. 构建上下文（含其他 Agent 的 output）
        ctxMessages := t.Bus.CollectFor(member.Agent.Name)

        // 4. 执行
        result, err := member.Agent.Run(ctx, step.Instruction, ctxMessages)
        if err != nil {
            // 支持 Handoff
            if step.AllowHandoff && len(member.Handoff) > 0 {
                result = t.Handoff(ctx, step, member)
            } else {
                return nil, err
            }
        }

        // 5. 发布结果到消息总线
        t.Bus.Publish(Message{
            From:    member.Agent.Name,
            Role:    member.Role,
            Content: result.Output,
        })
        results = append(results, result)
    }

    // 6. Coordinator 汇总
    finalSummary, _ := t.Coordinator.Summarize(ctx, results)
    return &TeamResult{Results: results, Summary: finalSummary}, nil
}
```

### 3.5 SubAgent 委托伪代码

```go
// SubAgentTemplate 定义可动态派生子 Agent 的模板
type SubAgentTemplate struct {
    Name         string
    SystemPrompt string
    Tools        []string         // 工具白名单
    MaxSteps     int
    InheritMem   bool             // 是否继承父 Agent 记忆
}

// SubAgent 派生实例
type SubAgent struct {
    Parent     *Agent
    Template   *SubAgentTemplate
    Instance   *Agent             // 独立 Agent 实例
    Session    *Session           // 独立 Session
    ctxManager *ContextManager    // 独立 contextmgr（理由见兼容性设计）
}

// Delegate 委托任务到子 Agent
//
// 父 Agent 调用此方法 → 子 Agent 在隔离上下文中执行 → 结果摘要回注父 Agent
func (a *Agent) Delegate(ctx context.Context, templateName string, task string) (*SubAgentResult, error) {
    tmpl := a.subAgentRegistry.Get(templateName)
    if tmpl == nil {
        return nil, fmt.Errorf("sub-agent template %q not found", templateName)
    }

    // 1. 创建子 Agent 实例（独立 Session + contextmgr）
    sub := &SubAgent{
        Parent:   a,
        Template: tmpl,
        Instance: a.spawnChild(tmpl),
        Session:  session.New(tmpl.Name + "-" + generateID()),
    }
    sub.ctxManager = contextmgr.New(sub.Session, a.ctxManagerConfig())

    // 2. 执行子 Agent
    result, err := sub.Instance.Run(ctx, task)

    // 3. 摘要回注父 Agent session
    summary := fmt.Sprintf("[SubAgent %s result]\n%s", tmpl.Name, result.Summary)
    a.session.Add(provider.Message{
        Role:    "tool",
        Content: summary,
        Metadata: map[string]any{
            "subagent":  tmpl.Name,
            "task_boundary": true, // 对应 InferGlow task_group_id 概念
        },
    })
    // 同步到父 contextmgr
    if a.ctxManager != nil {
        a.ctxManager.Ingest(contextmgr.StepRecord{
            Content:      summary,
            TaskBoundary: true,
            TaskGroup:    "subagent-" + tmpl.Name,
        })
    }

    return result, nil
}
```

### 3.6 消息总线

```go
// MessageBus Agent 间共享消息基础设施
type MessageBus struct {
    mu       sync.RWMutex
    messages []Message
    topics   map[string][]chan Message  // 按 topic 订阅
}

type Message struct {
    From      string
    To        string    // 空 = 广播
    Role      string
    Content   string
    Timestamp time.Time
}

// Publish 发布消息
func (b *MessageBus) Publish(msg Message) {
    b.mu.Lock()
    b.messages = append(b.messages, msg)
    b.mu.Unlock()
    // 通知订阅者
    for _, ch := range b.topics[msg.To] {
        select {
        case ch <- msg:
        default:
        }
    }
}

// CollectFor 收集特定 Agent 可见的消息
func (b *MessageBus) CollectFor(agentName string) []provider.Message {
    b.mu.RLock()
    defer b.mu.RUnlock()
    var msgs []provider.Message
    for _, m := range b.messages {
        if m.To == "" || m.To == agentName {
            msgs = append(msgs, provider.Message{
                Role:    m.Role,
                Content: fmt.Sprintf("[%s]: %s", m.From, m.Content),
            })
        }
    }
    return msgs
}
```

### 3.7 验证标准

- [ ] 创建 3-Agent Team（researcher + critic + executor）→ 协作完成一个分析任务
- [ ] Coordinator 选择发言人正确
- [ ] Agent 间 Handoff 正常工作（researcher 失败 → executor 接管）
- [ ] SubAgent 委托：父 Agent 调用子 Agent → 结果正确回注 session
- [ ] 子 Agent 拥有独立的 contextmgr（独立压缩域）
- [ ] OTel trace 中 `invoke_agent` span 正确嵌套

---

## Phase 4: RAG 文档加载与文本分割

### 4.1 现状（已审计修正）

- ✅ **`rag/` 已是独立 Go module**（`github.com/inferglow/rag`）
- ✅ `DocumentStore` 接口 + `Pipeline.Run()` 已实现
- ✅ 5 种 loader：text, markdown, html, json, csv
- ✅ 3 种 splitter：recursive, token, markdown
- ✅ `EmbeddingRegistry` 已实现
- ⚠️ 缺少：PDF 加载器（需 `go-fitz` 或 `ledongthuc/pdf`）
- ⚠️ `recursive.go` 可选增强 overlap 处理

### 4.2 目标

补全 RAG 管道前置环节：
- **文档加载器**：支持 PDF、HTML、Markdown、TXT、JSON、CSV
- **文本分割器**：RecursiveCharacterTextSplitter、TokenSplitter、MarkdownSplitter
- **Embedding 模型注册表**：统一管理 Embedding Provider
- **文档管道**：load → split → embed → store 流水线

### 4.3 包结构

```
internal/rag/
├── pipeline.go              # DocumentPipeline（load→split→embed→store）
├── loader/
│   ├── loader.go            # DocumentLoader 接口
│   ├── pdf.go               # PDF 加载器（依赖 ledongthuc/pdf 或 go-fitz）
│   ├── html.go              # HTML 加载器（goquery）
│   ├── markdown.go          # Markdown 加载器
│   ├── text.go              # 纯文本加载器
│   ├── json.go              # JSON 加载器
│   ├── csv.go               # CSV 加载器
│   └── directory.go         # 目录递归加载器
├── splitter/
│   ├── splitter.go          # TextSplitter 接口
│   ├── recursive.go         # RecursiveCharacterTextSplitter
│   ├── token.go             # TokenSplitter
│   ├── markdown.go          # MarkdownSplitter（保持标题层级）
│   └── semantic.go          # 语义分割器（按段落/句子边界）
├── embedder/
│   └── registry.go          # Embedding 模型注册表
└── document.go              # Document 数据结构
```

### 4.4 核心接口

```go
// Document 统一文档表示
type Document struct {
    ID        string            `json:"id"`
    Content   string            `json:"content"`
    Metadata  map[string]string `json:"metadata"`
    Source    string            `json:"source"`    // 文件路径或 URL
    Page      int               `json:"page"`      // PDF 页码
    ChunkIndex int              `json:"chunk_index"`
}

// DocumentLoader 文档加载器接口
type DocumentLoader interface {
    Load(ctx context.Context, source string) ([]Document, error)
    SupportedExtensions() []string
}

// TextSplitter 文本分割器接口
type TextSplitter interface {
    SplitText(text string, metadata map[string]string) ([]Document, error)
}

// DocumentPipeline 完整文档处理管道
type DocumentPipeline struct {
    loader   DocumentLoader
    splitter TextSplitter
    embedder Embedder
    store    VectorStore
}

func (p *DocumentPipeline) Run(ctx context.Context, source string) (int, error) {
    // 1. Load
    docs, err := p.loader.Load(ctx, source)
    if err != nil {
        return 0, fmt.Errorf("load: %w", err)
    }

    // 2. Split
    var chunks []Document
    for _, doc := range docs {
        chunked, _ := p.splitter.SplitText(doc.Content, doc.Metadata)
        chunks = append(chunks, chunked...)
    }

    // 3. Embed + Store（批量）
    texts := make([]string, len(chunks))
    for i, c := range chunks {
        texts[i] = c.Content
    }
    vectors, err := p.embedder.EmbedBatch(ctx, texts)
    if err != nil {
        return 0, fmt.Errorf("embed: %w", err)
    }

    for i, chunk := range chunks {
        p.store.Insert(ctx, VectorRecord{
            ID:       chunk.ID,
            Vector:   vectors[i],
            Content:  chunk.Content,
            Metadata: chunk.Metadata,
        })
    }

    return len(chunks), nil
}
```

### 4.5 RecursiveCharacterTextSplitter 伪代码

```go
// RecursiveCharacterTextSplitter 递归字符分割器
type RecursiveCharacterTextSplitter struct {
    ChunkSize    int      // 每块最大字符数
    ChunkOverlap int      // 块间重叠字符数
    Separators   []string // 分隔符优先级 ["\n\n", "\n", "。", ".", " ", ""]
}

func (s *RecursiveCharacterTextSplitter) SplitText(
    text string, meta map[string]string,
) ([]Document, error) {
    return s.splitWithSeparator(text, meta, 0)
}

func (s *RecursiveCharacterTextSplitter) splitWithSeparator(
    text string, meta map[string]string, sepIdx int,
) ([]Document, error) {
    if sepIdx >= len(s.Separators) {
        // 最后兜底：按字符数硬切
        return s.hardSplit(text, meta), nil
    }

    sep := s.Separators[sepIdx]
    var docs []Document

    if sep == "" {
        return s.hardSplit(text, meta), nil
    }

    parts := strings.Split(text, sep)
    var currentChunk strings.Builder

    for _, part := range parts {
        if currentChunk.Len()+len(part) > s.ChunkSize {
            // 当前块满 → 输出
            if currentChunk.Len() > 0 {
                docs = append(docs, s.newDoc(currentChunk.String(), meta))
                currentChunk.Reset()
            }
            // 单个 part 仍超长 → 递归下一级分隔符
            if len(part) > s.ChunkSize {
                subDocs, _ := s.splitWithSeparator(part, meta, sepIdx+1)
                docs = append(docs, subDocs...)
                continue
            }
        }
        if currentChunk.Len() > 0 {
            currentChunk.WriteString(sep)
        }
        currentChunk.WriteString(part)
    }
    if currentChunk.Len() > 0 {
        docs = append(docs, s.newDoc(currentChunk.String(), meta))
    }

    // 处理重叠
    if s.ChunkOverlap > 0 {
        docs = s.applyOverlap(docs)
    }
    return docs, nil
}
```

### 4.6 Embedding 模型注册表

```go
// EmbedderRegistry 统一管理 Embedding Provider
type EmbedderRegistry struct {
    providers map[string]Embedder
    default_  string
}

// 预置 Embedder
func DefaultEmbedderRegistry() *EmbedderRegistry {
    r := &EmbedderRegistry{providers: make(map[string]Embedder)}
    // OpenAI text-embedding-3-small
    r.Register("openai", NewOpenAIEmbedder("text-embedding-3-small"))
    // DeepSeek embedding
    r.Register("deepseek", NewDeepSeekEmbedder())
    // 本地模型（通过 Ollama）
    r.Register("ollama", NewOllamaEmbedder("nomic-embed-text"))
    // 国内模型
    r.Register("qwen", NewQwenEmbedder("text-embedding-v3"))
    r.SetDefault("openai")
    return r
}
```

### 4.7 验证标准

- [ ] PDF 加载 → 分割 → embed → store 端到端可用
- [ ] Markdown 分割保持标题层级（`# Title` 不跨块切割）
- [ ] `ChunkOverlap` 正确：相邻块重叠区域内容一致
- [ ] 分割后块数合理（100 页 PDF → ~500 chunks）
- [ ] 向量检索可召回原始文档片段

---

## Phase 5: 可选 Rerank 支持

### 5.1 现状（已审计修正）

- ✅ **`rerank/` 已是独立 Go module**（`github.com/inferglow/rerank`）
- ✅ `Reranker` 接口 + `Document` 类型已定义
- ✅ Cohere Rerank API 实现（`cohere.go`）
- ✅ LLM-based Rerank 实现（`llm.go`）
- ✅ Fallback 降级链（`fallback.go`）+ Factory（`factory.go`）
- ⚠️ 缺少：Cross-Encoder ONNX 实现

### 5.2 目标

可选的重排序支持，三种后端：
- **Cohere Rerank API**
- **Cross-Encoder 模型**（通过 huggingface 或本地 ONNX）
- **LLM-based Rerank**（用压缩小模型做 pairwise 排序）

### 5.3 接口设计

```go
// internal/rerank/rerank.go

// Reranker 重排序接口
type Reranker interface {
    // Rerank 对候选文档按与 query 的相关性重排序
    Rerank(ctx context.Context, query string, documents []RerankDocument) ([]RerankDocument, error)
    Name() string
}

type RerankDocument struct {
    Index    int               // 原始索引
    Content  string
    Score    float64           // 原始分数（检索阶段）
    RerankScore float64        // 重排序分数
    Metadata map[string]string
}

// RerankPipeline 检索后重排序管道
type RerankPipeline struct {
    reranker    Reranker
    topN        int             // 重排序后保留条数
    maxInputLen int             // 重排序输入最大字符数（截断）
}
```

### 5.4 Cohere Reranker 实现

```go
// internal/rerank/cohere.go

type CohereReranker struct {
    apiKey     string
    model      string            // "rerank-english-v3.0" | "rerank-multilingual-v3.0"
    httpClient *http.Client
}

func (r *CohereReranker) Rerank(ctx context.Context, query string, docs []RerankDocument) ([]RerankDocument, error) {
    // 构建 Cohere Rerank API 请求
    type cohereReq struct {
        Model     string   `json:"model"`
        Query     string   `json:"query"`
        Documents []string `json:"documents"`
        TopN      int      `json:"top_n,omitempty"`
    }

    req := cohereReq{
        Model:     r.model,
        Query:     query,
        Documents: make([]string, len(docs)),
    }
    for i, d := range docs {
        req.Documents[i] = truncate(d.Content, r.maxInputLen)
    }

    // 调用 Cohere API
    resp, err := r.call(ctx, "https://api.cohere.com/v1/rerank", req)
    if err != nil {
        return nil, err
    }

    // 按 rerank score 降序排序
    sort.Slice(docs, func(i, j int) bool {
        return docs[i].RerankScore > docs[j].RerankScore
    })
    return docs, nil
}
```

### 5.5 Cross-Encoder Reranker 实现

```go
// internal/rerank/cross_encoder.go

type CrossEncoderReranker struct {
    modelPath string              // ONNX 模型路径
    tokenizer *Tokenizer          // 内置 tokenizer
    session   *ort.Session        // ONNX Runtime session
}

func (r *CrossEncoderReranker) Rerank(ctx context.Context, query string, docs []RerankDocument) ([]RerankDocument, error) {
    // 为每个 (query, doc) 对计算相关性分数
    type pair struct {
        idx   int
        score float64
    }
    pairs := make([]pair, len(docs))

    // 批量推理（batch_size=32）
    for i := 0; i < len(docs); i += 32 {
        end := min(i+32, len(docs))
        batch := docs[i:end]
        inputs := r.tokenizeBatch(query, batch)
        outputs, err := r.session.Run(inputs)
        if err != nil {
            return nil, err
        }
        for j, score := range outputs {
            pairs[i+j] = pair{idx: i + j, score: score}
        }
    }

    // 按 score 排序
    sort.Slice(pairs, func(i, j int) bool { return pairs[i].score > pairs[j].score })

    result := make([]RerankDocument, len(docs))
    for i, p := range pairs {
        result[i] = docs[p.idx]
        result[i].RerankScore = p.score
    }
    return result, nil
}
```

### 5.6 检索管道集成

```go
// internal/contextmgr/retrieval.go — 现有三路融合召回末尾插入 rerank

func (h *HybridManager) retrieve(ctx context.Context, query string, topK int) ([]RetrievedDoc, error) {
    // 1. 三路融合召回（现有逻辑不变）
    candidates := h.fusionRetrieve(ctx, query, topK*2) // 召回 2x topK 候选

    // 2. 可选 Rerank
    if h.reranker != nil {
        rerankDocs := make([]rerank.RerankDocument, len(candidates))
        for i, c := range candidates {
            rerankDocs[i] = rerank.RerankDocument{
                Index:   i,
                Content: c.Content,
                Score:   c.Score,
            }
        }
        reranked, err := h.reranker.Rerank(ctx, query, rerankDocs)
        if err == nil {
            // 重排序成功 → 取 topK
            result := make([]RetrievedDoc, min(topK, len(reranked)))
            for i, r := range reranked[:len(result)] {
                result[i] = candidates[r.Index]
                result[i].RerankScore = r.RerankScore
            }
            return result, nil
        }
        // Rerank 失败 → 降级到原始排序
    }

    // 3. Fallback：原始排序取 topK
    return candidates[:min(topK, len(candidates))], nil
}
```

### 5.7 配置

```toml
[rerank]
enabled = true
backend = "cohere"               # "cohere" | "cross_encoder" | "llm"
model = "rerank-multilingual-v3.0"
api_key = "${COHERE_API_KEY}"
top_n = 10                       # 重排序后保留条数
max_input_length = 512           # 单文档最大输入 token/字符数
fallback_on_error = true         # 失败时降级到原始排序
```

### 5.8 验证标准

- [ ] Cohere Rerank API 调用成功，返回重排序结果
- [ ] Cross-Encoder ONNX 模型加载 + 推理正常
- [ ] 重排序后结果与原始排序有显著差异（NDCG 提升 ≥ 10%）
- [ ] Rerank 失败时降级到原始排序（不阻塞检索）
- [ ] 检索管道中可选启用/禁用 rerank

---

## Phase 6: 沙箱增强（从 AgentScope 借鉴）

### 6.1 现状（已审计修正）

InferGlow v5 沙箱后端审计结果：**7 个完整实现 + 1 个编排框架（3 子后端 stub）**：

| # | Provider | 文件 | 状态 |
|---|----------|------|:--:|
| 1 | **TrustedLocal** | `sandbox/trusted_local.go` | ✅ 完整 |
| 2 | **Docker** | `sandbox/docker.go` + `docker_real.go` | ✅ 完整 |
| 3 | **GVisor** | `sandbox/gvisor.go` | ✅ 完整（Docker + runsc） |
| 4 | **Bubblewrap** | `sandbox/bubblewrap.go` | ✅ 完整（仅 Linux） |
| 5 | **Landlock** | `sandbox/landlock.go` | ✅ 完整（仅 Linux） |
| 6 | **Seatbelt** | `sandbox/seatbelt.go` | ✅ 完整（仅 macOS） |
| 7 | **E2B** | `sandbox/e2b.go` | ✅ 完整（Firecracker 微虚拟机） |
| 8 | **WindowsRuntime** | `sandbox/windows_runtime.go` | ⚠️ 框架完整，3 子后端 **全部 stub** |

**WindowsRuntime 三个子后端 stub 明细**：

| 子后端 | 文件 | stub 证据 |
|--------|------|-----------|
| RestrictedToken | `windows_restricted_token.go:198` | `CreateRestrictedToken()` 返回 `"requires real Windows API implementation"`，Execute 直接用 `exec.CommandContext` |
| AppContainer | `windows_appcontainer.go:197` | `setupAppContainerEnvironment()` 只建目录，`isAppContainerAvailable()` 用了错误的 API 检测 |
| WindowsSandbox | `windows_sandbox.go:76` | `Start()` 只设状态不启 VM，`Execute()` 在 host 直接跑，`generateWSConfig()` **已正确实现** |

### 6.2 Windows 沙箱补齐方案

按实现难度和隔离强度排序——RestrictedToken 最简单应先做，AppContainer 中等，WindowsSandbox 需要 VM 生命周期管理。

#### 6.2.1 RestrictedToken（最简单，P0）

**缺失**：`CreateRestrictedToken()` 是一条 error stub。Execute 用 `exec.CommandContext` 直接跑在 host 上，零隔离。

**补齐方案**——调用 `golang.org/x/sys/windows` 原生 API：

```go
//go:build windows

// createRestrictedToken 调用 Windows advapi32 创建受限令牌
func createRestrictedToken() (windows.Token, error) {
    // 1. 获取当前进程令牌
    var token windows.Token
    err := windows.OpenProcessToken(windows.CurrentProcess(),
        windows.TOKEN_DUPLICATE|windows.TOKEN_QUERY|windows.TOKEN_ADJUST_PRIVILEGES,
        &token)
    if err != nil {
        return 0, fmt.Errorf("OpenProcessToken: %w", err)
    }
    defer token.Close()

    // 2. 定义要禁用的危险特权
    disablePrivs := []string{
        "SeShutdownPrivilege",
        "SeDebugPrivilege",
        "SeTakeOwnershipPrivilege",
        "SeLoadDriverPrivilege",
        "SeSystemtimePrivilege",
        "SeRemoteShutdownPrivilege",
        "SeIncreaseQuotaPrivilege",
        "SeSecurityPrivilege",
        "SeBackupPrivilege",
        "SeRestorePrivilege",
    }

    // 3. 将特权名转为 LUID 并禁用
    var disabledLUIDs []windows.LUIDAndAttributes
    for _, name := range disablePrivs {
        luid, err := lookupPrivilegeValue(name)
        if err != nil {
            continue // 特权不可用则跳过
        }
        disabledLUIDs = append(disabledLUIDs, windows.LUIDAndAttributes{
            Luid:       luid,
            Attributes: 0, // 禁用
        })
    }

    // 4. 创建受限令牌（移除指定特权）
    var restrictedToken windows.Token
    err = windows.CreateRestrictedToken(token, 0,
        uint32(len(disabledLUIDs)), disabledLUIDs,
        0, nil,  // 不删除 SID
        0, nil,  // 不添加限制 SID
        &restrictedToken)
    if err != nil {
        return 0, fmt.Errorf("CreateRestrictedToken: %w", err)
    }
    return restrictedToken, nil
}
```

**Execute 改造**——将 `exec.CommandContext` 替换为 `CreateProcessAsUser`：

```go
func (h *RestrictedTokenHandle) Execute(ctx context.Context, cmd *Command) (*ExecutionResult, error) {
    // ... 策略检查不变 ...

    // 使用受限令牌启动进程（替代 exec.CommandContext）
    process, err := launchWithToken(h.restrictedToken, cmd)
    // ... 等待 + 收集结果 ...
}
```

**工作量估算**：~200 行代码，1-2 天，纯 Windows API 调用，无外部依赖。

#### 6.2.2 AppContainer（中等，P1）

**缺失**：未调用 `CreateAppContainerProfile`（userenv.dll），未使用 `PROC_THREAD_ATTRIBUTE_SECURITY_CAPABILITIES` 启动进程。`isAppContainerAvailable()` 检查了错误的 API（`GetCurrentProcessVersion` 而非 `CreateAppContainerProfile`）。

**补齐步骤**：

```text
1. 修正可用性检测 → 检查 userenv.dll 中的 CreateAppContainerProfile
2. Start() 时调用：
   - CreateAppContainerProfile(name, displayName, description, caps, &sid)
   - DeriveAppContainerSidFromAppContainerName(name, &sid)
3. 配置文件系统能力：
   - GetNamedSecurityInfo(sandboxDir) → 添加 AppContainer SID 的读/执行 ACE
4. 配置注册表能力（可选）：
   - 限制到 HKCU\Software\AppContainer\*
5. 配置网络能力：
   - FIREWALL_CAPABILITY_INTERNET_CLIENT 或禁用
6. Execute() 使用 CreateProcess 启动：
   - EXTENDED_STARTUPINFO_PRESENT 标志
   - PROC_THREAD_ATTRIBUTE_SECURITY_CAPABILITIES
   - lpStartupInfo.lpAttributeList 包含 SECURITY_CAPABILITIES
7. Stop() 时调用 DeleteAppContainerProfile
```

**工作量估算**：~500 行代码 + Capability SID 管理，3-5 天。

#### 6.2.3 WindowsSandbox（最复杂，P2）

**缺失**：`generateWSConfig()` 已正确实现（生产就绪），但 `Start()` 不启 VM，`Execute()` 在 host 上跑，缺少 host↔sandbox 通信桥。

**补齐步骤**：

```text
1. Start() 改造：
   - 将 generateWSConfig() 输出写入临时 .wsb 文件
   - 启动 Microsoft.WindowsSandbox.exe <config>.wsb
   - 轮询共享文件夹中的 ready 标记文件（最多等 30s）
   - 存储 sandbox PID 供 Stop 使用

2. Execute() 桥接（推荐方案——基于共享文件夹的命令文件）：
   - 将命令写入共享文件夹作为 .ps1 脚本
   - 写入 trigger 标记文件（sandbox 内的 watcher 脚本检测到后执行）
   - 轮询 output 标记文件等待结果
   - 从共享文件夹读取 stdout/stderr

   简化方案（利用 LogonCommand + MappedFolder）：
   - 每个新 Execute 调用 = 新的 Sandbox 实例（利用 LogonCommand 执行单条命令）
   - 适合低频率命令执行（Start 冷启动 ~5s）

3. Stop() 改造：
   - 调用 TerminateProcess 杀掉 Microsoft.WindowsSandbox.exe
   - Sandbox 内所有状态自动销毁（VM 特性）
   - 清理临时 .wsb 文件
```

**工作量估算**：~400 行代码 + PowerShell watcher 脚本 + 同步协议，5-8 天。

#### 6.2.4 补齐优先级

| 优先级 | 子后端 | 隔离强度 | 工作量 | 理由 |
|:---:|--------|:-----:|:---:|------|
| **P0** | RestrictedToken | ⭐⭐ (进程级) | 1-2 天 | 只差 CreateRestrictedToken 一行 stub，补齐后整个 WindowsRuntime 即有一个真实可用的子后端 |
| **P1** | AppContainer | ⭐⭐⭐ (应用级) | 3-5 天 | 文件系统+注册表+网络三维能力控制，UWP 级别隔离 |
| **P2** | WindowsSandbox | ⭐⭐⭐⭐⭐ (VM 级) | 5-8 天 | WSConfig 已就绪，缺 VM 生命周期 + 通信桥 |

### 6.4 沙箱预热池（所有后端通用增强）

```go
// internal/sandbox/pool.go

// SandboxPool 沙箱预热池——减少冷启动延迟
type SandboxPool struct {
    backend   SandboxBackend
    pool      chan SandboxInstance  // 空闲沙箱队列
    minSize   int                   // 最小预热数
    maxSize   int                   // 最大并发数
    idleTimeout time.Duration       // 空闲沙箱回收时间
    image     string                // Docker 镜像
    mu        sync.Mutex
}

func NewSandboxPool(backend SandboxBackend, cfg PoolConfig) *SandboxPool {
    p := &SandboxPool{
        backend:     backend,
        pool:        make(chan SandboxInstance, cfg.MaxSize),
        minSize:     cfg.MinSize,
        maxSize:     cfg.MaxSize,
        idleTimeout: cfg.IdleTimeout,
    }
    // 预热：预启动 minSize 个沙箱
    for i := 0; i < cfg.MinSize; i++ {
        inst, err := backend.Create(context.Background())
        if err == nil {
            p.pool <- inst
        }
    }
    // 后台 goroutine 维持池大小
    go p.maintain()
    return p
}

// Acquire 获取一个预热沙箱（优先从池中取）
func (p *SandboxPool) Acquire(ctx context.Context) (SandboxInstance, error) {
    select {
    case inst := <-p.pool:
        // 池中有预热沙箱 → 快速获取
        return inst, nil
    default:
        // 池空 → 创建新沙箱
        return p.backend.Create(ctx)
    }
}

// Release 归还沙箱到池（或销毁）
func (p *SandboxPool) Release(inst SandboxInstance) {
    select {
    case p.pool <- inst:
        // 归还成功
    default:
        // 池满 → 销毁
        inst.Destroy()
    }
}

// maintain 后台维持池大小 + 空闲回收
func (p *SandboxPool) maintain() {
    ticker := time.NewTicker(10 * time.Second)
    for range ticker.C {
        // 补充到 minSize
        for len(p.pool) < p.minSize {
            inst, err := p.backend.Create(context.Background())
            if err == nil {
                p.pool <- inst
            }
        }
    }
}
```

### 6.5 沙箱资源限额

```go
// internal/sandbox/limits.go

type SandboxLimits struct {
    CPUMax        string  // "0.5" = 0.5 CPU 核心
    MemoryMax     string  // "256m"
    DiskMax       string  // "1g"
    NetworkAccess bool    // 是否允许网络访问
    TimeoutSec    int     // 最大执行时间
    MaxProcesses  int     // 最大进程数
}

// Docker 后端映射
func (l SandboxLimits) DockerHostConfig() container.HostConfig {
    return container.HostConfig{
        Resources: container.Resources{
            CPUQuota:   l.cpuQuota(),
            Memory:     l.memoryBytes(),
            DiskQuota:  l.diskBytes(),
            PidsLimit:  &l.MaxProcesses,
        },
        NetworkMode: l.networkMode(), // "none" 或 "bridge"
    }
}
```

### 6.6 验证标准

**Windows 沙箱补齐：**

- [ ] RestrictedToken：`CreateRestrictedToken()` 成功创建受限令牌（`golang.org/x/sys/windows` API）
- [ ] RestrictedToken：子进程 `whoami /priv` 输出不含 SeShutdownPrivilege 等危险特权
- [ ] AppContainer：`CreateAppContainerProfile` + `PROC_THREAD_ATTRIBUTE_SECURITY_CAPABILITIES` 进程启动成功
- [ ] AppContainer：沙箱进程无法访问 `C:\Windows\System32\config\`（无 ACL 权限）
- [ ] WindowsSandbox：`Microsoft.WindowsSandbox.exe` 启动后共享文件夹可读写
- [ ] WindowsSandbox：沙箱内执行命令的结果通过共享文件夹正确回传

**预热池 + 资源限额：**

- [ ] 沙箱预热池：第一个 Acquire 延迟 < 50ms（vs 冷启动 ~2s）
- [ ] 池大小自动维持（低于 minSize 自动补充）
- [ ] 空闲沙箱超时自动回收
- [ ] 资源限额生效：CPU/内存/磁盘超限 → 沙箱被 kill
- [ ] 网络隔离：`NetworkAccess=false` 时沙箱内 `curl` 失败

---

## Phase 7: MCP 三传输优化

### 7.1 现状

- ✅ MCP 已实现（基础 consumer）
- ❌ 无 stdio 传输
- ❌ 无 SSE 传输
- ❌ 无 StreamableHTTP 传输
- ❌ 无 MCP server 端

AgentScope 有 3 种传输（STDIO + SSE + StreamableHTTP）。

### 7.2 目标

补齐三种 MCP 客户端传输 + MCP Server 端能力。

### 7.3 MCP 传输层架构

```
internal/mcp/
├── client.go                     # MCP Client 统一入口
├── transport/
│   ├── transport.go              # Transport 接口
│   ├── stdio.go                  # STDIO 传输（子进程通信）
│   ├── sse.go                    # SSE 传输（HTTP long-poll）
│   └── streamable_http.go        # StreamableHTTP 传输（2024-11-05 规范）
├── server/
│   ├── server.go                 # MCP Server 端
│   ├── tool_provider.go          # Tool 注册接口
│   ├── resource_provider.go      # Resource 注册接口
│   └── prompt_provider.go        # Prompt 注册接口
├── types/
│   └── protocol.go               # MCP 协议类型（JSON-RPC 2.0）
└── mcp_test.go
```

### 7.4 Transport 接口

```go
// Transport MCP 传输层接口
type Transport interface {
    // Connect 建立连接
    Connect(ctx context.Context) error
    // Send 发送 JSON-RPC 请求
    Send(ctx context.Context, req json.RawMessage) error
    // Receive 接收 JSON-RPC 响应/通知
    Receive(ctx context.Context) (json.RawMessage, error)
    // Close 关闭连接
    Close() error
    // Type 返回传输类型标识
    Type() string // "stdio" | "sse" | "streamable_http"
}
```

### 7.5 STDIO 传输

```go
// internal/mcp/transport/stdio.go

type StdioTransport struct {
    cmd    *exec.Cmd
    stdin  io.WriteCloser
    stdout io.ReadCloser
    stderr io.ReadCloser
    mu     sync.Mutex
}

func NewStdioTransport(command string, args []string, env map[string]string) *StdioTransport {
    cmd := exec.Command(command, args...)
    if env != nil {
        cmd.Env = os.Environ()
        for k, v := range env {
            cmd.Env = append(cmd.Env, k+"="+v)
        }
    }
    return &StdioTransport{cmd: cmd}
}

func (t *StdioTransport) Connect(ctx context.Context) error {
    stdin, err := t.cmd.StdinPipe()
    if err != nil {
        return err
    }
    stdout, err := t.cmd.StdoutPipe()
    if err != nil {
        return err
    }
    t.stdin = stdin
    t.stdout = stdout
    t.stderr = t.cmd.Stderr // 可选的 stderr 捕获

    if err := t.cmd.Start(); err != nil {
        return fmt.Errorf("start MCP server process: %w", err)
    }
    return nil
}

func (t *StdioTransport) Send(ctx context.Context, req json.RawMessage) error {
    t.mu.Lock()
    defer t.mu.Unlock()
    // JSON-RPC 通过 stdin 发送，以换行符分隔
    _, err := fmt.Fprintf(t.stdin, "%s\n", req)
    return err
}

func (t *StdioTransport) Receive(ctx context.Context) (json.RawMessage, error) {
    // 从 stdout 读取一行 JSON-RPC 响应
    scanner := bufio.NewScanner(t.stdout)
    if scanner.Scan() {
        return json.RawMessage(scanner.Bytes()), nil
    }
    return nil, scanner.Err()
}

func (t *StdioTransport) Close() error {
    t.stdin.Close()
    return t.cmd.Wait()
}
```

### 7.6 SSE 传输

```go
// internal/mcp/transport/sse.go

type SSETransport struct {
    baseURL    string
    httpClient *http.Client
    eventCh    chan sse.Event
    cancel     context.CancelFunc
}

func (t *SSETransport) Connect(ctx context.Context) error {
    ctx, t.cancel = context.WithCancel(ctx)

    req, _ := http.NewRequestWithContext(ctx, "GET", t.baseURL+"/sse", nil)
    req.Header.Set("Accept", "text/event-stream")

    resp, err := t.httpClient.Do(req)
    if err != nil {
        return err
    }
    if resp.StatusCode != 200 {
        return fmt.Errorf("SSE connect failed: %d", resp.StatusCode)
    }

    // 后台解析 SSE 事件流
    go t.parseSSEStream(ctx, resp.Body)
    return nil
}

func (t *SSETransport) Send(ctx context.Context, req json.RawMessage) error {
    // SSE 模式：HTTP POST 发送请求，SSE 流接收响应
    httpReq, _ := http.NewRequestWithContext(ctx, "POST",
        t.baseURL+"/message", bytes.NewReader(req))
    httpReq.Header.Set("Content-Type", "application/json")
    resp, err := t.httpClient.Do(httpReq)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    return nil // 响应通过 event channel 异步返回
}

func (t *SSETransport) Receive(ctx context.Context) (json.RawMessage, error) {
    select {
    case event := <-t.eventCh:
        return json.RawMessage(event.Data), nil
    case <-ctx.Done():
        return nil, ctx.Err()
    }
}
```

### 7.7 MCP Server 端

```go
// internal/mcp/server/server.go

type MCPServer struct {
    name     string
    version  string
    tools    []ToolDefinition
    resources []ResourceDefinition
    prompts  []PromptDefinition
    transport Transport
}

// ServeTool 注册一个 InferGlow Action 为 MCP Tool
func (s *MCPServer) ServeTool(action *Action) ToolDefinition {
    tool := ToolDefinition{
        Name:        action.Name(),
        Description: action.Description(),
        InputSchema: action.InputSchema(),
    }
    s.tools = append(s.tools, tool)
    return tool
}

// Listen 启动 MCP Server
func (s *MCPServer) Listen(ctx context.Context, transport Transport) error {
    s.transport = transport
    if err := transport.Connect(ctx); err != nil {
        return err
    }
    return s.serve(ctx) // JSON-RPC 循环
}

func (s *MCPServer) handleToolsList(ctx context.Context, req jsonrpc.Request) jsonrpc.Response {
    return jsonrpc.Response{
        Result: map[string]any{
            "tools": s.tools,
        },
    }
}

func (s *MCPServer) handleToolsCall(ctx context.Context, req jsonrpc.Request) jsonrpc.Response {
    var params struct {
        Name      string         `json:"name"`
        Arguments json.RawMessage `json:"arguments"`
    }
    json.Unmarshal(req.Params, &params)

    // 查找并执行 Tool
    for _, tool := range s.tools {
        if tool.Name == params.Name {
            result, err := tool.Execute(ctx, params.Arguments)
            if err != nil {
                return jsonrpc.ErrorResponse(req.ID, -32000, err.Error())
            }
            return jsonrpc.Response{
                Result: map[string]any{
                    "content": []map[string]any{
                        {"type": "text", "text": result},
                    },
                },
            }
        }
    }
    return jsonrpc.ErrorResponse(req.ID, -32602, "tool not found")
}
```

### 7.8 配置

```toml
[mcp]
# MCP 客户端模式
[[mcp.servers]]
name = "filesystem"
transport = "stdio"
command = "npx"
args = ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]

[[mcp.servers]]
name = "weather"
transport = "streamable_http"
url = "https://mcp-weather.example.com/mcp"

[[mcp.servers]]
name = "database"
transport = "sse"
url = "https://mcp-db.example.com"

# MCP Server 模式（可选）
[mcp.server]
enabled = false
transport = "streamable_http"
listen = ":9090"
```

### 7.9 验证标准

- [ ] STDIO 传输：连接 `npx @modelcontextprotocol/server-filesystem` → 列工具 → 调用成功
- [ ] SSE 传输：连接远程 SSE 端点 → 消息收发正常
- [ ] StreamableHTTP 传输：POST + 流式响应完整
- [ ] MCP Server 端：InferGlow Action 暴露为 MCP Tool → 外部 MCP Client 可调用
- [ ] 三种传输可混用（不同 server 不同传输）

---

## 八、总体交付路线图

```
           Week 1-2          Week 3-4          Week 5-6          Week 7-8
Phase 0:   [████████████████]                                          上下文管理校准
Phase 1:   [████████████████████████████████]                          REST API Server
Phase 2:   [████████████████████████████████████████]                  OTel GenAI 全集成
Phase 3:   [                    ████████████████████████████████]      Multi-Agent
Phase 4:   [████████████████████████████████]                          RAG 文档加载+分割
Phase 5:   [████████████████████]                                      可选 Rerank
Phase 6:   [                    ████████████████████████████████]      沙箱增强（Win补齐+预热池+限额）
Phase 7:   [                    ████████████████████████████████]      MCP 优化
```

### 预期覆盖率提升

| 模块 | 当前 v5 覆盖率 | v6 目标覆盖率 | 增量 |
|------|:---------:|:---------:|:----:|
| 模型连接层 | 82% | 85% | +3%（OTel 成本追踪增强） |
| Chain 编排 | 63% | 68% | +5%（三问重组） |
| Agent 核心 | 58% | 75% | +17%（Multi-Agent） |
| 记忆管理 | 83% | 90% | +7%（甜点区+预热） |
| RAG | 33% | 60% | +27%（文档加载+分割+rerank） |
| 工具系统 | 86% | 88% | +2%（MCP 三传输） |
| 安全与合规 | 80% | 80% | —（已领先） |
| 可观测性 | 40% | 75% | +35%（完整 OTel GenAI） |
| 通道适配 | 15% | 40% | +25%（REST API） |
| **总覆盖率** | **72%** | **82%** | **+10%** |

### 风险矩阵

| 风险 | 概率 | 影响 | 缓解 |
|------|------|------|------|
| OTel GenAI spec 仍为 Development 状态 → 属性可能在后续版本变更 | 中 | 中 | 封装在 `internal/telemetry/genai/`，接口不变，属性常量集中管理 |
| 三问合并 prompt 输出格式异常 → JSON 解析失败或遗漏 step | 中 | 高 | `cleanJSONBlock()` 清理 markdown 包裹；完整性校验（必须覆盖所有活跃 step_id）；解析失败降级为 mechanicalFoldDigest |
| 三问合并输出超过 compress_model max_tokens → 截断 | 低 | 中 | Q3 的 step_decisions 即使 200 step 也仅 ~12K chars；compress_model max_tokens 设为 4096 |
| Cohere/Cross-Encoder 外部依赖不可用 | 低 | 低 | Rerank 设计为可选，失败降级到原始排序 |
| MCP stdio 传输子进程管理复杂（僵尸进程、信号处理） | 中 | 中 | 使用 `context.WithCancel` + `cmd.Wait()` 确保进程生命周期 |
| Multi-Agent 死锁（Agent A 等 Agent B 等 Agent A） | 中 | 高 | `MaxRounds` 硬限制 + 超时保护 + 死锁检测 |
| 甜点区阈值设置不当 → 频繁触发重组 | 低 | 中 | 默认 256K 保守值 + 动态容忍上调 + 衰减回退机制 |
| Windows RestrictedToken API 在旧版 Windows（<Win8）不可用 | 低 | 低 | `CreateRestrictedToken` 从 Windows 2000 就可用；编译时 `//go:build windows` + 运行时 API 可用性检测 |
| Windows AppContainer API 在 Windows Server 上行为不同 | 中 | 中 | 启动时 `isAppContainerAvailable()` 检测 → 不可用时自动降级到 RestrictedToken |

---

## 九、被拒绝的方案

1. **用 gRPC 替代 REST API**：Go 生态 REST 更通用，gRPC 可作为后续可选增强
2. **用 Langfuse/Weights & Biases 替代原生 OTel**：锁定特定平台不利于生态兼容，OTel 是开放标准
3. **实现 CrewAI 风格的完整 Multi-Agent 框架**：首版聚焦 AgentScope 级别的 Team + SubAgent 模式，不引入角色扮演/记忆共享等复杂性
4. **自研 embedding 模型**：使用现有 Provider（OpenAI/DeepSeek/Ollama），不自研
5. **MCP Server 完整实现**：Phase 7 仅实现 Tool Server（最高 ROI），Resource/Prompt Server 留到 Phase 7+
6. **完全替换现有 compact.go**：保留现有单级摘要作为甜点区内的 fallback，分级压缩作为增强模式并行，通过甜点区阈值切换
7. **三次独立 LLM 调用做三问决策**：浪费 ~58% prefill tokens，合并为单次调用的 JSON 三答模式，思维更连贯且输出量可控
8. **绝对引用数（≥5）触发容忍上调**：在 1M 窗口下阈值太低（高密度任务经常 10-20 次引用），改为滑动窗口引用率 + 衰减回退
9. **声称 8 沙箱后端（含 WindowsRuntime stub）**：修正为 7 个完整实现 + WindowsRuntime 3 子后端待补齐
