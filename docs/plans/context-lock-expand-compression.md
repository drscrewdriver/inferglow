# Context Lock, Expand Default L1, and Compression Enhancement

## Summary

Three interrelated changes: (1) `context_expand` defaults to L1 with optional L0 via `full=true`, (2) `context_lock_l0`/`context_unlock_l0` tools for forced L0 retention with append-only audit trail, (3) compression irreversibility enforcement with L2 header format validation.

## Changes

### 1. Expand Default L1

- `context/manager.go` — `ContextManager.Expand(stepID int, full bool)` interface change
- `context/hybrid.go` — `full=false` returns L1 (denoised), `full=true` returns L0 original; updates `ref_count`/`strength` to slow compression decay
- `context/assembly.go`, `passthrough.go`, `summary.go`, `threezone_adapter.go` — adapt to new signature
- `context/tools/tools.go` — `ContextExpandTool.Execute` passes `in.Full`; hint output shows level label
- `examples/example_context.go` — example updated to new signature

### 2. LockL0 Mechanism

**Runtime flag:**
- `context/step.go` — `RefRecord.LockL0 bool` (only field on RefRecord; `LockL0Reason`/`LockL0At` deliberately omitted)

**Audit trail:**
- `context/step.go` — `AuditRecord` type `{ts, action, step_id, reason, detail}`
- `context/registry.go` — `StepStoreLike.AppendAudit(AuditRecord)`
- `context/store/store.go` — `StepStore.AppendAudit(AuditRecord)`
- `context/store/jsonl/jsonl_store.go` — `{uuid}.audit.jsonl` append-only
- `context/store/sqlite/sqlite_store.go` — `audit` table
- `context/store/postgres/postgres_store.go` — `audit` table

**Tools:**
- `context/tools/tools.go` — `ContextLockL0Tool` (sets `LockL0=true`, appends audit log, returns report)
- `context/tools/tools.go` — `ContextUnlockL0Tool` (sets `LockL0=false`, appends audit log, returns report)

### 3. Compression Irreversibility

- `context/hybrid.go` — `perStepDecay` and `TriggerCompression` skip `LockL0=true` steps
- `context/compress/engine.go` — `BatchCompress` skips `LockL0=true` steps

### 4. L2/L3 Compression Format

- `context/compress/prompts.go` — L2 prompt simplified to "提取文件路径、配置键值、错误消息原文、决策结论"; `token_count` field added to `[User]` section
- `context/compress/engine.go` — `maskHeaderRegex` validates `[掩码 step_N|原X t|tool|params]` header; L3 L2-chain fallback -> `MechanicalL3` when LLM fails
- `context/compress/mechanical.go` — `MechanicalL3` format: `[掩码 step_N|原X t|tool|params] 工具:xxx 总Nt`

### 5. Test

- `context/phase0_test.go` — `fakeStore.AppendAudit` added

## Files Changed (18 + 1 new)

```
M  context/assembly.go
M  context/compress/engine.go
M  context/compress/mechanical.go
M  context/compress/prompts.go
M  context/hybrid.go
M  context/manager.go
M  context/passthrough.go
M  context/phase0_test.go
M  context/registry.go
M  context/step.go
M  context/store/jsonl/jsonl_store.go
M  context/store/postgres/postgres_store.go
M  context/store/sqlite/sqlite_store.go
M  context/store/store.go
M  context/summary.go
M  context/threezone_adapter.go
M  context/tools/tools.go
M  examples/example_context.go
A  docs/plans/context-lock-expand-compression.md
```