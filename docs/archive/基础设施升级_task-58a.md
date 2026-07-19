# inferflow 代码上下文感知实现计划

## 探索总结

### 当前架构
- **引擎层** (`engine.go`): 通过 `actExt.Register()` 注册 action，在 `runFlow()` 中初始化 flowContext
- **Coder 阶段** (`coder.go`): 当前仅接收 issue 描述，盲目生成代码，无仓库上下文
- **Action 系统**: `file_read` 和 `file_write` 已实现，具备 `AllowedDirs` 和 `MaxBytes` 安全限制
- **工作流** (`bug_fix_workflow.yaml`): clone_repo → coder，中间缺少代码分析阶段
- **Bash 执行**: `execBash` helper 已实现，可复用执行 shell 命令

### 关键发现
1. `file_read` 已内置安全机制：`AllowedDirs` 白名单 + `MaxBytes` 限制（默认 1MB）
2. `file_write` 需要审批（`ApprovalRequired: true`），适合受控写入
3. `execBash` 模式可复用于文件发现（find/grep）
4. workspace 路径通过 `fctx.GetValue("workspace")` 获取

---

## 方法概述

### 核心设计原则
1. **渐进式上下文构建**: 先目录树概览 → 再针对性读取关键文件
2. **Token 预算控制**: 总上下文限制在 50KB 以内（约 12K tokens）
3. **智能文件选择**: 优先读取 issue 提及的文件 + 错误栈相关文件
4. **安全隔离**: file_read/file_write 严格限制在 workspace 目录

### 三阶段改造
1. **注册层**: 在 engine 初始化时注册 file_read/file_write action
2. **分析层**: 新增 analyze 阶段，构建代码上下文
3. **编码层**: 改造 coder 接收结构化上下文，生成精准修改

---

## 实现步骤

### 阶段 1: 注册 file_read/file_write action（30 分钟）

**文件**: `inferflow/runtime/engine/engine.go`

**步骤 1.1**: 在 `runFlow()` 中注册 file_read action（第 558-561 行附近）
```go
// 在 actExt.Register(actions.NewBashExecutorAction(...)) 之后添加
if workspace != "" {
    actExt.Register(actions.NewFileReadAction(actions.FileReadConfig{
        AllowedDirs: []string{workspace},
        MaxBytes:    512 * 1024, // 512KB per file
    }))
    actExt.Register(actions.NewFileWriteAction(actions.FileWriteConfig{
        AllowedDirs: []string{workspace},
    }))
}
```

**步骤 1.2**: 在 `Resume()` 中同样注册（第 725-728 行附近）
```go
// 复用相同的注册逻辑
if workspace != "" {
    actExt.Register(actions.NewFileReadAction(actions.FileReadConfig{
        AllowedDirs: []string{workspace},
        MaxBytes:    512 * 1024,
    }))
    actExt.Register(actions.NewFileWriteAction(actions.FileWriteConfig{
        AllowedDirs: []string{workspace},
    }))
}
```

**验证**: 编译通过，action 可在 flowContext 中调用

---

### 阶段 2: 创建 analyze 阶段（2 小时）

**新文件**: `inferflow/runtime/stage/builtin/analyze.go`

**步骤 2.1**: 定义 analyze 阶段函数签名
```go
func Analyze(ctx context.Context, in stage.Inputs, fctx flow.FlowContext) (stage.Outputs, error)
```

**步骤 2.2**: 实现三层上下文收集策略

**Layer 1 - 目录结构概览**（~2KB）
```go
// 使用 find 生成目录树，限制深度和数量
cmd := `find . -type f -name "*.go" -o -name "*.py" -o -name "*.js" | head -100`
stdout, _, err := execBash(ctx, fctx, cmd, workspace)
```

**Layer 2 - 智能文件选择**（~5KB）
```go
// 从 issue 描述中提取文件路径和关键词
// 优先级：
// 1. issue 中明确提及的文件路径
// 2. 错误栈中的文件名
// 3. 与 bug 类型相关的文件（如 "null pointer" → 检查最近修改的文件）

// 使用 grep 搜索相关文件
keywords := extractKeywords(title + " " + description)
cmd := fmt.Sprintf(`grep -r -l "%s" --include="*.go" . | head -20`, keywords)
```

**Layer 3 - 精准文件读取**（~40KB 预算）
```go
// 使用 file_read action 读取选中的文件
// 总预算控制：50KB - 已用上下文
remainingBudget := 50*1024 - len(treeOutput) - len(grepOutput)

for _, file := range selectedFiles {
    result, err := fctx.ExecuteAction(ctx, "file_read", map[string]any{
        "path":      filepath.Join(workspace, file),
        "max_bytes": min(remainingBudget/len(selectedFiles), 10*1024),
    })
    // 累加到上下文，超出预算时停止
}
```

**步骤 2.3**: 构建结构化输出
```go
return stage.Outputs{
    "code_context": map[string]any{
        "tree":       treeOutput,      // 目录结构
        "files":      fileContents,    // map[path]content
        "keywords":   keywords,        // 提取的关键词
        "total_size": totalBytes,      // 总字节数
    },
    "issue_summary": title + "\n" + description,
}, nil
```

**步骤 2.4**: 在 `register.go` 中注册
```go
r.Register("analyze", Analyze)
```

---

### 阶段 3: 改造 coder 阶段（1 小时）

**文件**: `inferflow/runtime/stage/builtin/coder.go`

**步骤 3.1**: 修改系统提示词，接收结构化上下文
```go
const coderSystemPrompt = `You are a code modification assistant.
Given:
1. Bug report (title + description)
2. Code context (directory tree + relevant file contents)

Generate a JSON object with:
- "files": map of path → NEW content (only modified files)
- "summary": description of changes
- "reasoning": why these changes fix the bug

IMPORTANT:
- Use the provided code context to understand existing code structure
- Only return files that need modification
- Preserve existing code style and patterns
- Paths must be relative to repo root`
```

**步骤 3.2**: 修改 `buildCoderTaskDescription` 注入上下文
```go
func buildCoderTaskDescription(in stage.Inputs) string {
    var sb strings.Builder
    
    // 1. Issue 描述
    sb.WriteString("## Bug Report\n")
    sb.WriteString("Title: " + in["title"].(string) + "\n")
    sb.WriteString("Description: " + in["description"].(string) + "\n\n")
    
    // 2. 代码上下文（来自 analyze 阶段）
    if ctx, ok := in["code_context"].(map[string]any); ok {
        sb.WriteString("## Code Context\n")
        
        // 目录树（截断到 1KB）
        if tree, ok := ctx["tree"].(string); ok {
            sb.WriteString("### Directory Structure\n```\n")
            sb.WriteString(truncate(tree, 1024))
            sb.WriteString("```\n\n")
        }
        
        // 相关文件内容
        if files, ok := ctx["files"].(map[string]string); ok {
            sb.WriteString("### Relevant Files\n")
            for path, content := range files {
                sb.WriteString(fmt.Sprintf("#### %s\n```%s\n%s\n```\n\n", 
                    path, detectLanguage(path), content))
            }
        }
    }
    
    return sb.String()
}
```

**步骤 3.3**: 添加辅助函数
```go
func truncate(s string, maxLen int) string {
    if len(s) <= maxLen {
        return s
    }
    return s[:maxLen] + "\n... (truncated)"
}

func detectLanguage(path string) string {
    ext := filepath.Ext(path)
    langMap := map[string]string{
        ".go": "go", ".py": "python", ".js": "javascript",
        ".ts": "typescript", ".java": "java",
    }
    return langMap[ext]
}
```

---

### 阶段 4: 更新工作流定义（15 分钟）

**文件**: `inferflow/etc/config/flows/bug_fix_workflow.yaml`

**步骤 4.1**: 在 clone_repo 和 coder 之间插入 analyze 阶段
```yaml
steps:
  - name: read_issue
    operator: stage
    stage: gitlab_read_issue
  - name: triage
    operator: stage
    stage: triage
    depends_on: [read_issue]
  - name: create_branch
    operator: stage
    stage: gitlab_create_branch
    depends_on: [triage]
  - name: clone_repo
    operator: stage
    stage: git_clone
    depends_on: [create_branch]
  - name: analyze  # 新增
    operator: stage
    stage: analyze
    depends_on: [clone_repo]
  - name: locate
    operator: stage
    stage: coder
    depends_on: [analyze]  # 修改：从 clone_repo 改为 analyze
  - name: review
    operator: stage
    stage: reviewer
    depends_on: [locate]
  - name: commit_push
    operator: stage
    stage: git_commit_push
    depends_on: [review]
```

---

### 阶段 5: 测试与验证（1 小时）

**步骤 5.1**: 单元测试 analyze 阶段
- Mock flowContext，验证文件选择逻辑
- 测试预算控制（超出 50KB 时截断）
- 测试 AllowedDirs 安全限制

**步骤 5.2**: 集成测试
- 创建测试 issue，验证完整流程
- 检查 coder 输出是否引用了上下文中的代码
- 验证生成的文件路径存在于仓库中

**步骤 5.3**: 性能测试
- 大仓库（1000+ 文件）的上下文构建时间
- Token 使用量统计
- 内存占用监控

---

## 依赖关系

### 内部依赖
1. `actions.FileReadAction` 和 `actions.FileWriteAction` 已实现（inferglow 仓库）
2. `execBash` helper 已存在（helpers.go）
3. `stage.Registry` 接口已定义

### 外部依赖
- 无新增外部依赖

### 模块依赖
```
engine.go
  ↓ 注册
file_read/file_write actions (from inferglow/builtins/actions)
  ↓ 使用
analyze.go (新文件)
  ↓ 输出
coder.go (修改)
  ↓ 工作流
bug_fix_workflow.yaml (修改)
```

---

## 风险评估

### 高风险
1. **上下文溢出**: 大仓库可能超出 token 限制
   - **缓解**: 严格 50KB 预算 + 渐进式收集 + 截断机制
   
2. **文件选择偏差**: 关键词提取可能遗漏关键文件
   - **缓解**: 多层策略（目录树 + grep + issue 提及）+ 允许 coder 请求额外文件

3. **安全风险**: 路径遍历攻击
   - **缓解**: file_read 已内置 `isPathAllowed` 检查 + symlink 解析

### 中风险
4. **性能瓶颈**: 大量文件读取增加延迟
   - **缓解**: 限制读取文件数量（最多 20 个）+ 并行读取（future）

5. **上下文相关性**: 读取的文件可能与 bug 无关
   - **缓解**: 优先读取 issue 提及的文件 + 错误栈文件

### 低风险
6. **向后兼容**: 旧工作流可能缺少 analyze 阶段
   - **缓解**: analyze 阶段可选，coder 降级到盲目生成

---

## 关键文件清单

### 需修改的文件
1. `/home/joshua/Downloads/inferglow-workdir/inferflow/runtime/engine/engine.go`
   - 第 558-561 行：runFlow() 中注册 file_read/file_write
   - 第 725-728 行：Resume() 中注册 file_read/file_write

2. `/home/joshua/Downloads/inferglow-workdir/inferflow/runtime/stage/builtin/coder.go`
   - 第 13-21 行：修改 coderSystemPrompt
   - 第 61-74 行：修改 buildCoderTaskDescription 注入上下文

3. `/home/joshua/Downloads/inferglow-workdir/inferflow/runtime/stage/builtin/register.go`
   - 第 8-19 行：添加 `r.Register("analyze", Analyze)`

4. `/home/joshua/Downloads/inferglow-workdir/inferflow/etc/config/flows/bug_fix_workflow.yaml`
   - 第 34-37 行：插入 analyze 步骤，修改 depends_on

### 需新建的文件
5. `/home/joshua/Downloads/inferglow-workdir/inferflow/runtime/stage/builtin/analyze.go`
   - Analyze 阶段实现（~200 行）
   - 包含：目录树生成、关键词提取、文件选择、预算控制

### 参考文件（只读）
6. `/home/joshua/Downloads/inferglow-workdir/inferglow/builtins/actions/file_read.go`
   - FileReadConfig 结构定义
   - isPathAllowed 安全机制

7. `/home/joshua/Downloads/inferglow-workdir/inferglow/builtins/actions/file_write.go`
   - FileWriteConfig 结构定义

8. `/home/joshua/Downloads/inferglow-workdir/inferflow/runtime/stage/builtin/helpers.go`
   - execBash 函数签名和用法

---

## 性能优化要点

### Token 预算分配（总计 50KB ≈ 12K tokens）
- 目录树概览: 2KB (5%)
- Issue 描述: 3KB (6%)
- 相关文件内容: 40KB (80%)
- 系统提示词: 5KB (9%)

### 文件选择优先级
1. **P0**: Issue 中明确提及的文件路径（正则提取）
2. **P1**: 错误栈中的文件名（如有）
3. **P2**: 与 bug 关键词匹配的文件（grep）
4. **P3**: 最近修改的文件（git log，如有）
5. **P4**: 目录树中的核心文件（如 main.go, index.js）

### 上下文构建策略
```
1. 目录树（find | head -100）→ 了解项目结构
2. 关键词提取（从 issue 描述）→ 确定搜索方向
3. grep 搜索（关键词 → 文件列表）→ 定位相关文件
4. 文件读取（按优先级 + 预算控制）→ 构建代码上下文
5. 截断与压缩（超长文件 → 关键片段）→ 控制 token 使用
```

### 大仓库处理
- **深度限制**: find 限制 3 层目录
- **数量限制**: 最多返回 100 个文件路径
- **大小限制**: 单文件最大 10KB，总计最大 40KB
- **语言过滤**: 优先读取代码文件（.go/.py/.js），忽略文档/配置

---

## 验收标准

1. ✅ file_read/file_write 在 engine 中注册，AllowedDirs 限制为 workspace
2. ✅ analyze 阶段能从仓库中提取相关代码上下文
3. ✅ 上下文总大小控制在 50KB 以内
4. ✅ coder 接收结构化上下文，生成针对性修改
5. ✅ 工作流 YAML 更新，analyze 阶段正确插入
6. ✅ 安全测试：路径遍历攻击被阻止
7. ✅ 性能测试：1000 文件仓库上下文构建 < 10 秒
# 基础设施升级：多 Provider + GitLab 预设变量

## 设计概要

两个核心变更：
1. **多 LLM Provider** — YAML 配置多个 provider，支持 default + fallback 链 + 按 stage 指定
2. **GitLab 预设变量** — 定义在 config 中作为兜底值，POST /v1/runs 可传入覆盖

---

## Task 1: 扩展 Config 结构

### 1.1 修改 `runtime/config/config.go`

将 `LLMConfig` 从单 provider 改为多 provider 结构：

```go
// Config 新增 GitLab 字段
type Config struct {
    LLM         MultiLLMConfig    `yaml:"llm"`
    GitLab      GitLabConfig      `yaml:"gitlab"`
    // ... 其余不变
}

// MultiLLMConfig 支持多 provider 并行定义
type MultiLLMConfig struct {
    Default       string               `yaml:"default"`        // 默认 provider 名称
    Providers     map[string]LLMConfig `yaml:"providers"`      // 命名 provider 列表
    FallbackChain []string             `yaml:"fallback_chain"` // fallback 顺序
}

// LLMConfig 保持原样（单个 provider 配置）
type LLMConfig struct {
    Provider  string `yaml:"provider"`
    BaseURL   string `yaml:"base_url"`
    Model     string `yaml:"model"`
    APIKey    string `yaml:"api_key"`     // 新增：直接配置（不用 env var）
    APIKeyEnv string `yaml:"api_key_env"` // 保留：从环境变量读取
    Timeout   string `yaml:"timeout"`
    ForceJSON bool   `yaml:"force_json"`
}

// GitLabConfig 定义 GitLab 预设变量（兜底值）
type GitLabConfig struct {
    HTTPURL      string `yaml:"http_url"`       // https://gitlab.example.com
    ProjectPath  string `yaml:"project_path"`   // group/project
    SSHURL       string `yaml:"ssh_url"`        // git@gitlab.example.com:group/project.git
    BranchPrefix string `yaml:"branch_prefix"`  // feature/
    DefaultBranch string `yaml:"default_branch"` // main
}
```

### 1.2 向后兼容

保留旧 `llm:` 单 provider 格式的自动迁移：如果 YAML 中有 `llm.provider` 但没有 `llm.providers`，自动将其包装为 `Providers["default"] = oldConfig`。

---

## Task 2: ProviderRegistry — 多 Provider 管理

### 2.1 新建 `runtime/integration/provider_registry.go`

```go
// ProviderRegistry 管理多个 LLM provider，支持按名称查找和 fallback。
type ProviderRegistry struct {
    providers map[string]model.ModelRequester
    defaultName string
    fallbackChain []string
}

func NewProviderRegistry(providers map[string]model.ModelRequester, defaultName string, fallback []string) *ProviderRegistry

// Resolve 按名称查找 provider。name 为空时返回 default。
func (r *ProviderRegistry) Resolve(name string) (model.ModelRequester, error)

// Default 返回默认 provider。
func (r *ProviderRegistry) Default() model.ModelRequester

// Names 返回所有已注册 provider 名称。
func (r *ProviderRegistry) Names() []string
```

### 2.2 修改 `runtime/integration/model_provider.go`

新增 `NewProviderRegistryFromConfig(cfg MultiLLMConfig) (*ProviderRegistry, error)`:
- 遍历 `cfg.Providers`，对每个调用已有的 `NewModelProvider`
- API key 优先用 `cfg.APIKey`（直接值），其次 `cfg.APIKeyEnv`（环境变量）
- 构建 `ProviderRegistry`

---

## Task 3: Engine 接入 ProviderRegistry

### 3.1 修改 `runtime/engine/engine.go`

- `Engine.modelReq` 改为 `Engine.providers *integration.ProviderRegistry`
- `NewEngine` 签名改为接受 `*integration.ProviderRegistry`（或保持 `model.ModelRequester` 做向后兼容，内部包装为单 provider registry）
- `runFlow` 中：检查 `StepDef.Inputs["_provider"]`，若指定则用该 provider 构建 flowContext
- `flowContext` 的 `modelReq` 可在每个 step 执行时动态切换（通过 context value 传递）

### 3.2 Per-stage provider override

工作流 YAML 中可指定：
```yaml
steps:
  - name: analyze
    operator: stage
    stage: triage
    inputs:
      _provider: "local-qwen"   # 使用指定 provider
  - name: review
    operator: stage
    stage: reviewer
    # 不指定 _provider → 使用 default
```

---

## Task 4: GitLab 预设变量集成

### 4.1 修改 `runtime/api/handler_runs.go`

`POST /v1/runs` 接收 inputs 时，自动注入 GitLab 预设变量（如果 inputs 中未提供）：
```go
// 在 handleCreateRun 中
if d.cfg != nil {
    if inputs["gitlab_http_url"] == "" { inputs["gitlab_http_url"] = d.cfg.GitLab.HTTPURL }
    if inputs["gitlab_project_path"] == "" { inputs["gitlab_project_path"] = d.cfg.GitLab.ProjectPath }
    if inputs["gitlab_ssh_url"] == "" { inputs["gitlab_ssh_url"] = d.cfg.GitLab.SSHURL }
    if inputs["branch_prefix"] == "" { inputs["branch_prefix"] = d.cfg.GitLab.BranchPrefix }
}
```

### 4.2 新增 `GET /v1/config/gitlab` 端点

在 `handler_flows.go` 或新文件中，暴露当前 GitLab 配置（脱敏）：
```go
// GET /v1/config/gitlab → 返回当前 GitLab 预设变量
```

---

## Task 5: 更新 config.yaml

```yaml
llm:
  default: local-qwen
  providers:
    local-qwen:
      provider: openai
      base_url: http://192.168.100.242:8200/v1
      model: Qwen3.6-35B-A3B
      api_key: dummy
    backup-ollama:
      provider: ollama
      base_url: http://localhost:11434
      model: qwen2.5:14b
  fallback_chain:
    - local-qwen
    - backup-ollama

gitlab:
  http_url: "https://gitlab.example.com"
  project_path: "group/project"
  ssh_url: "git@gitlab.example.com:group/project.git"
  branch_prefix: "feature/"
  default_branch: "main"
```

---

## Task 6: 测试 + 实测

- `config_test.go` — 多 provider 解析 + 旧格式兼容
- `provider_registry_test.go` — Resolve/fallback 逻辑
- `engine_test.go` — per-stage provider override
- 实测：启动 daemon → curl POST /v1/runs（传入 GitLab 变量）→ 验证 provider 选择

---

## 文件变更总览

| 文件 | 操作 | 说明 |
|---|---|---|
| `runtime/config/config.go` | 修改 | MultiLLMConfig + GitLabConfig |
| `runtime/integration/provider_registry.go` | 新建 | 多 provider 管理 + fallback |
| `runtime/integration/model_provider.go` | 修改 | NewProviderRegistryFromConfig |
| `runtime/engine/engine.go` | 修改 | ProviderRegistry + per-stage override |
| `runtime/api/handler_runs.go` | 修改 | GitLab 变量注入 |
| `runtime/api/server.go` | 修改 | 新增 /v1/config/gitlab 路由 |
| `cmd/inferflowd/wire.go` | 修改 | 构建 ProviderRegistry |
| `etc/config.yaml` | 修改 | 多 provider + gitlab 配置 |
| `runtime/integration/provider_registry_test.go` | 新建 | Registry 测试 |
| `runtime/config/config_test.go` | 修改 | 多 provider 解析测试 |

---

## 依赖关系

```
Task 1 (config 扩展) → Task 2 (ProviderRegistry) → Task 3 (Engine 接入)
                                              → Task 4 (GitLab 集成)
Task 5 (config.yaml 更新) — 可与 Task 1-4 并行
Task 6 (测试 + 实测) — 依赖 Task 1-5
```
