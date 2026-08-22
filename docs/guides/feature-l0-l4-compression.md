# InferGlow L0-L4 分层上下文压缩机制（Feature 说明）

> 本文是 `contextmgr`（context 模块，包名 `contextmgr`）**L0-L4 分层压缩**的机制说明书，自包含、不依赖翻代码。
> 用途：理解压缩原理 / 调参 / 排查压缩行为 / 对比其他框架（openhanako、Reasonix、Claude auto-compact 等）。
> 配套：`docs/guides/context-management.md`（使用指南）+ `examples/example_context.go`（可运行示例）。

## 🔍 搜索标签（tags）

`L0` `L1` `L2` `L3` `L4` `分层压缩` `上下文压缩` `上下文管理` `HybridManager` `ModeHybrid` `压缩级别` `LevelThresholds` `阈值表` `decay` `衰减` `strength` `引用强度` `RefRecord` `ref_count` `raw_decay` `TargetLevel` `MaxLevelForType` `类型约束矩阵` `EffectiveDecay` `ComputeDecay` `压缩触发` `窗口压力` `TriggerCompression` `BatchCompress` `CompressStep` `CompressModelChain` `压缩模型链` `机械压缩` `MechanicalCompress` `掩码` `mask header` `L3派生` `事实抽取` `denoise` `去噪` `IdleConsolidator` `空闲整合` `pending_l4` `Introspector` `内省` `L4清理` `task_group归档` `LockL0` `锁L0` `SemanticHold` `渲染缓存` `RenderStepWithCache` `BuildContext` `宪法区` `hot facts` `Expand` `Surround` `Search` `transient` `长时记忆` `RAG` `后备存储`

---

## 1. 一句话定位

**InferGlow 的上下文管理是「全部步骤落库 + 永不丢弃原文 + 按引用热度逐级压缩」的分层模型**：每个 step 永远保留 L0 原文，随着上下文压力（decay）增大，步骤从 L0 逐级降级到 L1（精简）→ L2（事实）→ L3（掩码），最终可被 L4（丢弃，仅限低价值类型）。与「滑动窗口/折叠窗口」不同，它**没有窗口移动**——历史永远可回溯。

---

## 2. 五级定义（L0 ~ L4）

| 级别 | 名称 | 存储 | 内容 | 能否回溯原文 |
|------|------|------|------|-------------|
| **L0** | 原文 | `steps`（主表） | `StepRecord.Content` 原始内容，Ingest 时写入 | 始终可 |
| **L1** | 精简（denoised） | `.l1.jsonl` | `L1Record{Content, TokenCount}`，去噪/瘦身后的内容 | 可（full=true 拿 L0） |
| **L2** | 事实 | `.l2.jsonl` | `L2Record{Facts[]}`，事实抽取 + 首行掩码头 `[掩码 step_N\|原Xt\|tool\|params]` | 可 |
| **L3** | 掩码 | `.l3.jsonl` | `L3Record{Mask}`，仅保留第一行掩码头（**由 L2 输出派生**） | 可 |
| **L4** | 丢弃 | `refs` 中 `level=4` + `RemoveRef` | 从活跃上下文中移除（见 §4 类型约束） | **不可**（唯一的真删除） |

> 核心原则：**压缩是降级不是删除**——L0~L3 全部可逆，`Expand(stepID, full=true)` 随时拿回原文；只有 L4 真正移除。

---

## 3. 核心数据结构

### 3.1 `RefRecord`（每 step 的压缩状态，refs 表）
| 字段 | 含义 |
|------|------|
| `Level` | 当前压缩级别（0~4） |
| `RefCount` | 累计 §N 引用计数（被后续步骤引用次数） |
| `LastRefAtStep` | 最近一次被引用的 step 号 |
| `Strength` | 累计访问强度（初始 1.0，每次引用 +0.1；相关文件活跃编辑 file_mod=0.3 加成） |
| `TaskGroupID` / `TaskBoundary` | 任务组归属 / 是否为组起点 |
| `SemanticHold` | 语义安全网保持标记（Redis） |
| `PendingL4` | 空闲整合预标记（见 §7.4） |
| `RelatedFiles` | 关联文件列表 |

### 3.2 分级记录
- `L1Record{StepID, Content, TokenCount, CompressedAtStep}` → `.l1.jsonl`
- `L2Record{StepID, Facts[]string, TokenCount, CompressedAtStep}` → `.l2.jsonl`
- `L3Record{StepID, Mask, TokenCount, CompressedAtStep}` → `.l3.jsonl`

### 3.3 `ThresholdConfig`（阈值表，`compress/levels.go`）
```go
LevelThresholds() = {
  L1Tool: 16000,  L1Reasoning: 32000,   // decay 16K-48K  → L1（tool 12.5%-37.5% 占用）
  L2Tool: 48000,  L2Reasoning: 96000,   // decay 48K-128K → L2（37.5%-100%）
  L3Tool: 128000, L3Reasoning: 256000,  // decay ≥128K    → L3（≥100%）
  L4Reasoning: 512000,                  // reasoning decay ≥512K → L4（≥400%）
}
```
> 基线为 128K token 窗口（`DefaultConfig().WindowTokens = 128000`），阈值可按 `Config.Thresholds` 覆盖。

---

## 4. 类型约束矩阵（谁最多压到哪级）

`MaxLevelForType(stepType)`（`context/decay.go`）：

| step 类型 | 最大级别 | 理由 |
|-----------|---------|------|
| `user` | **L2 封顶** | 用户意图不可丢弃 |
| `tool` | **L3 封顶** | 工具结果再大也不删（结果依赖） |
| `reasoning` | L4 | 模型思考可丢 |
| `plan` | L4 | 已执行/已归档 |
| `failed` | L4 | 已失败/已覆盖 |
| 其他（default） | L3 | 兜底上限 |

配套工具：`context_lock_l0` / `context_unlock_l0`（`context/tools/tools.go`）——把 `RefRecord.LockL0` 置位后该 step 永不压缩（跳过 `ref.Level >= maxLvl || ref.LockL0` 检查）。

---

## 5. 触发时机（何时发生压缩）

| 触发源 | 位置 | 说明 |
|--------|------|------|
| **逐步骤 decay 检查** | `HybridManager.Ingest → perStepDecay`（`context/hybrid.go`） | 每次 Ingest 后对旧 step 计算 raw_decay 并决定是否降级；**sweet-spot 机制**：总 token 低于 `sweetSpotTokens` 时跳过 decay（保前缀缓存命中率）；另含 `maybeWarmup` 预热（接近甜区时异步预压缩旧步骤）、语义漂移检测（drift）与 `decayTolerance` 衰减回弹 |
| **窗口压力** | `BuildContext`（zone 划分） | 按 `windowTokens` 划分 tail/压缩区，压力大时压缩区变大 |
| **手动触发** | `TriggerCompression(ctx, CompressOpts{Force, TargetLevel, TaskGroupID})` | 调用方主动压缩，Force 强制、TargetLevel 指定级别 |
| **空闲整合** | `compress.IdleConsolidator`（`compress/idle.go`） | 连续 `IdleSteps` 无新步骤后轻量整合：pre-mark `pending_l4`、强化高频引用（ref_count>0）、合并相邻 L2 facts |
| **全局内省** | `context.Introspector`（`context/introspect.go`） | 5 步：①checkpoint 快照 → ②批压缩（`Engine.BatchCompress`）→ ③**L4 清理** → ④task_group 归档 → ⑤cache marker 更新 |

---

## 6. decay 与目标级别计算

1. **raw_decay（主路径，perStepDecay）**：`sumTokens(lastRef+1, currentStep)`——自该 step **上次被引用（`LastRefAtStep`）以来新增的 token 总数**，越久没被引用、中间产生的 token 越多，衰减越大。
2. **raw_decay（批路径，BatchCompress 简化估计）**：`step.TokenCount × 活跃 step 数`。
3. **有效衰减**：`ComputeDecay(ref, rawDecay, fileActive, taskGroupID, DecayConfig)`（`context/decay.go`）——含跨组引用调制与热度调制；`EffectiveDecay` 是忽略跨组/热度的旧版简化入口。
4. **目标级别**：`TargetLevel(decay, stepType, thresholds)`（`context/decay.go`）——按 §3.3 阈值表映射，再与 `MaxLevelForType` 取小。
5. **值不值得压**：`ShouldCompress(originalTokens)`——原文 ≥ 4000 tokens 才动手（至少省 2K，按 50% 缩减率估计）；`BatchCompress` 中 `estimatedSaved = tokens/2 < 2000` 直接跳过。

---

## 7. 压缩执行链路

### 7.1 模型链：小模型 → 主模型 → 机械兜底（`compress/engine.go`）
`CompressModelChain.Compress(level, prompt)`：
1. **小模型**优先（`Available()` 且质量校验通过）；
2. 失败回退**主模型**；
3. 都失败走 **`MechanicalCompress`（无 LLM 机械压缩）**。

### 7.2 质量校验（`validate`）
- 压缩结果不得比原文长；
- L2/L3 必须匹配掩码头正则 `^\[掩码 step_\d+\|原\d+t\|.+\|.+\]`；
- 不得为空。L3 目标下若链输出无掩码头，强制 `MechanicalL3`（用 step 元数据拼掩码）。

### 7.3 `CompressStep` 八步流程
1. 读当前 ref → 2. 读 L0 原文 → 3. 取**上一级**内容作为输入（`getContentAtLevel`：L3 以 L2 为输入）→ 4. 构建 prompt 并执行压缩（**L3 复用 L2 prompt**，目标级别=3 时 promptLevel=2）→ 5. 按级别写 store（L1/L2 分表；L3 写 L3 表并**同时写 L2 表**）→ 6. 更新 `ref.Level` → 7/8. 统计与返回。

> **L3 派生规则**：L3 是 L2 输出的第一行（掩码头），不重新生成——`strings.SplitN(compressed, "\n", 2)[0]`。

### 7.4 批量与空闲
- `BatchCompress`（全局）：遍历 `AllActiveStepIDs`，跳过 `LockL0` / 已达 `maxLvl` / 省不到 2K 的步骤，逐批 `CompressStep`。
- `IdleConsolidator.Consolidate`（轻量，不建 checkpoint / 不动 head buffer / 不刷 Redis）：只做 pre-mark `pending_l4`、强化高频、合并相邻 L2 facts。

---

## 8. 渲染与回溯（BuildContext）

`HybridManager.BuildContext(ctx, windowTokens)` 输出 `[]RenderedBlock`，分区顺序：

```
Zone 0.5  宪法区 <constitutional>（不可妥协规则，永远置顶）
Zone 1    head buffer（语义保持的近期头部）
Zone 2    hot facts（L2 事实按 strength 降序注入，A-8 数值隔离）
Zone 3    压缩历史（按 ref.Level 渲染：L1 精简 / L2 事实 / L3 掩码；经 RenderStepWithCache 缓存）
Zone 4    tail 原文（窗口尾部 step 以 L0 直出）
（transient 标记的步骤被排除；LockL0 的步骤始终 L0）
```

回溯能力：
- `Expand(stepID, full)`：默认 L1（denoised），`full=true` 返回 L0 原文；副作用：更新 refs（引用计数 → 影响后续 decay 方向）。
- `Surround(stepID, before, after)`：取某 step 前后文。
- `Search(ctx, query)`：RAG 检索历史。
- `SearchLongMem`：长时记忆检索（长期情况分类存）。

---

## 9. 存储落盘（StepStoreLike）

`context/store/{jsonl, sqlite, postgres}/` 三实现，统一 `StepStoreLike` 接口：
- **JSONL**：`{uuid}.jsonl`（steps）+ `.l1.jsonl` + `.l2.jsonl` + `.l3.jsonl` + refs 文件——零依赖、文件可读，本地开发首选；
- **SQLite**：单文件 + 事务 + 索引，`refs` 表含全部 RefRecord 字段；
- **PostgreSQL**：生产多实例，`refs` 表字段同 SQLite。

---

## 10. 关键文件索引

| 文件 | 内容 |
|------|------|
| `context/manager.go` | `ContextManager` 接口、`Mode`、`CompressOpts/CompressResult/ContextStats` |
| `context/config.go` | `Config`、`DefaultConfig()`（ModeHybrid + 128K）、`ThresholdConfig`、`IdleConsolidation` |
| `context/hybrid.go` | `HybridManager`：Ingest / BuildContext / TriggerCompression / 宪法区 / transient / hot facts |
| `context/step.go` | `StepRecord`、`RefRecord`、`L1Record`、`L2Record` |
| `context/decay.go` | `ComputeDecay` / `EffectiveDecay` / `TargetLevel` / `MaxLevelForType` |
| `context/compress/levels.go` | `LevelThresholds`、`TypeConstraintMatrix`、`ShouldCompress` |
| `context/compress/engine.go` | `CompressModelChain`、质量校验、`CompressStep` 八步、`BatchCompress` |
| `context/compress/idle.go` | `IdleConsolidator`（pending_l4 预标记 / 强化 / 合并 L2） |
| `context/compress/mechanical.go` | 无 LLM 机械压缩（含 `MechanicalL3` 掩码兜底） |
| `context/compress/prompts.go` | L1/L2/L3 prompt 模板 |
| `context/render_cache.go` | `RenderStepWithCache`（压缩历史渲染缓存） |
| `context/introspect.go` | 全局内省 5 步（L4 清理 / task_group 归档） |
| `context/tools/tools.go` | `context_lock_l0` / `context_unlock_l0` / `context_expand` / `context_trace` |
| `context/store/{jsonl,sqlite,postgres}/` | 后备存储三实现 |

---

## 11. 与其他方案的对比（防混淆）

| 方案 | 模型 | 信息可回溯性 | 成本 |
|------|------|-------------|------|
| **InferGlow L0-L4** | 全量落库 + 逐级降级 | L0-L3 全可逆，仅 L4 删除 | 压缩需 LLM（有机械兜底） |
| **ModeSummary** | 会话级摘要（对标 Reasonix compact.go） | 旧消息被摘要替换 | LLM 摘要 |
| **openhanako 瞬时简易压缩** | 滚动折叠窗口（tail 原文 + 旧区有损丢弃/摘要链） | 丢弃后不可恢复 | 零模型 |
| **经典滑动窗口** | 固定大小窗口，窗口外直接丢 | 不可回溯 | 零成本 |

**关键区分**：InferGlow 是「永不丢弃的渐进降级」，openhanako 是「窗口 + 窗口外折叠」；InferGlow 的 L4 丢弃只针对低价值类型（reasoning/plan/failed），user/tool 永远留痕。

---

## 12. 常见问题（FAQ）

- **为什么 user 步骤最多压到 L2？** `MaxLevelForType("user")=2`：用户意图是任务锚点，掩码化会丢失指令细节；要完全保留用 `context_lock_l0`。
- **L3 和 L2 什么关系？** L3 不重新生成，直接取 L2 输出的首行掩码头（`L3Record.Mask`），同时 L2 全量事实仍保留。
- **L4 丢弃后还能找回来吗？** 不能。L4 是唯一的真删除（`RemoveRef`），因此只有 reasoning/plan/failed 类型可达 L4。
- **压缩失败会怎样？** 模型链兜底到机械压缩；`BatchCompress` 对单步失败 `continue`，不影响其他步骤。
- **如何观察压缩状态？** `Stats()` 返回 `LevelCounts` / `ActiveSteps`（非 L4）/ `CompressedTokens`；`context_trace` 工具输出引用链。
- **怎么调参？** `Config.Thresholds`（改阈值表）、`Config.IdleConsolidation`（空闲整合开关与步数）、`CompressOpts.TargetLevel`（指定目标级别）。
