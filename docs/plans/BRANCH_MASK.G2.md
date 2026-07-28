# BRANCH_MASK — G2 · feat/g2-flow-ports

> 三副本并行锁，共同 spec： `inferglow/docs/plans/00-integrated-master-spec.md`
> 本副本只许动 flow 作用域，其余一律「只读、不改、不重构」。共享基座冻结，见末尾。

## 职责范围
**线B Flow 增强**：端口模型（Stage/FlowDef 显式端口与元数据声明）、registry 扩展、端点接线。
现状基础：`flow/flowdef/` 子包与 `flow/stage/registry.go` 已存在，在**其上扩展**而非新建同名文件。

## 允许区（白名单 · 可改写）
```
flow/**                        # 全部 Flow 实现（flowdef、stage/registry、graph、exec……）
```

## 禁止区（黑名单 · 冻结）
```
context/**                     # 归 G1
server/**  memory/**  observability/**   # 归 G3
cli/**  desktop/  imbridge/  builtins/**   # 基座/独立线，冻结
model/  schema/                # 稳定基座，冻结
orchestrator/agent/** action/**           # 归 G1
docs/plans/00-integrated-master-spec.md   # 宿主
go.mod / go.sum                # 不增外部依赖
```

## 硬约束
1. **不得新建** `flow/flowdef.go` 或与既有 `flow/flowdef/` 子包同名的文件——在子包内扩展（spec B-6 的并入说明 / 合规做法：文件名带端口后缀）。
2. `flow/stage/registry.go` 只新增 StageMeta/端口声明，不改既有 `StageFunc` 注册语义。
3. 端口模型语义需与 G1 的 `ContextManager` 注册机制互通，但**不 import / 不修改 context**（只按 spec 定义抽象，避免与 G1 冲突）。

## 子阶段（串行）
- wp-b1：StageMeta 端口声明（入参/出参/Meta），扩展 stage
- wp-b2：FlowDef 端口化 + FlowRegistry（flowdef 子包内）
- wp-b3：端点/executor 接线与示例

## 验收
- `go test ./flow/...` 全绿。
- 不触碰任何黑名单文件。
- merge 目标顺序：**G1 → G3 → G2**（本分支 **最后** 合入，承接前两者借机消冲突）。

## 共享冻结区（三副本共同不可改）
`context/manager.go` 接口签名、`flow/stage` 既有注册语义、`model/`、`schema/`、`docs/plans/00-integrated-master-spec.md`。