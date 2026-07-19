# maybeSliceCompact 实现计划

## 区域划分（sliceCompact 视角）

```
┌─ Zone 0: System Prompt（不可变）──────────────────────┐
│ base prompt + env + memory + skill index               │  ← boot 时构建，永不改
├─ Zone 0.5: Constitutional Zone（可 append）────────────┤
│ 宪法准则 / 操作禁止 / 规划模式约束                        │  ← 重组时可追加
│ 上下文管理提示词（动态版本）                              │
│ 贯穿 session 的 skill-like prompt                       │
│ 实现：session.Messages 中第二条 RoleSystem 消息           │
│ 标记：<constitutional>...</constitutional>              │
├─ Zone 1: Head（头部简述）──────────────────────────────┤
│ 初始任务声明 + 头部简述                                  │  ← A/C/D 触发改写
├─ Zone 2: Process（中间过程区）← sliceCompact 主战场 ────┤
│ step 序列，按 step_id:level 分级渲染                     │
│ 预热排期压缩这里的老 step → L1/L2/L3                     │
├─ Zone 3: Tail（尾部原文区）────────────────────────────┤
│ 最近 N 步 L0 原文                                        │  ← 不主动压缩
└─────────────────────────────────────────────────────────┘
```

## 重组流程（三问决策）

重组触发后，调 compress 模型执行三问：

```
输入：当前 step_id:level 列表 + 宪法区当前内容 + 头部简述

Q1: 宪法区是否需要追加？
  → 输出：新增的宪法条目（操作禁止/约束/提示词更新）
  → 动作：append 到 Zone 0.5 末尾

Q2: 任务 summary 是否转移？
  → 输出：新的头部简述（如果任务重点已变）
  → 动作：替换 Zone 1 的头部简述，原头部归档到 archive

Q3: step 区域的保留决策（id+level）
  → 输入：当前所有 step 的 id:level 列表
  → 输出：每个 step 的目标 level（可升级/保持/丢弃）
  → 动作：按指令批量执行压缩等级变更

三问结果合并 → 一次性重组 session 上下文
```

## 触发条件与动作映射

| # | 触发条件 | 动作 |
|---|---------|------|
| A | task_group 转移 + 超过准备点 | 头部区改写（Zone 1 重写） |
| C | promptTokens > sweetSpot | 超甜点区重组（Process 区批量压缩 + 头部改写） |
| D | LLM 调用 context_reorganize | 完整三问重组 |
| 预热 | promptTokens ≥ 0.8 × sweetSpot | 异步压缩 Process 区老 step → L1 → L2 |
| L3 | targetLevel=L3 | 从 L2 格式截断生成掩码，不走 LLM |
| 容忍 | LLM 引用 ≥ 5 个不同 step | 上调 sweetSpotTokens 10%（上限 1.5×） |

## 阶段一：Zone 0.5 宪法区

### 1.1 宪法区消息结构
- 文件：`internal/agent/compact.go`
- 在 session.Messages 中，system prompt（msgs[0]）之后插入一条 RoleSystem 消息
- 内容包裹在 `<constitutional>...</constitutional>` 标签中
- `pinnedPrefixLen()` 扩展：pin 住 msgs[0]（system）+ msgs[1]（constitutional，如果存在）

### 1.2 宪法区追加方法
- 文件：`internal/agent/compact.go`
- `appendConstitutional(entries []string) error`：追加新条目到宪法区末尾
- 重组时调用，不破坏 prefix cache（重组本身就要打破缓存）

### 1.3 boot.go 宪法区初始化
- 从现有 sysPrompt 中提取宪法级内容（UserDecisionPolicy、LanguagePolicy、操作禁止）
- 移入独立的宪法区消息
- `contextmgr.SystemPromptHint()` 移入宪法区（动态版本）

## 阶段二：contextmgr 补全

### 2.1 CompressOldSteps 方法
- 文件：`internal/contextmgr/contextmgr.go`
- 签名：`CompressOldSteps(ctx, beforeStep, targetLevel, keepRecent) (int, error)`
- 遍历 refs 中 stepID < beforeStep 且 level < targetLevel 的 step
- 按 ref_count/strength 排序（低优先先压缩），跳过最近 keepRecent 个
- 注入 Compressor 接口实现（从 agent 的 summarize 链路适配）

### 2.2 L3 从 L2 格式截断
- 文件：`internal/contextmgr/contextmgr.go`
- `TruncateL2ToL3(stepID) error`：读取 L2 facts，生成 `[掩码段 step_N | 原 Xk | N facts]` + 事实首行
- 纯机械截断，不走 LLM

### 2.3 Status 方法
- 签名：`Status() string`
- 返回各 level 的 step 计数 + 总估算 token

### 2.4 BuildTieredContext 方法
- 签名：`BuildTieredContext(windowTokens int) []ContextBlock`
- 包装 BuildContext，转换为 agent.ContextBlock 类型

### 2.5 SetCompressFunc 适配
- Manager 新增 `compressFn` 字段
- CompressOldSteps 内部使用

### 2.6 SetStepLevels 批量方法（重组 Q3 用）
- 签名：`SetStepLevels(ctx, decisions []StepLevelDecision) (int, error)`
- 接收 LLM 的 id+level 决策列表，批量执行压缩等级变更
- 支持升级（L0→L1→L2→L3）和降级（保持）

## 阶段三：maybeSliceCompact 触发逻辑

### 3.1 核心函数
- 文件：`internal/agent/compact.go`
- `maybeSliceCompact(ctx, usage)` 替代 maybeCompactTiered
- 三级阈值：
  - `prepareRatio = 0.8`：预热排期点
  - `reorganizeRatio = 1.0`：超过甜点区，触发重组
  - `emergencyRatio = 1.2`：紧急重组

### 3.2 预热排期（0.8 × sweetSpot）
- `promptTokens ≥ 0.8 × sweetSpotTokens` 时触发
- 异步 goroutine 压缩 Process 区老 step → L1 → L2
- `a.sliceCompactPending` 防止重复排期

### 3.3 超甜点区重组（> sweetSpot）
- 触发完整三问重组流程
- 调 compress 模型执行 Q1（宪法追加）+ Q2（任务 summary 转移）+ Q3（step 保留决策）
- 合并三问结果，一次性重组

### 3.4 头部区改写（子动作）
- 触发原因：
  - A: task_group 转移 + 超过准备点
  - C: 超甜点区重组附带执行
  - D: LLM 调用 context_reorganize
- 动作：重写 Zone 1 头部简述，原头部归档

### 3.5 动态容忍上调
- LLM 输出引用 ≥ 5 个不同 step_id 时
- 上调 sweetSpotTokens 10%（上限 1.5×）
- `a.sweetSpotTolerance` 跟踪上调比例

## 阶段四：重组三问实现

### 4.1 reorganizePrompt 构建
- 文件：`internal/agent/compact.go`
- 构建重组 prompt，包含：
  - 当前宪法区内容
  - 当前头部简述
  - step_id:level 列表（从 ctxManager.Status() 获取）
  - 三问指令

### 4.2 reorganize(ctx) 方法
- 调 compress 模型执行重组 prompt
- 解析三问输出：
  - Q1 输出：新增宪法条目列表 → appendConstitutional()
  - Q2 输出：新头部简述 → 替换 Zone 1，原头部归档
  - Q3 输出：[]StepLevelDecision → ctxManager.SetStepLevels()
- 合并执行，原子更新

## 阶段五：context_reorganize 工具（LLM 主动触发）

### 5.1 工具定义
- 文件：`internal/contextmgr/tools.go`
- `ReorganizeTool`
- Name: `context_reorganize`
- ReadOnly: false
- 参数：`{ "focus": string }` — focus 指定重组重点

### 5.2 工具逻辑
- 调用 agent 的 reorganize(ctx) 方法
- 返回重组结果摘要

### 5.3 注册到 boot.go
- addContextTools 闭包中注册

## 阶段六：stream() 上下文组装

### 6.1 BuildTieredContext 集成
- 文件：`internal/agent/agent.go` 的 stream() 方法
- 当 ctxManager != nil 且 promptTokens > sweetSpot 时
- 用 ctxManager.BuildTieredContext 组装 Process 区
- Zone 0 + Zone 0.5 保持原样（从 session.Messages 直接取）
- Zone 1（头部简述）+ Zone 2（压缩 step）+ Zone 3（tail）由 BuildContext 组装

## 阶段七：编译验证

- `go build ./...` 验证编译通过
- 检查所有接口方法实现完整

## 文件修改清单

| 文件 | 变更 |
|------|------|
| `internal/contextmgr/contextmgr.go` | +CompressOldSteps +TruncateL2ToL3 +Status +BuildTieredContext +SetCompressFunc +SetStepLevels +compressFn 字段 |
| `internal/contextmgr/tools.go` | +ReorganizeTool |
| `internal/agent/compact.go` | +maybeSliceCompact +reorganize +reorganizePrompt +appendConstitutional +pinnedPrefixLen 扩展 |
| `internal/agent/agent.go` | +sweetSpotTolerance 字段 + stream() 上下文组装 |
| `internal/boot/boot.go` | 宪法区初始化 + 注册 ReorganizeTool |
# maybeSliceCompact 实现计划

## 设计决策归纳

| # | 触发条件 | 动作 |
|---|---------|------|
| 1 | `promptTokens ≥ 0.8 × sweetSpot` | **预热排期**：异步对老 step 做 L1，再做 L2 |
| 2 | L3 掩码生成 | **从 L2 截断**生成，不额外调 LLM（格式截断） |
| 3 | step 数过多 + task 重点转移 + 超过准备点 | **头部区改写**：原头部按序加尾标缓存，取新头部简述 |
| 4 | 上下文超过甜点区一定量（如 tool 大输入） | **触发重组** |
| 5 | LLM 自己有感知 | 允许调用 `context_reorganize` 工具主动重组 |
| 6 | LLM 确认保留 steps 足够多 | **甜点区阈值容忍增加**（动态上调 sweetSpotTokens） |

## 阶段一：contextmgr 补全

### 1.1 CompressOldSteps 方法
- 文件：`internal/contextmgr/contextmgr.go`
- 签名：`CompressOldSteps(ctx, beforeStep, targetLevel, keepRecent) (int, error)`
- 逻辑：遍历 refs 中 stepID < beforeStep 且 level < targetLevel 的 step，按 ref_count/strength 排序（低优先先压缩），跳过最近 keepRecent 个，逐个调用 CompressStep
- 需要注入 Compressor 接口实现（从 agent 的 summarize 链路适配）

### 1.2 L3 从 L2 格式截断
- 文件：`internal/contextmgr/contextmgr.go`
- 新增 `TruncateL2ToL3(stepID) error`：读取 L2 facts，生成掩码格式 `[掩码段 step_N | 原 Xk | N facts]` + 事实首行摘要
- 不需要 LLM 调用，纯机械截断

### 1.3 Status 方法
- 文件：`internal/contextmgr/contextmgr.go`
- 签名：`Status() string`
- 返回各 level 的 step 计数 + 总估算 token

### 1.4 BuildTieredContext 方法
- 文件：`internal/contextmgr/contextmgr.go`
- 签名：`BuildTieredContext(windowTokens int) []ContextBlock`
- 包装 BuildContext，转换为 agent.ContextBlock 类型

### 1.5 SetCompressFunc 适配
- 文件：`internal/contextmgr/contextmgr.go`
- Manager 新增 `compressFn` 字段，CompressOldSteps 内部使用
- 适配 Compressor 接口

## 阶段二：maybeSliceCompact 触发逻辑

### 2.1 核心函数
- 文件：`internal/agent/compact.go`
- 新增 `maybeSliceCompact(ctx, usage)` 替代 maybeCompactTiered
- 三级阈值：
  - `prepareRatio = 0.8`：预热排期点（异步 L1 → L2）
  - `reorganizeRatio = 1.0`：超过甜点区，触发重组
  - `emergencyRatio = 1.2`：紧急重组（tool 大输入场景）

### 2.2 预热排期（0.8 × sweetSpot）
- 检查 `promptTokens ≥ 0.8 × sweetSpotTokens`
- 异步 goroutine：`go a.ctxManager.CompressOldSteps(ctx, beforeStep, L1, keepRecent)`
- L1 完成后再排期 L2（通过 channel 或 sync.WaitGroup 跟踪状态）
- 用 `a.sliceCompactPending` 防止重复排期

### 2.3 L3 从 L2 截断
- 在 CompressOldSteps 中，当 targetLevel=L3 时调用 TruncateL2ToL3
- 不走 LLM，纯机械生成掩码

### 2.4 头部区改写
- 触发条件：step 数 > 阈值（如 50）且 task_group 发生转移 且 promptTokens > prepareRatio × sweetSpot
- 动作：对头部区（非 tail）的老 step 批量提升到 L2/L3
- 头部简述：调 compress 模型生成一段头部摘要，替换原头部内容
- 原头部按序加尾标缓存到 archive

### 2.5 超甜点区重组
- 触发条件：`promptTokens > sweetSpotTokens`（超过甜点区）
- 动作：批量将 L0 step 压缩到 L2，释放空间
- 与头部区改写的区别：重组更激进，可能涉及 tail 区前面的所有 step

### 2.6 动态容忍上调
- 当 LLM 输出中引用了足够多不同 step（如 ≥ 5 个不同 step_id）
- 上调 sweetSpotTokens 10%（`a.sweetSpotTokens = int(float64(a.sweetSpotTokens) * 1.1)`）
- 上限：原始值的 1.5 倍
- 用 `a.sweetSpotTolerance` 跟踪当前上调比例

## 阶段三：context_reorganize 工具

### 3.1 工具定义
- 文件：`internal/contextmgr/tools.go`
- 新增 `ReorganizeTool`
- Name: `context_reorganize`
- ReadOnly: false（会修改压缩等级）
- 参数：`{ "focus": string, "aggressive": bool }`
- focus 指定重组重点（如 "redis config"），aggressive 控制压缩激进程度

### 3.2 工具逻辑
- 调用 CompressOldSteps 批量压缩
- 根据 focus 参数决定哪些 step 保留较高 level
- 返回重组结果摘要

### 3.3 注册到 boot.go
- 在 addContextTools 闭包中注册 `NewReorganizeTool`

## 阶段四：stream() 上下文组装接入

### 4.1 BuildTieredContext 集成
- 文件：`internal/agent/agent.go` 的 stream() 方法
- 当 ctxManager != nil 且 promptTokens > sweetSpot 时
- 用 ctxManager.BuildTieredContext 组装的 blocks 替换原始 session messages
- 保持 system prompt + tail 不变

## 阶段五：编译验证

- `go build ./...` 验证编译通过
- 检查所有接口方法实现完整

## 文件修改清单

| 文件 | 变更 |
|------|------|
| `internal/contextmgr/contextmgr.go` | +CompressOldSteps +TruncateL2ToL3 +Status +BuildTieredContext +SetCompressFunc +compressFn 字段 |
| `internal/contextmgr/tools.go` | +ReorganizeTool |
| `internal/agent/compact.go` | +maybeSliceCompact（替换 maybeCompactTiered）+ 预热/重组/容忍逻辑 |
| `internal/agent/agent.go` | +sweetSpotTolerance 字段 + stream() 上下文组装 |
| `internal/agent/compact.go` | ContextManager 接口已有（上一轮扩展），确认方法签名匹配 |
| `internal/boot/boot.go` | 注册 ReorganizeTool |
