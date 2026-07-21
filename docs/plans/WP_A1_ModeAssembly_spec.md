# WP-A1 Spec — ModeAssembly 骨架接入（线A 探路）

> 分文件：`inferglow-G1` · 分支 `feat/g1-context-assembly`
> 上级：`docs/plans/00-integrated-master-spec.md`（线A） · `docs/plans/BRANCH_MASK.G1.md`
> 目标：以「新增 Mode 枚举 + Manager 实现 + Registry 注册」接入线A 9层装配，不触碰冻结的 `ContextManager` 接口、不新建 `orchestrator/assembly/`。

## 1. 任务范围（wp-a1）

- [x] 新增 `ModeAssembly` 枚举常量（`context/manager.go`）
- [x] 新增 `AssemblyManager` 结构骨架：Wire `cfg + store` 依赖 + 空/最小实现（`context/assembly.go`）
- [x] 新增工厂 `NewAssemblyManager(cfg, store) (ContextManager, error)`
- [x] 在 `cli/memory_bridge.go` Registry 注册区登记 `ModeAssembly`（仅改注册区，不碰其它逻辑）
- [x] 单测闭环（`context/assembly_test.go`）
- [x] `go test ./context/... ./action/...` 通过
- [x] 单 commit 提交

## 2. 实现决策（可追溯依据）

| 决策点 | 选择 | 依据 |
| --- | --- | --- |
| 模式字符串 | `"assembly"` | 与 `passthrough/three_zone/summary/hybrid` 命名同构 |
| 接入方式 | 新增 Mode + Manager + Registry | `manager.go` 接口冻结，`BRANCH_MASK` 硬约束 #2 |
| 依赖载体 | 沿用 `StepStoreLike`（共享 store） | 与 `HybridManager`/`Registry.SwitchMode` 同构，热切换零中断 |
| A-11 审计 | 仅保留 `AssemblyAudit`，其余类型 YAGNI | spec 线A 第 680 行「仅保留 AssemblyAudit，其余 YAGNI」 |
| Ingest 空实现 | 仅 `AppendStep` + 维护 active ref | 使 `BuildContext`/`Stats` 装配路径可见，后续 wp-a2..a5 在此类型上充实 |

## 3. 改动文件

| 文件 | 改动 | 归属 |
| --- | --- | --- |
| `context/manager.go` | 新增 `ModeAssembly Mode = "assembly"` 常量 | 白名单 engine |
| `context/assembly.go` | 新增 `AssemblyManager` / `NewAssemblyManager` / `AssemblyAudit`/`AuditEntry`/`LayerAudit` | 白名单 context/** |
| `context/assembly_test.go` | 新增单片测 | 白名单 context/** |
| `cli/memory_bridge.go` | `SwitchMode` 内 Registry 注册 `ModeAssembly` | 白名单限定区 |

## 4. 单测覆盖的装配路径

- `TestAssemblyManagerImplementsInterface` — 编译期断言满足冻结接口
- `TestAssemblyManagerFactoryAndMode` — 工厂返回 `ModeAssembly` 且 Wire 共享 store
- `TestAssemblyManagerIngestBuildContextRoundTrip` — Ingest→BuildContext→Stats 装配路径
- `TestAssemblyManagerSearchNotImplemented` — wp-a1 未实现路径返回显式错误
- `TestAssemblyManagerRegistrySwitch` — Registry 热切换 hybrid→assembly，store 内容存活；未注册模式报错

## 5. 验收

- `go test ./context/... ./action/...` 全绿
- 未触碰任何黑名单文件（`BRANCH_MASK.G1.md`）
- wp-a1 提交追溯：`git log --oneline --grep='mode assembly skeleton'`（单 commit）

## 6. 后续子阶段（串行追踪）

- wp-a2：衰减重构（`context/decay.go` 内串行）
- wp-a3：retrieval 增强（`context/retrieval/**`）
- wp-a4：render / headBuffer 生成侧
- wp-a5：C轨（记忆纵横双写）