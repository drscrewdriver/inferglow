# WP-A3 Spec — retrieval 增强：Recency 实现 + 三路接入主装配（线A · feat/g1-context-assembly）

> 分文件：`inferglow-G1` · 分支 `feat/g1-context-assembly`
> 上级：`docs/plans/00-integrated-master-spec.md`（线A · A-7）· `docs/plans/BRANCH_MASK.G1.md`
> 目标：补齐 `RecencySearcher` 具体实现并注册进 `FusionRetriever`，使融合从「二路/单路」升为可测的「三路」；在 context 内部（`NewHybridManager` 注入点）将三路检索接入主装配路径 `HybridManager.Search`，以「EnableFusion 门控 + 旧路径保壳」换取零回归。
> 范围确认（用户）：**仅 A-7 接入 + Recency 实现**；A-8（数值隔离）/ A-9（同组回溯）留后置。
> 评审决策（用户拍板）：① `EnableFusion` **默认开**（测试阶段无生产债务，先开验证，不行看数据翻转）；② 语义路保持 `NoopEmbedder` 占位（依赖设施/可用性检查，graceful 不出问题即可）；③ 融合归一化**沿用既有 `fuseScores`，本轮不改，后效观察**。

## 0. 前置对齐（本阶段前已完成）

- wp-a1（`ModeAssembly` 骨架，`6be9d53`）、wp-a2（衰减重构，`f57cd91`）已提交。
- `context/retrieval/` 已存在 `semantic.go`(VectorStore/cosine)、`bm25.go`(BM25+倒排)、`fusion.go`(三路融合 0.5/0.3/0.2、阈值 0.35)、`tokenizer.go`、`embed.go`(`Embedder`/`NoopEmbedder`)、`vectorstore.go`(`VectorStoreBackend`)。
- 硬约束：`ContextManager` 接口签名冻结；不新建 `orchestrator/assembly/`；不改 `go.mod`（零新依赖）；不碰黑名单（`cli/**` 除 `memory_bridge.go`、`builtins/**`、`flow/ server/ memory/ model/ schema/`）。

## 1. 现状缺口（基于工作树证据）

| 缺口 | 证据 | 影响 |
| --- | --- | --- |
| `RecencySearcher` 只有接口、**无实现类** | `fusion.go:40` 定义接口；全仓无实现 | `NewFusionRetriever` 的 recency 入参恒可为 nil，三路融合实际跑成≤二路 |
| 现有消费方 recency=nil | `cli/memory_bridge.go:123` `NewFusionRetriever(nil, bm25, nil, NoopEmbedder)`（黑名单，只读参照） | 即便 fusion 存在，recency 权重 0.20 从未生效 |
| context 主装配未消费 retrieval | `hybrid.go:505 Search` 为朴素 `strings.Contains` 关键词匹配（Score 恒 1.0） | 衰减/压缩主线走不到三路融合 |

## 2. 任务范围（wp-a3）

- [ ] `context/retrieval/recency.go`：新增 `RecencyIndex`（实现 `RecencySearcher`），`Add`/`Search`，recency+strength 归一加权打分
- [ ] `context/config.go`：新增 `RetrievalConfig` + `DefaultRetrievalConfig()`；`Config.Retrieval` 字段 + `DefaultConfig()` 填充
- [ ] `context/hybrid.go`：`HybridManager` 增 `fusion`/三索引字段；`NewHybridManager` 按 `cfg.Retrieval.EnableFusion` 装配；`reindex(ctx)` 从 store 回填三索引；`Search` 门控委派 fusion（`SearchResult→SearchHit` 映射）+ 旧路径保壳；mutation 处置脏标
- [ ] `context/retrieval/recency_test.go`：RecencyIndex 单测闭环
- [ ] `context/retrieval/fusion_test.go`：三路 vs 二路可测差异 + 权重 0.20 手算核对（mock 三路，不依赖真实 embedder）
- [ ] `go test ./context/... ./action/...` 全绿 + `go vet ./context/...` 无告警
- [ ] 单 commit 提交

## 3. 核心取舍（实现决策，可追溯）

| 决策点 | 选择 | 依据 |
| --- | --- | --- |
| RecencyIndex 形态 | 内存「Add 后 Search」结构，同构 `VectorStore`/`BM25Index` | 与检索包既有模式一致；纯函数可测；零外部依赖 |
| Recency 打分 | `score = RecencyW×(lastRefAtStep/maxStep) + StrengthW×(strength/maxStrength)`，归一到 [0,1] | A-7「LastRefAtStep + Strength 加权」；归一后可直接与 fusion 阈值 0.35 比较 |
| Recency 内部权重 | `RecencyW=0.6 / StrengthW=0.4`（外置 `RetrievalConfig`） | 时效优先、强度次之；外置可调，沿用 wp-a2「系数外置」纪律 |
| 「now」基准 | `maxStep = max(已见 lastRefAtStep)`，由 `Add` 维护 | `RecencySearcher.Search(ctx,limit)` 签名无 currentStep 入参，内部自洽 |
| 空门控 | 索引为空 → 返回 nil（与 VectorStore/BM25 一致） | 杜绝空库噪声；fusion `maxScoreVal` 对 nil 天然中性 |
| 主装配接入策略 | `EnableFusion` 门控，**默认 true（测试阶段常开）**；fusion 为 nil 或关闭时 `Search` 回退旧朴素路径（保壳兜底） | 测试阶段无生产债务，先开验证、不行看数据翻转；保壳兜底确保异常时可一行回退 |
| 融合归一化 | **沿用既有 `fuseScores`（按路 max 归一），本轮不改** | 归一化调优后效观察，先证「二路→三路」差异，避免过度设计 |
| 语义路 | 运行期以 `NoopEmbedder` 占位（语义贡献 0），关键词+recency 双路实际生效 | 真实 embedder 属 OT-4（他线/后置）；与 `memory_bridge` 现状一致，不阻塞本轮 |
| 索引刷新 | 懒加载：首次 fusion Search 时 `reindex`；mutation（AppendStep/UpsertRef）置 `fusionDirty=true` | 收缩回归面与复杂度；增量索引列后置 |
| `SearchResult→SearchHit` 映射 | `Snippet=Text(≤200)`；`Level`/`Type` 经 `GetRef`/`stepType` 补齐 | 复用现有 `renderStepContent`/`stepType`，不改接口 |

## 4. 公式与验收点

- Recency 单路打分（A-7）：
  - `recency_norm = lastRefAtStep / maxStep`（`maxStep<=0 → 0`）
  - `strength_norm = strength / maxStrength`（`maxStrength<=0 → 0`）
  - `score = RecencyW × recency_norm + StrengthW × strength_norm`，降序、截断 limit
- 三路融合（`fusion.go` 既有，权重 `[0.50,0.30,0.20]`、阈值 0.35）：
  - `fused[id] = Σ_path (score/maxPath) × weight[path]`
- **可测差异（fusion_test 手算，Threshold=0.2 构造）**：
  - semantic `{A:1.0, B:0.2}`、keyword `{A:1.0, B:0.2}`、recency `{B:1.0}`
  - 二路（recency=nil）：A=0.5+0.3=0.80；B=0.1+0.06=0.16 → **B 被阈值过滤**，结果 `[A]`
  - 三路（recency 接入）：B=0.16 + 1.0×0.20=**0.36**（recency 贡献恰为权重 0.20）→ **B 被救回**，结果 `[A(0.80), B(0.36)]`
  - 验收：recency 使 B 由「丢弃」变「召回」，且增量 == `Weights[2]×normRecency`，证明 0.20 生效、二路→三路有可测差异

## 5. 改动文件

| 文件 | 改动 | 归属 |
| --- | --- | --- |
| `context/retrieval/recency.go` | 新增 `RecencyIndex`（`recencyEntry`/`NewRecencyIndex`/`Add`/`Search`） | 白名单 `context/retrieval/**` |
| `context/config.go` | 新增 `RetrievalConfig`+`DefaultRetrievalConfig()`；`Config.Retrieval`；`DefaultConfig()` 填充 | 白名单 `context/**` |
| `context/hybrid.go` | `HybridManager` 增 `fusion`/`vsIndex`/`bm25Index`/`recencyIndex`/`fusionDirty` 字段；`NewHybridManager` 门控装配；新增 `reindex(ctx)`；`Search` 门控委派+保壳；mutation 置脏 | 白名单 `context/**` |
| `context/retrieval/recency_test.go` | 新增 RecencyIndex 单测 | 白名单 `context/retrieval/**` |
| `context/retrieval/fusion_test.go` | 新增三路 vs 二路差异 + 权重手算 + 阈值过滤（mock 三路） | 白名单 `context/retrieval/**` |
| `docs/plans/WP_A3_Retrieval_spec.md` | 本文档 | 文档 |

未改动：`context/manager.go`（接口冻结）、`cli/memory_bridge.go` 及一切黑名单文件、`go.mod`。

## 6. 单测覆盖

- `recency_test.go`
  - `TestRecencyIndexOrdering` — 按 lastRefAtStep 降序（新者优先）
  - `TestRecencyIndexStrengthWeight` — 同步长下 strength 高者分高（0.4 权重生效）
  - `TestRecencyIndexEmpty` — 空索引 → nil
  - `TestRecencyIndexLimit` — 截断到 limit
  - `TestRecencyIndexNormalization` — 分数落在 [0,1]；单条目 maxStep 自洽
- `fusion_test.go`
  - `TestFusionThreeWayVsTwoWay` — §4 手算用例：recency 救回 B，增量==0.20
  - `TestFusionRecencyWeight` — 固定 semantic/keyword，仅变 recency，分数差==`Weights[2]×norm`
  - `TestFusionThresholdFilter` — 低于阈值过滤
  - `TestFusionNilRecencyCompat` — recency=nil 时不 panic，退化为二路（保壳兼容现状）

## 7. 验收

- `go test ./context/... ./action/...` 全绿（含 retrieval 子包新增测试）
- `go vet ./context/...` 无告警
- 未触碰任何黑名单文件（`BRANCH_MASK.G1.md`）
- `EnableFusion=true`（默认）下 `HybridManager.Search` 经三路融合返回分级 `SearchHit`；置 `false` 时回退旧朴素路径（保壳兜底可一行回退）
- wp-a3 提交追溯：`git log --oneline --grep='retrieval recency'`（单 commit）

## 8. 后置项（明确不做，记账）

- 真实 `Embedder` 接入以激活语义路（OT-4 向量检索，他线）。
- 增量索引（mutation 级 `Add`）替代懒加载全量 `reindex`（性能优化）。
- A-8 数值隔离（Layer 6 按 Strength/RefCount 原始数值排序）。
- A-9 同组回溯展开（Layer 8 注入 task_group 历史 top-K，依赖 `BacktrackConfig`）。
- 融合归一化策略调优（本轮沿用既有按路 max 归一，后效观察后再议）。
- `cli/memory_bridge.go` 消费方升级 recency（黑名单，归并后由宿主统一处理）。

## 9. 后续子阶段（串行追踪）

- wp-a4：render / headBuffer 生成侧
- wp-a5：C 轨（记忆纵横双写）
