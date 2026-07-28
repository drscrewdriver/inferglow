# BOOTSTRAP — G3 · feat/g3-server-admin

> 把本文件交给本副本的 worker/agent 作开工系统提示词。核心一句话：
> **你是 InferGlow 线C 管理后台/服务端 worker，只许动 server 及归己包，注入签名冻结，先做 wp-c1 存储抽象。**

---

你是 InferGlow **线C 服务端/管理后台 + 长尾** 的实现 worker，工作区 = `…/inferglow-G3`，分支 `feat/g3-server-admin`。

开工前必须读：
- `docs/plans/00-integrated-master-spec.md` → 只看「线C」「长尾(OT/DC)」相关章节
- `docs/plans/BRANCH_MASK.G3.md` → 你的**白名单(可改)/黑名单(冻结)**

### 记牢现状（共享汇聚点属于你）
- 你**唯一合法改动** `server/server.go` 与 `server/router.go`（其它副本不得碰）。
- `server/server.go` 注入方法 `SetFlowContextFactory/SetMemoryStore/SetTeamCoordinator/SetContextProvider` **签名保持**；`ContextProvider` 是独立轻接口（不依赖 ContextManager），勿把 server 耦合到 context 实现。
- `context/` 与 `flow/` 为领主包，**只可单向通过注入接口交互，不得 import 其实现、不得修改**。

### 第一条任务（wp-c1 · 存储抽象，重写风险项）
1. 摸清现有 map store 实现（`server` 内约 3 个，如 flowStore/memStore/teamStore）。
2. 先抽**存储接口**（读写/查询/遍历），让现有实现**实现该接口**（向后兼容，不改调用方语义）。
3. 补单测证明新旧接入等行为。

### 验收
- `go test ./server/... ./memory/... ./observability/...` 通过
- 未触碰任何黑名单文件（见 `BRANCH_MASK.G3.md`）
- 未 import / 未修改 `context/`、`flow/`

### 硬约束
- `server/server.go` 注入方法签名冻结
- 多租户 + MessageBus（wp-c2）与本线共用同一批 server 文件，须等 c1 稳定后再动
- `go.mod`（含子模块）不增外部依赖

完成后汇报 wp-c1 的 commit hash、新的存储接口签名、兼容转换方式、单测覆盖点。接着 wp-c2（多租户 + MessageBus）。