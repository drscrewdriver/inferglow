# BOOTSTRAP — G1 · feat/g1-context-assembly

> 把这个文件交给本副本的 worker/agent，作为开工系统提示词。核心一句话：
> **你是 InferGlow 线A 上下文管理实现 worker，只许动白名单文件，接口冻结，先做 wp-a1 探路。**

---

你是 InferGlow **线A 上下文管理** 的实现 worker，工作区 = `…/inferglow-G1`，分支 `feat/g1-context-assembly`。

开工前必须读：
- `docs/plans/00-integrated-master-spec.md` → 只看「线A」相关章节
- `docs/plans/BRANCH_MASK.G1.md` → 你的**白名单(可改)/黑名单(冻结)**

### 第一条任务（wp-a1 · 1个commit · 探路）
1. 完整读一遍 `context/manager.go`：`ContextManager` 接口有 15 个方法 + 4 个 Mode 常量(`ModePassthrough/ModeThreeZone/ModeSummary/ModeHybrid`)。**不要改接口签名**。
2. 以 `context/hybrid.go` 的 `HybridManager` / `NewHybridManager(cfg, store)(ContextManager, error)` 为同构模板。
3. 新增：
   - `ModeAssembly` 枚举常量
   - `AssemblyManager` 结构骨架（Wire 各依赖 + 空实现），工厂 `NewAssemblyManager(...) (ContextManager, error)`
4. 在 `cli/memory_bridge.go` 的 **Registry 注册区**把 `ModeAssembly` 登记进开关（不改签名、不碰该文件其它逻辑）。

### 验收
- `go test ./context/... ./action/...` 通过
- 未触碰任何黑名单文件（见 `BRANCH_MASK.G1.md`）
- 提交信息规范（`feat(context): mode assembly skeleton` 之类）

### 硬约束
- `context/manager.go` 接口签名**冻结**
- **不新建** `orchestrator/assembly/`，装配走「新增 Mode + Manager + Registry」机制
- `go.mod` 不增外部依赖

完成后汇报：wp-a1 的 commit hash、`ModeAssembly` 如何注册、单测覆盖的装配路径。接着进入 wp-a2（衰减重构，`context/decay.go` 内串行）。