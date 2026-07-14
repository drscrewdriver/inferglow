# example_flow_pause.go — Flow 暂停/恢复/检查点

## 中文说明

演示 Flow 引擎的 Pause/Resume/Checkpoint 机制，支持长时间运行流程的中断恢复。

### 核心概念
- **Execution.Pause()**：暂停当前执行，返回 PausePoint 记录断点
- **Flow.Resume()**：从 PausePoint 恢复执行
- **FileCheckpointStore**：文件系统持久化检查点
- **WithAutoCheckpoint**：自动在 Pause 时保存检查点
- **Flow.ResumeFromSnapshot()**：从快照重建 Execution 并恢复

### 运行方式
```bash
cd examples
go run example_flow_pause.go
```

### 示例输出
```
=== Example 1: Simple Pause and Resume ===
Full execution result: ...
Paused at step: step_a
Resumed from step_b, result: ...

=== Example 2: Checkpoint Persistence ===
Auto-checkpoint saved: ...
Checkpoint loaded from store
Resumed from snapshot, result: ...
```

---

## English Description

Demonstrates Flow Pause/Resume/Checkpoint mechanisms for interruptible long-running workflows.

### Key Concepts
- **Execution.Pause()**: Pause execution and record breakpoint
- **Flow.Resume()**: Resume from PausePoint
- **FileCheckpointStore**: File-system checkpoint persistence
- **WithAutoCheckpoint**: Auto-save checkpoint on Pause
- **Flow.ResumeFromSnapshot()**: Rebuild and resume from snapshot

### Run
```bash
cd examples
go run example_flow_pause.go
```