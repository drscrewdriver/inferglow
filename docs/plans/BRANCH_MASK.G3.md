# BRANCH_MASK — G3 · feat/g3-server-admin

> 三副本并行锁，共同 spec： `inferglow/docs/plans/00-integrated-master-spec.md`
> 本副本只许动 server 及归己的新包，其余（尤其 context/flow 领主包）一律「只读、不改」。

## 职责范围
**线C 管理后台 / AgentScope 集成** + 存储抽象 + 多租户 + MessageBus + multiagent 编排 + eval。
持有 **共享汇聚文件** `server/server.go` 与 `server/router.go`（唯一合法改动方）。

## 允许区（白名单 · 可改写）
```
server/**                       # router.go / server.go / handlers* / dashboard.html (共享汇聚点，归本分支独占)
memory/**                       # graph 存储（长尾并入本线）
observability/**                # 采集器
eval/**                         # 评估平台（新增包）
messagebus/**  storage/**       # 新增抽象（若落地，建独立包，勿塞进 server）
orchestrator/multiagent/**      # 若需编排新增
```

## 禁止区（黑名单 · 冻结）
```
context/**                      # 归 G1（仅可通过 ContextProvider 轻接口单向交互，不 import context 实现）
flow/**                         # 归 G2
cli/**  desktop/  imbridge/  builtins/**   # 基座/独立线，冻结
model/  schema/                 # 稳定基座，冻结
orchestrator/agent/** action/** # 归 G1
docs/plans/00-integrated-master-spec.md   # 宿主
go.mod / go.sum                 # 不增外部依赖（含各子 go.mod）
```

## 硬约束
1. `server/server.go` 注入依赖保持：`SetFlowContextFactory/SetMemoryStore/SetTeamCoordinator/SetContextProvider` 等**签名不变**；`ContextProvider` 是独立轻接口（不依赖 ContextManager），勿耦合 context 实现。
2. 存储抽象是**重写风险项**：先抽接口再落实现（map store→接口），旧 3 个 map store 转换要向后兼容。
3. 多租户 + MessageBus（C-2/C-3）共用同一批 server 层文件——在本分支内串行，勿与他人抢。
4. 不 import / 不修改 `context/` 与 `flow/`。

## 子阶段（串行，本线强耦合须有序）
- wp-c1：存储抽象（map store → 接口层，兼容转换）
- wp-c2：多租户 + MessageBus（同一批 server 文件，紧接 c1）
- wp-c3：管理后台 API / dashboard / eval 平台接线
- 长尾并入：DC-6 / OT 系列（独立小项，可穿插）

## 验证
- `go test ./server/... ./memory/... ./observability/...` 全绿。
- 不触碰任何黑名单文件。
- merge 目标顺序：**G1 → G3 → G2**（本分支第二顺位，承接 G1 后合入）。

## 共享冻结区（三副本共同不可改）
`context/manager.go` 接口签名、`server/server.go` 注入方法签名（由本分支唯一合法改动）、`model/`、`schema/`、`docs/plans/00-integrated-master-spec.md`。