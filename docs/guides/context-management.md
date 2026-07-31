# 上下文管理使用指南

> 说明如何管理 Agent 对话上下文：选择压缩模式、初始化管理器、记录步骤、渲染上下文、使用 transient 与回溯。
> 对应示例：`examples/example_context.go`。

## 1. 核心概念

`context` 模块（包名 `contextmgr`）提供**可插拔、多模式**的上下文管理器，与 session 层解耦，通过**双写（Ingest）**与**上下文注入（BuildContext）**通信。

| 概念 | 说明 |
|------|------|
| `ContextManager` | 统一接口，所有模式实现它 |
| `Mode` | 运行模式（5 种） |
| `Config` | 配置（模式、窗口、压缩阈值、检索、回溯等） |
| `StepStoreLike` | 后备存储接口（步骤/引用/压缩记录） |
| `HybridManager` | ModeHybrid 实现：L0-L4 压缩 + RAG + 长时记忆 |

`ContextManager` 接口方法：

```go
Mode() Mode
Ingest(step StepRecord) error
BuildContext(ctx, windowTokens int) ([]RenderedBlock, error)
TriggerCompression(ctx, opts CompressOpts) (*CompressResult, error)
Search(ctx, query SearchQuery) ([]SearchHit, error)
SearchLongMem(ctx, query, category string, limit int) ([]LongMemRecord, error)
Expand(stepID int) (*ExpandResult, error)
Surround(stepID int, before, after int) ([]RenderedBlock, error)
Stats() ContextStats
Close() error
```

## 2. 5 种模式选择

| Mode | 常量 | 行为 | 适用场景 |
|------|------|------|---------|
| passthrough | `ModePassthrough` | 不做压缩，直接透传 | 短对话 / 需要前缀缓存命中率 |
| three_zone | `ModeThreeZone` | 三区会话适配器 | 兼容既有 ThreeZoneSession |
| summary | `ModeSummary` | 会话级摘要压缩（对标 Reasonix compact.go） | 超长对话、低成本压缩 |
| hybrid | `ModeHybrid` | 完整 L0-L4 压缩 + RAG + 长时记忆 | **默认**，功能最全 |
| assembly | `ModeAssembly` | 9 层上下文组装引擎（线A/C轨） | 高级组装场景 |

> 默认 `DefaultConfig()` 使用 `ModeHybrid`，窗口 128000 tokens。

## 3. 选择后备存储

`ContextManager` 需要存步骤/引用/压缩记录，通过 `StepStoreLike` 接口接驳。三种落地方案：

| 存储 | 构造 | 优点 | 适用 |
|------|------|------|------|
| JSONL | `jsonl.New(dir, uuid)` | 零依赖、文件可读 | 本地开发 / 单会话 |
| SQLite | `sqlite.New(dbPath)` | 单文件、事务、索引 | 本地持久化 |
| PostgreSQL | `postgres.New(ctx, dsn)` | 并发、可扩展、可查询 | 生产 / 多会话 |

**推荐**：本地开发用 JSONL，需要稳定持久化用 SQLite，生产多实例用 PostgreSQL。

## 4. 初始化与使用

```go
import (
	contextmgr "github.com/inferglow/context"
	"github.com/inferglow/context/store/jsonl"
)

func main() {
	// 1. 准备存储
	store, err := jsonl.New("./data", "session-1")
	if err != nil {
		panic(err)
	}

	// 2. 配置（可用默认值，再按需覆盖）
	cfg := contextmgr.DefaultConfig()
	cfg.Mode = contextmgr.ModeHybrid
	cfg.WindowTokens = 128000

	// 3. 创建管理器
	cm, err := contextmgr.NewHybridManager(cfg, store)
	if err != nil {
		panic(err)
	}
	defer cm.Close()

	// 4. 记录步骤（Ingest 双写）
	_ = cm.Ingest(contextmgr.StepRecord{
		Type:    "user",
		Role:    "user",
		Content: "请解释一下这个项目",
	})
	_ = cm.Ingest(contextmgr.StepRecord{
		Type:    "tool",
		Role:    "tool",
		Content: `{"files":[{"path":"README.md"}]}`,
		ToolName: "list_files",
	})

	// 5. 渲染上下文（供下一次 LLM 调用）
	blocks, err := cm.BuildContext(context.Background(), cfg.WindowTokens)
	if err != nil {
		panic(err)
	}
	for _, b := range blocks {
		fmt.Printf("[%d|L%d] %s\n", b.StepID, b.Level, b.Content)
	}

	// 6. 查询
	stats := cm.Stats()
	fmt.Printf("steps=%d active=%d tokens=%d\n", stats.TotalSteps, stats.ActiveSteps, stats.TotalTokens)
}
```

> 其他模式用对应构造函数：`NewPassthroughManager(cfg, store)`、`NewThreeZoneAdapter(cfg, store)`、`NewAssemblyManager(cfg, store)`。

## 5. 宪法区（Constitutional Zone）

`HybridManager` 的 `BuildContext` 在 head buffer 之前渲染 **Zone 0.5 宪法区**——一组始终置顶的动态约束条目，用于把不可妥协的规则注入每次上下文中。

```go
// 通过注意：宪法条目在 HybridManager 内部维护（constitutionalEntries）。
// 它在 BuildContext 中渲染为 <constitutional> 块，位于最前。
```

## 6. 压缩与渲染缓存

- `Ingest` 存储 L0 并创建初始引用（level=0, strength=1.0）。
- `BuildContext` 按引用将步骤渲染为 `RenderedBlock`；压缩历史经 `RenderStepWithCache(stepID, ref, cache, store)` 走缓存——命中走快路径（`cache.Get`），未命中走慢路径渲染并回填缓存。
- `TriggerCompression(ctx, opts)` 手动触发压缩（可用 `CompressOpts.Force` 强制、`TargetLevel` 指定级别、`TaskGroupID` 限定任务组）。

```go
res, err := cm.TriggerCompression(context.Background(), context.CompressOpts{
	Force:       true,
	TargetLevel: 2,
})
fmt.Printf("compressed=%d tokensSaved=%d\n", res.StepsCompressed, res.TokensSaved)
```

## 7. transient 与回溯

- **transient**：某些步骤不应进入最终上下文（如规划草稿、中间检索）。`MarkTransient(stepID, scope, round)` 标记后，该步骤从 `BuildContext` 排除。注意：`MarkTransient` 是 `HybridManager` 的具体方法（不在 `ContextManager` 接口上），需类型断言后调用。
- **回溯**：`Search` / `Expand` / `Surround` 用于检索与复原历史。

```go
// 标记某步骤为 transient（排除出上下文）——需类型断言到 HybridManager
if hm, ok := cm.(*contextmgr.HybridManager); ok {
	_ = hm.MarkTransient(3, "planning", 1)
}

// 搜索历史
hits, _ := cm.Search(ctx, contextmgr.SearchQuery{
	Query:    "项目结构",
	Limit:    5,
})

// 展开某步骤的原始内容
expanded, _ := cm.Expand(1)

// 取某步骤前后文
surround, _ := cm.Surround(1, 2, 2)
```

## 8. 完整可运行示例

请直接运行 `examples/example_context.go`：

```bash
cd examples
go run example_context.go
```

## 9. 关键文件

| 文件 | 内容 |
|------|------|
| `context/manager.go` | `ContextManager` 接口、`Mode`、`StepRecord`、`SearchQuery` 等 |
| `context/config.go` | `Config`、`DefaultConfig()`、压缩/检索/回溯配置 |
| `context/hybrid.go` | `HybridManager`（L0-L4、宪法区、transient、检索、stats） |
| `context/passthrough.go` | `PassthroughManager` |
| `context/threezone_adapter.go` | `ThreeZoneAdapter` |
| `context/assembly.go` | `AssemblyManager` |
| `context/render_cache.go` | `RenderStepWithCache`、`RenderedCache` |
| `context/registry.go` | `StepStoreLike` 接口 |
| `context/store/{jsonl,sqlite,postgres}/` | 三种后备存储实现 |