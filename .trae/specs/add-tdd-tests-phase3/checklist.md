# Checklist

- [x] `model/internal/ssestream` 测试覆盖 EffectiveHTTPClient/MapRole/ParseDataLine/RunLines
- [x] `orchestrator/agent/internal/turnloop` 测试覆盖 TurnLoop 状态机、Preempt/Reset/Snapshot
- [x] `builtins/actions` 测试覆盖 grep_executor（GrepRunner 注入）
- [x] `builtins/actions` 测试覆盖 image_generate（ImageGenerator 注入）
- [x] `builtins/actions` 测试覆盖 list_dir（路径穿越防护）
- [x] `builtins/actions` 测试覆盖 speech_to_text（SpeechTranscriber 注入）
- [x] `builtins/actions` 测试覆盖 text_to_speech（SpeechSynthesizer 注入）
- [x] `builtins/actions` 测试覆盖 memory_forget/recall/remember（mock Store）
- [x] `builtins/actions` 测试覆盖 run_skill（inline/subagent 模式）
- [x] `builtins/actions` 测试覆盖 sub_agent（spawn_agent 逻辑）
- [x] `builtins/actions` 测试覆盖 task_tracker（TaskStore CRUD + 4 个 Action）
- [x] `go test -count=1`（三个模块全部通过）
- [ ] GitHub CI 测试通过（等待 CI 运行中）