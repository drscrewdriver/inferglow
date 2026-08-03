# Add TDD Tests Phase 3 Spec

## Why

Phase 2 补全了 10 个包的测试，但仍有 2 个内部包完全无测试覆盖，且 `builtins/actions` 中有 11 个源文件缺少独立单元测试。这些是用户可交互的 Action 实现，缺少测试意味着回归风险较高。

## What Changes

### 类别 A：内部工具包（2 包，各 1 文件）
- `model/internal/ssestream/ssestream.go`：SSE 流处理工具（RunLines, EffectiveHTTPClient, MapRole, ParseDataLine）
- `orchestrator/agent/internal/turnloop/turnloop.go`：Agent 轮次状态机（TurnLoop, TurnPhase, TurnState）

### 类别 B：builtins/actions 缺失的 Action 测试（11 文件）
- `builtins/actions/grep_executor.go`：GrepRunner 注入的 Action
- `builtins/actions/image_generate.go`：ImageGenerator 注入的 Action（已有 MockImageGenerator）
- `builtins/actions/list_dir.go`：目录列表 Action（含路径穿越防护逻辑）
- `builtins/actions/memory_forget.go`：归档记忆 Action
- `builtins/actions/memory_recall.go`：搜索/读取/列出记忆 Action（含 BM25 搜索、子串回退）
- `builtins/actions/memory_remember.go`：保存记忆 Action
- `builtins/actions/run_skill.go`：运行技能 Action（inline/subagent 两种模式）
- `builtins/actions/speech_to_text.go`：语音转文字 Action（已有 MockSpeechTranscriber）
- `builtins/actions/sub_agent.go`：子 Agent 生成 Action
- `builtins/actions/task_tracker.go`：任务跟踪器（TaskStore + 4 个 Action：task_add/update/list/delete）
- `builtins/actions/text_to_speech.go`：文字转语音 Action（已有 MockSpeechSynthesizer）

## Impact
- Affected code: 2 个内部包 + 11 个 Action 源文件
- 预期新增 ~40 个测试函数
- No functional changes to production code
- `builtins/actions` 的测试使用 mock 注入，不依赖外部服务

## ADDED Requirements

### Requirement: 内部工具包测试
每个导出函数/类型的关键路径 SHALL 有测试覆盖。

#### Scenario: ssestream 正常路径
- **WHEN** 调用 EffectiveHTTPClient(nil)
- **THEN** 返回非 nil 的 *http.Client 且超时为 DefaultTimeout

#### Scenario: ssestream 边界
- **WHEN** 调用 ParseDataLine 处理非 data 行
- **THEN** 返回 ("", false)

#### Scenario: turnloop 状态机
- **WHEN** 按 idle→planning→active→idle 顺序转换
- **THEN** Phase() 返回对应状态

#### Scenario: turnloop 抢占
- **WHEN** 在 planning/active 时调用 Preempt
- **THEN** 状态回到 idle，IsPreempted=true

#### Scenario: turnloop 错误
- **WHEN** 在 idle 时调用 Preempt
- **THEN** 返回 ErrCannotPreemptIdle

### Requirement: Action 测试
每个 Action 的 Execute 方法 SHALL 有测试覆盖，包括正常路径和错误路径。

#### Scenario: Action 正常执行
- **WHEN** 提供有效输入
- **THEN** Execute 返回 OK=true 的结果

#### Scenario: Action 缺少必填参数
- **WHEN** 缺少必填字段
- **THEN** Execute 返回 OK=false 的错误结果

#### Scenario: Action 依赖不可用
- **WHEN** 注入的 runner/generator/transcriber 为 nil
- **THEN** Execute 返回对应的错误结果

#### Scenario: 委托调用失败
- **WHEN** 注入的依赖返回错误
- **THEN** Execute 包装错误并返回

#### Scenario: TaskStore 持久化
- **WHEN** 调用 Add/Update/Delete/List
- **THEN** 操作正确且数据一致