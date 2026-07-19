
# Inferflow Agentic 架构设计方案

## 一、从 pi 提炼的核心哲学

### 1.1 提示词即编排层

pi **不做硬编码的阶段划分**。它的"多步工作流"完全通过提示词模板中的步骤指令驱动 LLM 自主执行：

```
# pi 的方式（提示词驱动）
1. Add or update the relevant package changelog entry
2. Draft a final comment in my tone, preview it, and post
3. Commit only files you changed in this session
4. Include `closes #<issue>` in the commit message
5. Check the current git branch
6. Push the current branch
```

LLM 读取指令后，每步通过 tool calls（bash, read, edit, write）自主完成。

**Inferflow 当前方式（代码编排）**：每个 stage 是一个 Go struct，硬编码 system prompt + builder 函数，YAML 定义步骤顺序。

**设计建议**：保留代码编排的确定性优势，但引入 pi 的**动态提示词组装**——system prompt 不再是一个 const 字符串，而是根据当前 stage 的可用上下文（inputs 中有哪些字段）动态拼装。

### 1.2 按需注入 > 全量灌入（Token 节省核心）

pi 的三层 token 节省策略：

| 层级 | pi 做法 | Inferflow 现状 | 设计目标 |
|------|---------|---------------|---------|
| **工具可见性** | 工具只有 `promptSnippet` 存在时才出现在提示词 | 10 个 action 全部注册，提示词中不体现 | 每个 stage 只"看到"自己需要的工具描述 |
| **指南条件化** | 指南按工具可用性动态生成（如只有 bash 可用时才注入"用 bash 做文件操作"） | coder/reviewer 提示词固定不变 | 按 stage 类型 + 语言检测结果条件注入编码规范 |
| **Skill 索引化** | 系统提示词只含 name+description（~3行/skill），完整内容通过 read 工具按需加载 | 无此概念 | 语言规范/框架约定作为"Skill"，只放索引 |

### 1.3 上下文流动与刷新

pi 的 `prepareNextTurnWithContext` 钩子在**每个 agent turn 之前**重新注入系统提示词。工具集变化时自动重建。

**Inferflow 现状**：每次 LLM 调用独立（system + 1 条 user message），无多轮对话，无 turn 间刷新。

**设计目标**：在 stage 之间引入 `PromptContext` 结构体，携带累积上下文，每个 stage 可以从中提取/追加信息。

---

## 二、提示词动态组装框架设计

### 2.1 架构概览

```
PromptBuilder（新增）
├── PromptBlock[]          ← 可组合的提示词块
│   ├── RoleBlock          ← "You are an expert coding assistant..."
│   ├── OutputFormatBlock  ← JSON schema 定义
│   ├── GuidelinesBlock    ← 条件注入的编码规范
│   ├── ContextBlock       ← 代码上下文、文件列表等
│   └── TaskBlock          ← 具体任务描述
├── LanguageDetector       ← 检测仓库语言，决定注入哪些 Guidelines
├── TokenBudget            ← 控制总 token 不超限
└── Render() → string      ← 最终组装
```

### 2.2 PromptBlock 接口设计

```go
// runtime/prompt/block.go

type PromptBlock interface {
    // Name returns the block identifier for dedup and debugging.
    Name() string
    // EstimateTokens returns approximate token count for budget planning.
    EstimateTokens() int
    // Render produces the markdown text for this block.
    // ctx carries accumulated data from previous stages.
    Render(ctx *PromptContext) string
    // IsActive returns whether this block should be included.
    // Blocks can conditionally activate based on context.
    IsActive(ctx *PromptContext) bool
}
```

### 2.3 条件注入的语言规范系统

**核心思路**：不在 triage 阶段注入编码规范，而是在 coder stage 根据 analyze 检测到的语言动态注入。

```go
// runtime/prompt/language_guidelines.go

type LanguageGuidelines struct {
    Language    string   // "go", "python", "typescript", etc.
    Frameworks  []string // detected frameworks
    Blocks      []PromptBlock
}

// Go 编码规范块
type GoGuidelinesBlock struct{}

func (b GoGuidelinesBlock) IsActive(ctx *PromptContext) bool {
    return ctx.DetectedLanguage == "go"
}

func (b GoGuidelinesBlock) Render(ctx *PromptContext) string {
    return `## Go Coding Guidelines
- Follow standard Go formatting (gofmt)
- Exported names must have doc comments
- Use errors.Is/errors.As for error comparison
- Prefer table-driven tests
- ...`
}
```

**Token 节省效果估算**：
- 当前：coder system prompt ~56 行（固定），不管什么语言都包含所有规范
- 设计后：基础 prompt ~20 行 + 语言规范 ~15 行（仅匹配语言）+ 上下文（动态）
- 对于简单 bug fix，可节省 ~40% token

### 2.4 语言检测策略

```go
// runtime/prompt/language_detector.go

func DetectLanguage(files map[string]string) LanguageGuidelines {
    // 策略 1: 根据文件扩展名统计
    // .go → Go, .py → Python, .ts/.tsx → TypeScript
    // 策略 2: 根据特征文件
    // go.mod → Go, package.json → JS/TS, pyproject.toml → Python
    // 策略 3: 使用 analyze stage 已有的搜索结果
    // analyze 的 file_glob + grep_search 结果可作为输入
}
```

**关键**：语言检测在 analyze stage 完成，结果存入 accumulated data map 的 `detected_language` 字段。coder stage 的 PromptBuilder 读取此字段决定注入哪些 Guidelines。

### 2.5 Token 预算管理

```go
// runtime/prompt/budget.go

type TokenBudget struct {
    MaxTokens     int            // 模型上下文窗口
    ReservedTokens int           // 预留给回复的 token
    Blocks        []PriorityBlock // 按优先级排序的块
}

type PriorityBlock struct {
    Block    PromptBlock
    Priority int    // 1=必须, 2=重要, 3=可选
    MaxTokens int   // 该块最大 token 数
}

func (b *TokenBudget) Render(ctx *PromptContext) string {
    // 1. 先渲染所有 Priority=1 的块
    // 2. 按优先级依次渲染 Priority=2 的块，直到预算用完
    // 3. Priority=3 的块填充剩余空间
    // 4. 如果单个块超限，截断其内容（如 code_context 截断到 N 个文件）
}
```

**优先级分配**：
- P1（必须）：RoleBlock, OutputFormatBlock, TaskBlock
- P2（重要）：LanguageGuidelines（匹配的语言）, ReviewFeedback（retry 时）
- P3（可选）：CodeContext（analyze 输出，可截断）, Subtasks（可精简）

---

## 三、Git 规范化提示词设计

### 3.1 设计原则

pi 的 Git 集成是**约定驱动**而非代码驱动——没有专门的 git tool，所有操作通过 bash + AGENTS.md 约定。Inferflow 应借鉴此模式：

1. **保留 git_clone / git_commit_push 等 stage 作为基础操作**（确定性高）
2. **新增 git_executor stage**：处理分支切换、冲突解决等复杂场景
3. **通过 GitGuidelines 提示词块约束 git 行为**

### 3.2 GitGuidelines PromptBlock

```go
// runtime/prompt/git_guidelines.go

type GitGuidelinesBlock struct{}

func (b GitGuidelinesBlock) Render(ctx *PromptContext) string {
    return `## Git Guidelines

### Branch Management
- Always create a new branch for changes: git checkout -b fix/<issue-id>-<short-desc>
- Never commit directly to main/master
- Branch naming: feat/<id>-desc, fix/<id>-desc, docs/<id>-desc

### Commit Conventions
- Commit message format: {type}[(scope)]: {description}
- Types: feat, fix, docs, refactor, test, chore
- Only commit files modified in this session
- Use explicit git add <file1> <file2>, NEVER git add -A or git add .
- NEVER use: git reset --hard, git stash, git commit --no-verify

### Before Pushing
- Run go build ./... (or language equivalent) to verify compilation
- Run go test ./... to verify tests pass
- Check git status for unintended changes
- Verify diff is minimal and relevant`
}
```

### 3.3 Git Executor Stage 设计

```go
// runtime/stage/builtin/git_executor.go

// git_executor 是一个智能 git 操作 stage
// 它接收 git_operation 参数，根据操作类型注入不同的提示词
type GitExecutor struct{}

func (g *GitExecutor) Execute(ctx context.Context, fctx flow.FlowContext, in stage.Inputs) (stage.Outputs, error) {
    operation := in.String("git_operation") // "switch_branch", "create_branch", "commit", "push", "resolve_conflict"
    
    // 根据操作类型构建不同的提示词
    systemPrompt := buildGitSystemPrompt(operation)
    userMessage := buildGitTaskMessage(in)
    
    // 调用 LLM 生成 git 命令
    // 通过 bash_executor action 执行
    // 返回执行结果
}
```

### 3.4 Git 操作的提示词模板

```
# switch_branch
You are a git operations assistant. Switch to branch "{branch_name}" in repository {repo_path}.
If the branch doesn't exist locally, try: git checkout -b {branch_name} origin/{branch_name}
If that fails, create it: git checkout -b {branch_name}
Always verify with: git branch --show-current

# create_branch  
Create branch {branch_name} from {base_branch} in {repo_path}.
Command sequence:
1. git fetch origin
2. git checkout {base_branch}
3. git pull origin {base_branch}
4. git checkout -b {branch_name}
Verify: git branch --show-current

# commit
Commit changes in {repo_path}:
1. git status (review changes)
2. git diff (verify diff is minimal)
3. git add <specific files only>
4. git commit -m "{commit_message}"
NEVER use git add -A. Only add files you intentionally modified.
```

---

## 四、可操作的 Go 工作流步骤设计

### 4.1 完整 Pipeline 步骤

```
Step 1: triage        → 分类 + 优先级 + 子任务分解 + 搜索提示
Step 2: analyze       → 文件扫描 + 语言检测 + 代码上下文收集
Step 3: coder         → 代码生成（带语言规范按需注入）
Step 4: write_files   → 文件写入磁盘
Step 5: reviewer      → 代码审查（带语言规范 + 项目约定）
Step 6: tester        → 运行测试
Step 7: git_executor  → 分支管理 + 提交 + 推送（带 Git 规范）
```

### 4.2 每步的提示词组装策略

| Step | System Prompt 组成 | Token 预算分配 |
|------|-------------------|---------------|
| triage | Role + OutputFormat + Task | ~800 tokens（轻量） |
| analyze | 不调 LLM（纯 action） | 0 |
| coder | Role + OutputFormat + **LanguageGuidelines(动态)** + Task + CodeContext | ~2000-4000 tokens |
| write_files | 不调 LLM（纯 action） | 0 |
| reviewer | Role + OutputFormat + **LanguageGuidelines(动态)** + ReviewCriteria + CodeToReview | ~2000-3000 tokens |
| tester | 不调 LLM（纯 action） | 0 |
| git_executor | Role + **GitGuidelines** + Task | ~500-800 tokens |

### 4.3 上下文流动图

```
triage ──→ {category, priority, subtasks, spec, acceptance_criteria, search_hints}
              │
analyze ──→ {detected_language, code_context, relevant_files}
              │
coder ──→ {files (JSON), summary, reasoning}
    │         ↑ 读取 detected_language → 注入 LanguageGuidelines
    │         ↑ 读取 code_context → 作为参考
    │
write_files ──→ {files_written, write_ok}
    │
reviewer ──→ {approved, comments, passed}
    │         ↑ 读取 detected_language → 注入 LanguageGuidelines
    │         ↑ 读取 files → 审查生成的代码
    │
tester ──→ {test_output, tests_passed, test_count, failures}
    │
git_executor ──→ {branch, commit_sha, push_ok}
                  ↑ 读取 files_written → 只 add 这些文件
                  ↑ 注入 GitGuidelines → 规范 commit message
```

---

## 五、关键文件清单

### 新增文件

| 文件路径 | 职责 |
|---------|------|
| `runtime/prompt/block.go` | PromptBlock 接口 + PromptContext 定义 |
| `runtime/prompt/builder.go` | PromptBuilder：组装 blocks → 最终字符串 |
| `runtime/prompt/budget.go` | TokenBudget：token 计数 + 优先级裁剪 |
| `runtime/prompt/language_detector.go` | 语言检测逻辑 |
| `runtime/prompt/language_guidelines.go` | Go/Python/TS 等语言的 GuidelinesBlock |
| `runtime/prompt/git_guidelines.go` | Git 规范 GuidelinesBlock |
| `runtime/prompt/role_blocks.go` | 各 stage 的 RoleBlock 定义 |
| `runtime/stage/builtin/git_executor.go` | Git 智能操作 stage |

### 修改文件

| 文件路径 | 修改内容 |
|---------|---------|
| `runtime/stage/builtin/coder.go` | 用 PromptBuilder 替换硬编码 coderSystemPrompt |
| `runtime/stage/builtin/reviewer.go` | 用 PromptBuilder 替换硬编码 reviewerSystemPrompt |
| `runtime/stage/builtin/triage.go` | 用 PromptBuilder 替换硬编码 triageSystemPrompt |
| `runtime/stage/builtin/analyzer.go` | 增加语言检测，输出 detected_language |
| `runtime/stage/builtin/register.go` | 注册 git_executor stage |
| `etc/config/flows/bug_fix_workflow.yaml` | 增加 git_executor step |

### 已有但未使用的设施

| 文件路径 | 说明 |
|---------|------|
| `runtime/prompt/prompt.go` | prompt.Store 已实现 .tmpl 文件加载和热加载，可直接用于外部化 prompt 模板 |

---

## 六、Token 节省效果预估

### 当前 vs 设计后对比

| 场景 | 当前 token 消耗 | 设计后 token 消耗 | 节省 |
|------|-----------------|------------------|------|
| Go 项目 coder | ~3500（固定 prompt + 全量 context） | ~2200（基础 + Go 规范 + 裁剪 context） | ~37% |
| Python 项目 coder | ~3500（同上，含无关 Go 规范） | ~2000（基础 + Python 规范） | ~43% |
| 简单 bugfix（无 retry） | ~7000（triage+coder+reviewer） | ~4500（精简 prompt + 按需注入） | ~36% |
| 带 retry 的完整流程 | ~14000+ | ~8000（retry 时只增量注入 review feedback） | ~43% |

### 关键节省来源

1. **语言规范按需注入**：只注入匹配语言的规范（~500 tokens），而非所有语言的通用规范
2. **Code context 裁剪**：TokenBudget 按优先级裁剪，简单任务只保留最相关文件
3. **Output format 精简**：根据 stage 类型精简 JSON schema 描述
4. **Retry 增量注入**：retry 时不重发完整 context，只追加 review feedback

---

## 七、风险与缓解

| 风险 | 影响 | 缓解措施 |
|------|------|---------|
| 语言检测不准确 | 注入错误语言的规范 | 多信号交叉验证（扩展名+特征文件+analyze 结果），不确定时不注入 |
| Token 估算不精确 | 实际超限导致截断 | 使用 tiktoken-go 库精确计数，预留 10% buffer |
| PromptBuilder 复杂度增加 | 维护成本上升 | 保持 PromptBlock 接口简洁，每个 block 独立可测试 |
| Git executor 的 LLM 调用可能生成错误命令 | git 操作失败 | 增加命令白名单验证，危险命令（reset --hard 等）硬拒绝 |
| 外部化 prompt 模板后版本管理 | 模板与代码版本不一致 | 模板文件随代码一起版本控制，CI 检查模板语法 |

---

## 八、Rejected Alternatives

1. **完全 pi-style（无代码编排，纯 LLM 自主）**：
   - 拒绝原因：Inferflow 是确定性 pipeline，需要可审计的 step log 和 checkpoint。纯 LLM 自主不可预测、不可复现。
   
2. **引入 tiktoken-go 做精确 token 计数**：
   - 暂不采用：增加 CGO 依赖。先用简单的 `len(text)/4` 估算，后续需要时再引入。

3. **多轮对话（累积 chat history）**：
   - 暂不采用：当前每个 stage 独立调用 LLM 已足够。多轮对话增加 session 管理复杂度，且与 accumulated data map 模式冗余。

4. **Skill 索引+按需加载（pi 的完整 skill 系统）**：
   - 暂不采用：Inferflow 的 stage 数量有限，不需要动态 skill 发现。语言规范直接作为 PromptBlock 条件注入即可。
