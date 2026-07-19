# 上下文管理模式切换与压缩增强 Spec

> 日期：2026-08-01
> 状态：待实施
> 关联：[上下文Zone结构设计](../archive/上下文Zone结构设计与压缩经济账_20260801.md)

---

## 一、背景

当前 InferGlow 支持四种上下文管理模式：

| 模式 | 说明 | 实现位置 |
|------|------|---------|
| `passthrough` | 无压缩，直接透传 | `context/passthrough.go` |
| `three_zone` | 三区适配器（简化版） | `context/threezone_adapter.go` |
| `summary` | Session 级摘要压缩（对标 Reasonix） | `context/summary.go` |
| `hybrid` | L0-L4 分级压缩（完整实现） | `context/hybrid.go` |

**当前问题**：
1. 模式在代码中硬编码为 `hybrid`，无法通过配置切换
2. 用户无法在甜点区阈值触发前手动触发压缩
3. 缺少模式切换的 TUI 命令

---

## 二、已完成工作

### 2.1 目录结构自动初始化 ✅

**实现位置**：`cli/config.go` `EnsureDataDirs()`

```go
func EnsureDataDirs(dataDir string) error {
    dirs := []string{
        filepath.Join(dataDir, "constitutional"),
        filepath.Join(dataDir, "sessions"),
        filepath.Join(dataDir, "memory"),
        filepath.Join(dataDir, "skills", "global"),
        filepath.Join(dataDir, "projects", "default", "skills"),
    }
    for _, d := range dirs {
        if err := os.MkdirAll(d, 0o755); err != nil {
            return fmt.Errorf("ensure data dir %s: %w", d, err)
        }
    }
    return nil
}
```

**目录结构**：
```
~/.inferglow/
├── config.json                  # 长期配置
├── constitutional/              # Zone 0.5 规则与元指令
│   └── rules.md
├── sessions/                    # Session JSONL 文件
│   └── index.jsonl
├── memory/                      # 长期记忆存储
├── skills/                      # 全局技能存储
│   └── global/
└── projects/                    # 项目级数据
    └── default/
        └── skills/
```

### 2.2 TUI 配置持久化 ✅

**实现位置**：`cli/config.go` `TUIConfig`

```go
type TUIConfig struct {
    Theme         string `json:"theme,omitempty"`          // "dark", "light", "auto"
    ShowReasoning bool   `json:"show_reasoning"`           // 显示 LLM 推理步骤
    MaxScrollback int    `json:"max_scrollback,omitempty"` // 最大历史记录行数
}
```

### 2.3 宪法区默认路径 ✅

**实现位置**：`cli/config.go` `DefaultCLIConfig()`

```go
Constitutional: filepath.Join(dataDir, "constitutional", "rules.md"),
```

### 2.4 /async-compress TUI 命令 ✅

**实现位置**：`cli/tui_commands.go` + `cli/memory_bridge.go`

**功能**：手动触发强制压缩，绕过甜点区阈值检查。

```go
// /async-compress 命令
case "async-compress":
    m.tuiHandleAsyncCompress(args)
    return nil, false

// ForceAsyncCompress 实现
func (b *MemoryBridge) ForceAsyncCompress(ctx context.Context) (*contextmgr.CompressResult, error) {
    opts := contextmgr.CompressOpts{
        Force:       true,
        TargetLevel: 2, // 压缩到 L2（事实保留）
    }
    return b.mgr.TriggerCompression(ctx, opts)
}
```

---

## 三、待实施工作

### 3.1 上下文管理模式配置（P1）

**目标**：通过配置文件切换上下文管理模式。

**配置字段**：
```go
// cli/config.go
type CLIConfig struct {
    // ... existing fields ...
    ContextMode string `json:"context_mode,omitempty"` // "passthrough", "three_zone", "summary", "hybrid"
}

// DefaultCLIConfig
func DefaultCLIConfig() CLIConfig {
    return CLIConfig{
        // ... existing defaults ...
        ContextMode: "hybrid", // 默认使用完整分级压缩
    }
}
```

**实现位置**：`cli/memory_bridge.go` `NewMemoryBridge()`

```go
func NewMemoryBridge(cfg CLIConfig, sessionID string) (*MemoryBridge, error) {
    // ... existing setup ...
    
    // 根据配置选择上下文管理模式
    var mgr contextmgr.ContextManager
    switch cfg.ContextMode {
    case "passthrough":
        mgr, err = contextmgr.NewPassthroughManager(cfg.ToContextConfig(), store)
    case "three_zone":
        mgr, err = contextmgr.NewThreeZoneAdapter(cfg.ToContextConfig(), store)
    case "summary":
        // 需要额外参数：session 和 summarizer
        mgr, err = contextmgr.NewSummaryManager(cfg.ToSummaryConfig(), store, session, summarizer)
    default: // "hybrid"
        mgr, err = contextmgr.NewHybridManager(cfg.ToContextConfig(), store)
    }
    
    // ... rest of setup ...
}
```

### 3.2 /mode TUI 命令（P2）

**目标**：在 TUI 中动态切换上下文管理模式。

**命令语法**：
```
/mode              # 显示当前模式
/mode <mode>       # 切换到指定模式
```

**实现位置**：`cli/tui_commands.go`

```go
case "mode":
    m.tuiHandleMode(args)
    return nil, false

func (m *chatTUI) tuiHandleMode(args string) {
    if args == "" {
        // 显示当前模式
        mode := m.bridge.ContextManager().Mode()
        m.commitLine(fmt.Sprintf("Current context mode: %s", mode))
        return
    }
    
    // 切换模式（需要重建 manager）
    m.commitLine(fmt.Sprintf("Switching to mode: %s", args))
    // TODO: 实现模式切换逻辑
}
```

**注意**：模式切换需要重建 `ContextManager`，可能涉及 session 数据迁移。建议先实现配置级别切换，TUI 动态切换作为后续增强。

### 3.3 首次运行引导（P2）

**目标**：首次启动时检测空 endpoint，提示用户输入配置。

**实现位置**：`cli/cmd/inferglow-cli/main.go`

```go
// 检测空 endpoint
if cfg.LLM.Endpoint == "" {
    fmt.Println("Welcome to InferGlow!")
    fmt.Println("Please configure your LLM endpoint:")
    // TODO: 交互式配置引导
}
```

### 3.4 配置热重载（P3）

**目标**：TUI 中 `/config reload` 命令，无需重启即可更新配置。

**实现位置**：`cli/tui_commands.go`

```go
case "config":
    if args == "reload" {
        // 重新加载配置文件
        newCfg, _, err := cli.LoadOrDefaultConfig("")
        if err != nil {
            m.commitLine(errorText(fmt.Sprintf("Config reload error: %v", err)))
        } else {
            m.cfg = newCfg
            m.commitLine(successText("Config reloaded."))
        }
        return nil, false
    }
    m.tuiHandleConfig(args)
    return nil, false
```

---

## 四、实施优先级

| 任务 | 优先级 | 预估工作量 | 依赖 |
|------|--------|-----------|------|
| 上下文管理模式配置 | P1 | 2-3 小时 | 无 |
| /mode TUI 命令 | P2 | 1-2 小时 | 3.1 |
| 首次运行引导 | P2 | 2-3 小时 | 无 |
| 配置热重载 | P3 | 1-2 小时 | 无 |

---

## 五、测试计划

### 5.1 单元测试

- [ ] `EnsureDataDirs` 创建所有必需目录
- [ ] `ForceAsyncCompress` 绕过甜点区检查
- [ ] 配置加载正确解析 `context_mode`

### 5.2 集成测试

- [ ] 启动时自动创建目录结构
- [ ] `/async-compress` 命令正常执行
- [ ] 不同 `context_mode` 配置下 agent 正常运行

### 5.3 手动测试

- [ ] 删除 `~/.inferglow/` 后重新启动，验证目录自动创建
- [ ] 执行 `/async-compress`，验证压缩效果
- [ ] 修改 `config.json` 中 `context_mode`，验证模式切换

---

## 六、风险与待定问题

| 风险 | 说明 | 缓解方案 |
|------|------|---------|
| 模式切换数据兼容性 | 不同模式的数据格式可能不兼容 | 模式切换时提示用户新建 session |
| Summary 模式依赖 | Summary 模式需要 `RewritableSession` 和 `Summarizer` | 提供默认实现或限制切换 |
| 配置热重载范围 | 部分配置无法热重载（如 LLM endpoint） | 明确标注可热重载的配置项 |

---

## 七、参考资料

- [上下文Zone结构设计](../archive/上下文Zone结构设计与压缩经济账_20260801.md)
- [12-quality-and-roadmap.md](../system-analysis/12-quality-and-roadmap.md)
- `context/manager.go` - ContextManager 接口定义
- `context/hybrid.go` - HybridManager 实现
