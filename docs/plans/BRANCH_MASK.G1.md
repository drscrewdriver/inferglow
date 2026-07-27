# BRANCH_MASK — G1 · feat/g1-context-assembly

> 三副本并行锁.spec： `inferglow/docs/plans/00-integrated-master-spec.md`
> 本文件是**工作禁区约束**。这个副本只许作用域内文件，其余一律「只读、不改名、不重构」。

## 职责范围
**线A 上下文管理（P0 主线）** + 衰减重构 + C轨(ModeAssembly) 。
两条可单独交付子线：A线(9层 retrieval/render)，C轨(记忆纵横双写)。

## 允许区（白名单 · 可改写）
```
context/**                    # 全部上下文实现(L0-L4/headBuffer/Zone/Summary/drift/render/retrieval)
cli/memory_bridge.go          # 仅 Registry 注册区（ContextManager/Mode 注册），不碰其余逻辑
orchestrator/agent/**         # agent engine 与 loop_guard（上下文联动处）
action/**                     # action 工具链（cache 等）
context/retrieval/**          # semantic / bm25 / fusion
```

## 禁止区（黑名单 · 冻结，绝对不碰）
```
flow/**                       # 归 G2
server/**                     # 归 G3
memory/**                     # graph 属于 G3 长尾
observability/**              # 归 G3
desktop/  imbridge/  builtins/**   # 基座/独立线，冻结
model/  schema/               # 稳定基座，冻结
cli/**  (除 memory_bridge.go) # TUI 基座，冻结
docs/plans/00-integrated-master-spec.md   # 宿主
go.mod / go.sum               # 除法显必须，不增外部依赖
```

## 硬约束（不可违反）
1. `context/manager.go` 的 `ContextManager` 接口签名**冻结**——只可新增 Mode 枚举/实现，不改 15 个方法签名。
2. 9层装配以「新增 `ModeAssembly` 枚举 + Manager 实现 + Registry 注册」接入，**不新建 orchestrator/assembly/**，不改 manager.go 接口。
3. `cli/memory_bridge.go` 之外的所有 cli 文件只读。
4. 单测闭环：每个子阶段必须补 test 并通过 `go test ./context/...`。

## 子阶段（串行，按此顺序交付）
- wp-a1：`ModeAssembly` 枚举 + Manager 骨架 + Registry 注册（空实现）— 探路，1 个 commit
- wp-a2：衰减重构（EffectiveDecay→crossGroupMod + DecayTrace + 系数外置 + H 维度），decay.go 单文件内串行
- wp-a3：retrieval 增强（semantic/bm25/fusion 三路 + tokenizer）
- wp-a4：render / headBuffer 生成侧
- wp-a5：C轨（记忆纵横双写）

## 验收
- `go test ./context/...` 全绿。
- 不触碰任何黑名单文件。
- merge 目标顺序：**G1 → G3 → G2**（本分支第一顺位合入 master）。