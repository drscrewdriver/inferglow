# Tasks

- [x] Task 1: 为 `model/internal/ssestream` 补全测试（16 测试，全部通过）
  - [x] SubTask 1.1: `ssestream_test.go` — EffectiveHTTPClient/MapRole/ParseDataLine 工具函数
  - [x] SubTask 1.2: RunLines 正常流程（mock body + parse 函数）
  - [x] SubTask 1.3: RunLines 边界（EOF、context cancel、parse 返回 stop）

- [x] Task 2: 为 `orchestrator/agent/internal/turnloop` 补全测试（11 测试，全部通过）
  - [x] SubTask 2.1: `turnloop_test.go` — TurnPhase 枚举和 String 方法
  - [x] SubTask 2.2: TurnLoop 状态机转换（idle→planning→active→idle）
  - [x] SubTask 2.3: Preempt/IsPreempted/Reset 并发安全
  - [x] SubTask 2.4: Snapshot 和 ErrCannotPreemptIdle 错误路径

- [x] Task 3: 为 `builtins/actions` 补全基础 Action 测试（42 测试，全部通过）
  - [x] SubTask 3.1: `grep_executor_test.go` — GrepRunner 注入、正常/错误路径
  - [x] SubTask 3.2: `image_generate_test.go` — ImageGenerator 注入、MockImageGenerator
  - [x] SubTask 3.3: `list_dir_test.go` — 路径穿越防护、正常/错误路径
  - [x] SubTask 3.4: `speech_to_text_test.go` — SpeechTranscriber 注入、MockSpeechTranscriber
  - [x] SubTask 3.5: `text_to_speech_test.go` — SpeechSynthesizer 注入、MockSpeechSynthesizer

- [x] Task 4: 为 `builtins/actions` 补全记忆相关 Action 测试（17 测试，全部通过）
  - [x] SubTask 4.1: `memory_forget_test.go` — mock Store、Archive 成功/失败/未找到
  - [x] SubTask 4.2: `memory_recall_test.go` — search/read/list 三种操作
  - [x] SubTask 4.3: `memory_remember_test.go` — 保存/缺少必填字段

- [x] Task 5: 为 `builtins/actions` 补全复杂 Action 测试（50 测试，全部通过）
  - [x] SubTask 5.1: `run_skill_test.go` — inline/subagent 模式、skill 未找到
  - [x] SubTask 5.2: `sub_agent_test.go` — 正常生成、缺少 task、无 flow context
  - [x] SubTask 5.3: `task_tracker_test.go` — TaskStore CRUD、4 个 Action 的 Execute

- [x] Task 6: 验证所有测试通过
  - [x] SubTask 6.1: 运行 `go test -count=1`（三个模块全部通过）
  - [x] SubTask 6.2: 提交并推送至 GitHub 触发 CI（c6ce150）

# Task Dependencies
- Tasks 1-5 无依赖，可并行执行
- Task 6 依赖 Tasks 1-5