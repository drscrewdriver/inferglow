# WP-A2 Spec — 衰减重构（线A · feat/g1-context-assembly）

> 分文件：`inferglow-G1` · 分支 `feat/g1-context-assembly`
> 上级：`docs/plans/00-integrated-master-spec.md`（线A） · `docs/plans/BRANCH_MASK.G1.md`
> 目标：主 spec A-5（跨组老化）/ A-6（DecayTrace + 系数外置）/ A-13（H 维度热度），三段**串行**落地、全部算法收敛于 `context/decay.go`，以「保壳 + 富函数 + Enabled 门控」换取零回归。

## 0. 前置对齐（本阶段前已完成）

- 上一子阶段 `WP_A1_ModeAssembly_spec.md`（wp-a1）随 `feat(context): mode assembly skeleton` 提交。
- `ContextManager` 接口签名冻结；不新建 `orchestrator/assembly/`；不改 `go.mod`（零新依赖）。

## 1. 任务范围（wp-a2）

- [x] `RefRecord` 补 `CrossGroupRefs`/`Heat` 字段（`context/step.go`）
- [x] `Config` 新增 `DecayConfig`/`GroupModConfig`/`HeatModConfig` + `DefaultDecayConfig()`（`context/config.go`）
- [x] `context/decay.go` 单文件实现 `crossGroupMod`/`heatMod`/`DecayTrace`/`ComputeDecay` + `applyRecallBoost`，`EffectiveDecay` 改保壳
- [x] 引用写路径门控累积 Heat/CrossGroupRefs（`context/tolerance.go`、`context/hybrid.go`）
- [x] 迁移 `context/hybrid.go` 两处调用点为 `ComputeDecay`（perStepDecay / TriggerCompression）
- [x] 单测闭环（`context/decay_test.go`）
- [x] `go test ./context/... ./action/...` 通过 + `go vet ./context/...` 无告警
- [x] 单 commit 提交

## 2. 核心取舍（实现决策，可追溯）

| 决策点 | 选择 | 依据 |
| --- | --- | --- |
| 回归策略 | 旧 `EffectiveDecay` 保壳 + 新 `ComputeDecay` 富函数 | 签名冻结；3 调用点仅 2 处迁移，`compress/engine.go` 靠保壳天然兼容 |
| 系数来源 | 全部外置到 `DecayConfig`，零值回退旧魔数 | A-6 可观测/可调；零 cfg 与旧公式逐阶数值相等 |
| H 维度门控 | `heatMod` 前置 `!Enabled \|\| heat==0 → 1.0` | 杜绝「Heat 缺失/未启用时落入 <40 区 → 全员加速 30%」静默漂移 |
| 跨组门控 | `crossGroupMod` 前置 `!Enabled \|\| 同组 → 1.0` | 同组归一，避免误导性跨组加速 |
| Schema 落库 | SQLite/Postgres `refs` 列迁移**后置** | 热量 0 门控已中和；默认 JSONL 自动持久化；为收缩回归面 |
| `groupCompleted` 参数 | 保留但不再生效，注释标 deprecated | 保壳签名兼容，避免炸调用点 |
| A-5 crossRefs 累积源 | 引用写路径（门控）累积；验收在纯函数层以显式入参断言 | `taskGroupID` 运行时无推进者，不强接真实生产计算，避免语义漂移 |

## 3. 公式与验收点

- `group_mod = 1.0 + DistanceW × distance / (1.0 + CrossRefW × crossRefs)`（A-5）
  - 同组 = 1.0；d1/cross5 = 1.15；d1/cross0 = 1.30；d3/cross10 = 1.30；d3/cross0 = 1.90
- `heat_mod`（A-13）：`>=70 → 0.7`（显著区）；`<40 → 1.3`（衰减区）；`[40,70) → 1.0`；`0 / disabled → 1.0`
- `effective = raw_decay × ref_mod × file_mod / strength × group_mod × heat_mod`（A-6，spec §6.2）
  - `ref_mod = 1/(1 + ref_count × RefModWeight)`；`file_mod = fileActive ? FileModWeight : 1`；`strength` clamp `<0.1 → 0.1`

## 4. 改动文件

| 文件 | 改动 | 归属 |
| --- | --- | --- |
| `context/step.go` | `RefRecord` 补 `CrossGroupRefs`/`Heat` 字段（具名字面量仅 3 处，不炸编译） | 白名单 context/** |
| `context/config.go` | 新增 `DecayConfig`/`GroupModConfig`/`HeatModConfig` + `DefaultDecayConfig()`；`Config.Decay`；`DefaultConfig()` 填充 | 白名单 context/** |
| `context/decay.go` | 新增 `ComputeDecay`/`DecayTrace`/`crossGroupMod`/`heatMod`/`applyRecallBoost`；`EffectiveDecay` 改保壳 | 白名单 context/** |
| `context/tolerance.go` | `ProcessCitationsWithTolerance` 引用命中处方调用 `applyRecallBoost` | 白名单 context/** |
| `context/hybrid.go` | `ProcessCitations` 引用命中处 + 两处衰减调用点迁移 `ComputeDecay` | 白名单 context/** |
| `context/decay_test.go` | 新增单片测 | 白名单 context/** |
| `docs/plans/WP_A2_Decay_spec.md` | 本文档 | 文档 |

未改动：`context/compress/engine.go`（保壳天然兼容）、`manager.go` 接口签名、其余黑名单文件。

## 5. 单测覆盖（`context/decay_test.go`）

- `TestCrossGroupMod` — A-5 全部 5 个验收点 + 同组 with crossrefs + disabled→1.0 + 负距离 clamp→1.0
- `TestHeatMod` — 三区边界（70/40 含界、100/69/39/1/0）+ disabled→1.0
- `TestComputeDecayZeroCfgEquivalence` — 零 cfg 与旧公式在 fileActive=false/true、strength clamp、强引用下逐阶数值相等 + trace group/heat 中性
- `TestComputeDecayEnabledChain` — 双维度同时启用时全链路手算核对
- `TestEffectiveDecayCompatShell` — 保壳==零 cfg `ComputeDecay`；`groupCompleted` 忽略
- `TestDecayTraceFields` — 各字段填充断言，`TargetLevel`/`Reason` 留给调用点回填
- `TestApplyRecallBoost` — disabled 不变、同组/cross 组、heat cap 100
- `TestTargetLevelBackfill` — 调用点回填模式自洽（decay==trace.Effective）
- `TestDecayTraceZeroCfgNeutral`（并入等价测试）— 零 cfg group/heat 恒 1.0

## 6. 验收

- `go test ./context/... ./action/...` 全绿（context 含全部子包）
- `go vet ./context/...` 无告警
- 未触碰任何黑名单文件（`BRANCH_MASK.G1.md`）
- wp-a2 提交追溯：`git log --oneline --grep='decay refactor crossGroupMod'`（单 commit）

## 7. 后置项（明确不做，记账）

- SQLite/Postgres `refs` 表加 `heat`/`cross_group_refs` 列 + ALTER 迁移（约 6 处 + 存量迁移）。
- `DecayTrace` 落盘审计（本轮仅内存/审计用途）。
- StepRecord `Transient`（A-12）与 9-layer assembly 真正消费该衰减（后续 wp-a3/a4/a5）。

## 8. 后续子阶段（串行追踪）

- wp-a3：retrieval 增强（`context/retrieval/**`）
- wp-a4：render / headBuffer 生成侧
- wp-a5：C 轨（记忆纵横双写）