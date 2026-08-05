# 内置工具目录与 Schema 说明（Tool Catalog）

> 面向**用户/扩展者**的内置工具权威清单：注册名、参数 Schema、安全规格、返回形状，以及与 Claude.ai 可对照能力的映射。
> 与 [tool-organization.md](./tool-organization.md)（"如何注册/分组/过滤/调度工具"）互补：本目录回答"有哪些内置工具、每个工具长什么样、怎么用、怎么改进"。

---

## 0. 最新事实校准（重要）

> ⚠️ 过去的分析文档（如 `docs/archive/maturity-v4-analysis.md`）写"内置工具 **8 个**"，**已严重过时**，请以本目录为准。

**截至当前代码库，`builtins/actions/` 实际包含：**

| 维度 | 数量 |
|------|------|
| 源文件 | **19** 个 |
| 注册工具名 | **22** 个（`task_tracker.go` 产出 4 个、`memory_*` 产出 3 个） |

数据来源：`builtins/actions/*.go` 中的 `ActionSpec` 与 `New*Action()` 构造器。工具按能力分为 **10 大类**。

---

## 1. 工具总览（按可对照能力分类）

| 能力类别 | 工具名 | 副作用级别 | 需审批 | 需沙箱 | Claude.ai 可对照 |
|---------|--------|:---------:|:------:|:------:|-----------------|
| **执行 / Shell** | `bash_executor` | exec | ✅ | ✅ | Computer Use / Bash（子集） |
| **代码执行** | `code_executor` | exec | ✅ | ✅ | Code / Assistant（代码执行） |
| **文件系统·读** | `file_read` | read | — | — | Text Editor（读） |
| | `list_dir` | read | — | — | Text Editor（浏览） |
| | `grep` | read | — | — | Text Editor / 代码检索 |
| **文件系统·写** | `file_write` | write | ✅ | — | Text Editor（写） |
| **数据处理** | `calculator` | none | — | — | —（数值原语） |
| | `json_processor` | none | — | — | —（数据处理原语） |
| **Web** | `web_search` | read(网络) | — | — | Web Search |
| | `url_fetch` | read(网络) | — | — | Web Fetch / Browse |
| **多媒体** | `image_generate` | write | — | — | 图像生成 |
| | `text_to_speech` | write | — | — | TTS（Claude.ai 无内建） |
| | `speech_to_text` | read | — | — | STT（Claude.ai 无内建） |
| **记忆** | `remember` | write | — | — | Memory（写入） |
| | `memory` | read | — | — | Memory（检索） |
| | `forget` | write | — | — | Memory（管理） |
| **Agent 协作** | `spawn_agent` | — | — | — | 子代理 / Multi-Agent |
| **技能** | `run_skill` | — | — | — | Skills（Claude Code） |
| **任务跟踪** | `task_add` / `task_update` / `task_list` / `task_delete` | write | — | — | —（需 MCP 接入） |

> **映射口径**：Claude.ai 是"模型托管的消费级产品工具"（黑盒、带版本号、跟随系统提示词）；InferGlow 是"框架级自托管工具原语"（开源、可审计、可替换）。因此对照只在**能力类别**层面成立，不做"工具对 JSON Schema"的逐字对照。

---

## 2. 各工具 Schema 详解

> 每个工具给出：注册名、安全规格、参数表、返回形状、使用要点、如何改进。
> 参数表中的 `required` 依据 `Schema.properties` + `required` 数组；`SideEffectLevel` 见 [action/spec.go](../../action/spec.go)。

---

### 2.1 执行类（高风险：exec + 审批 + 沙箱）

#### `bash_executor` — 在沙箱内执行 shell 命令
- **安全规格**：`SideEffectExec` · `ApprovalRequired=true` · `SandboxRequired=true` · `ReplaySafe=false`
- **执行**：委托给注入的 `BashExecutor`（典型由 `sandbox.Manager` 实现），本包不直接执行命令。

| 参数 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `command` | string | ✅ | 要执行的 shell 命令 |
| `workdir` | string | — | 工作目录 |
| `stdin` | string | — | 传给命令的标准输入 |
| `timeout` | string | — | 可选时长字符串（如 `"30s"`） |
| `env` | object | — | 环境变量覆盖（`map[string]string`） |

- **返回**：`object { exit_code int, stdout string, stderr string, duration string }`
- **改进点**：`command` 缺少 `description`；`env` 未标注 `additionalProperties` 值类型；`timeout` 未给格式示例。建议参照 `image_generate` 补齐字段级 `description`，并给 `command` 加 risk 提示。

---

#### `code_executor` — 在沙箱内执行任意源代码
- **安全规格**：`SideEffectExec` · `ApprovalRequired=true` · `SandboxRequired=true` · `ReplaySafe=false`

| 参数 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `language` | string | ✅ | 代码语言（如 python、go、js） |
| `source` | string | ✅ | 源代码 |
| `stdin` | string | — | 运行时的标准输入 |
| `timeout` | string | — | 可选时长字符串 |

- **返回**：`object { exit_code, stdout, stderr, duration }`
- **改进点**：`language` 未给可选枚举（`python/go/js/...`），模型可能传非法值；建议补 `enum`。

---

### 2.2 文件系统类

#### `file_read` — 从允许目录读取文件
- **安全规格**：`SideEffectRead` · `ReplaySafe=true`
- **路径守卫**：受 `FileReadConfig.AllowedDirs` 限制，路径穿越会被拒绝。

| 参数 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `path` | string | ✅ | 文件路径（必须在允许目录内） |
| `max_bytes` | integer | — | 读取字节上限（默认见 `DefaultFileReadLimit`） |

- **返回**：`object { path, bytes_read, content }`
- **改进点**：`path` 缺 `description`；建议说明"必须位于 AllowedDirs 内，否则返回 blocked"。

---

#### `list_dir` — 列出目录内容
- **安全规格**：`SideEffectRead` · 路径守卫同 `file_read`。

| 参数 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `path` | string | ✅ | 目录路径（必须在允许目录内） |

- **返回**：`object { path, entries[ { name, type, size } ], count }`，`type ∈ file|dir|symlink|other`
- **改进点**：`path` 缺 `description`；`type` 的枚举应写进说明。

---

#### `grep` — 在文件中搜索模式
- **安全规格**：`SideEffectRead`（搜索类别）。

| 参数 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `pattern` | string | ✅ | 正则/文本模式 |
| `path` | string | ✅ | 搜索路径 |
| `recursive` | boolean | — | 是否递归子目录 |
| `max_results` | integer | — | 最大结果数 |

- **返回**：`object { pattern, matches[ { file, line, content } ], count }`
- **改进点**：`pattern` 应注明是正则还是字面量；`max_results` 缺默认值说明。

---

#### `file_write` — 在允许目录内写入文件
- **安全规格**：`SideEffectWrite` · `ApprovalRequired=true` · `ReplaySafe=false`

| 参数 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `path` | string | ✅ | 目标文件路径 |
| `content` | string | ✅ | 写入内容 |
| `append` | boolean | — | 追加而非覆盖 |

- **返回**：`object { path, bytes_written }`
- **改进点**：`content` 缺 `description`；建议提示大文件单次写入上限。

---

### 2.3 数据处理类

#### `calculator` — 计算数学表达式
- **安全规格**：`SideEffectNone` · `ReplaySafe=true`
- **支持**：加/减/乘/除/取模/幂/括号。

| 参数 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `expression` | string | ✅ | 数学表达式 |

- **返回**：`number`
- **改进点**：`expression` 缺 `description`，应给出支持运算符与示例（如 `"1+2*3"`）。

---

#### `json_processor` — 解析 JSON 并按 JSONPath 提取
- **安全规格**：`SideEffectNone` · `ReplaySafe=true`

| 参数 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `json` | string | ✅ | JSON 文档 |
| `path` | string | — | JSONPath 表达式（`op=query` 时） |
| `op` | string(`query`\|`parse`) | — | 操作类型，默认 `query` |

- **返回**：`any`
- **改进点**：`op` 缺 `enum` 约束；`path` 缺 JSONPath 语法示例。

---

### 2.4 Web 类

#### `web_search` — 网页搜索
- **安全规格**：`SideEffectRead`（网络）· `ReplaySafe=false`

| 参数 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `query` | string | ✅ | 搜索查询 |

- **返回**：`array[ { title string, url string, snippet string } ]`
- **改进点**：`query` 缺 `description`；返回数组未标 `maxResults` 上限。

---

#### `url_fetch` — 抓取 HTTP(S) 页面
- **安全规格**：`SideEffectRead`（网络）· `ReplaySafe=false` · 内建超时与字节上限防 DoS。

| 参数 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `url` | string | ✅ | 要抓取的 URL |
| `max_bytes` | integer | — | 内容字节上限（默认 `DefaultURLFetchLimit`） |

- **返回**：`object { url, status_code, content_type, content, bytes_read }`
- **改进点**：`url` 缺 `description`；建议注明仅允许 `http/https` scheme。

---

### 2.5 多媒体类

#### `image_generate` — 文生图
- **安全规格**：`SideEffectWrite` · `ReplaySafe=true`
- **后端**：DALL-E / Stable Diffusion / 兼容 API（注入 `ImageGenerator`）。

| 参数 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `prompt` | string | ✅ | 图像的文字描述 |
| `size` | string | — | 尺寸，如 `1024x1024`、`1792x1024` |
| `model` | string | — | 模型名，如 `dall-e-3`、`stable-diffusion-xl` |

- **返回**：`object { prompt, mime_type }` + 图片 `ContentBlock`
- **改进点**：`size` 应给更完整的枚举示例；`model` 建议列出当前可用集合。**这是 schema 字段级 `description` 写法的良好范本**，其余工具可参照补齐。

---

#### `text_to_speech` — 文本转语音
- **安全规格**：`SideEffectWrite` · `ReplaySafe=true`
- **后端**：OpenAI TTS / ElevenLabs / 兼容 API。

| 参数 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `text` | string | ✅ | 要转成语音的文本 |
| `voice` | string | — | 音色名，如 `alloy`、`echo`、`shimmer` |
| `model` | string | — | 模型名，如 `tts-1`、`tts-1-hd` |

- **返回**：音频 `ContentBlock`（MP3/WAV/OGG）
- **改进点**：`voice`/`model` 建议给枚举或示例。

---

#### `speech_to_text` — 语音转文本
- **安全规格**：`SideEffectRead` · `ReplaySafe=true`
- **后端**：Whisper / Deepgram / 兼容 API。

| 参数 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `audio_data` | string | ✅ | Base64 音频数据或音频 URL |
| `language` | string | — | 语言码，如 `en`、`zh`、`ja` |
| `model` | string | — | 模型名，如 `whisper-1`、`deepgram-nova-2` |

- **返回**：`{ text, duration, words[ { text, start, end } ] }`
- **改进点**：`audio_data` 未区分"Base64 还是 URL"的类型约束；建议拆分参数或加 `format` 说明。

---

### 2.6 记忆类

#### `remember` — 保存/更新长期记忆
- **安全规格**：`SideEffectWrite` · 写入文件型记忆库。

| 参数 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `name` | string | — | 稳定的 kebab-case 记忆名；复用旧名即更新 |
| `title` | string | — | 记忆索引中的短标签 |
| `description` | string | ✅ | 索引中的一行钩子（未来会话据此决定是否打开） |
| `type` | string | — | `user\|feedback\|project\|reference` |
| `scope` | string | — | `project\|global`，默认 `project` |
| `body` | string | ✅ | 事实本身（Markdown）；feedback/project 需含 `**Why:**` 与 `**How to apply:**` |

- **改进点**：`type`/`scope` 的 `enum` 已给出，但 `name` 可选会让模型难以决定是否传；建议在 `description` 中更强调"查重后复用旧名"。

---

#### `memory` — 搜索/读取/列出记忆
- **安全规格**：`SideEffectRead` · 只读。

| 参数 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `operation` | string | ✅ | `search\|read\|list` |
| `query` | string | — | `operation=search` 时的查询词 |
| `name` | string | — | `operation=read` 时的记忆名 |
| `type` | string | — | 记忆类型过滤（见上） |
| `scope` | string | — | 作用域过滤 |
| `limit` | integer | — | 结果上限，默认 8，最大 20 |

- **改进点**：`operation` 的 `enum` 已给出；建议说明 `search` 采用 BM25 排序。

---

#### `forget` — 归档记忆
- **安全规格**：`SideEffectWrite` · 软删除（移入 `.archive/` 保留追溯）。

| 参数 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `name` | string | ✅ | 要归档的记忆名（kebab-case） |

- **返回**：归档确认（含 `archived` 状态）
- **改进点**：schema 已较完整；建议提示"先用 `memory(operation=list)` 确认确切名字"。

---

### 2.7 Agent 协作类

#### `spawn_agent` — 派生子代理执行委托任务
- **安全规格**：通过 `Context.RunAgent` 运行，带深度/轮次控制（默认 `MaxDepth=3`、`MaxRounds=15`）。

| 参数 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `task` | string | ✅ | 子代理要完成的任务描述 |
| `system_prompt` | string | — | 引导子代理行为的可选系统提示词 |
| `max_rounds` | number | — | 子代理最大迭代轮次（默认 15） |

- **改进点**：`max_rounds` 用 `number`（float），建议用 `integer`；`task` 缺 `description`。

---

### 2.8 技能类

#### `run_skill` — 调用技能剧本
- **两种模式**：`inline`（把技能内容注入当前轮次）或 `subagent`（隔离子代理中运行复杂剧本，默认 15 轮）。

| 参数 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| `name` | string | ✅ | 技能名（如 `go-test-fix`、`code-review`） |
| `arguments` | string | — | 任务相关参数/上下文 |

- **改进点**：`arguments` 用 `string` 而非结构化对象，限制模型传参；建议改为 `object` 或给出 JSON 字符串示例。

---

### 2.9 任务跟踪类（`task_tracker.go` 产出 4 个工具）

| 工具 | 必填参数 | 其它参数 | 返回要点 |
|------|---------|---------|---------|
| `task_add` | `title` | `description`、`priority`(0正常/1高) | 新任务 `id` |
| `task_update` | `task_id` | `status`(`pending\|in_progress\|done\|cancelled`)、`title`、`description` | 更新后任务 |
| `task_list` | — | `status_filter`(同 status 枚举) | `{count, tasks[]}` |
| `task_delete` | `task_id` | — | 删除确认 |

- **安全规格**：`task_add/update/delete` 为 `SideEffectWrite`；`task_list` 只读。
- **改进点**：整体 schema 已带 `description`，是较好的范本；`priority` 建议加 `enum`。

---

## 3. 与 Claude.ai 可对照能力映射表

> 详见本次系列文档的 `fable-research/` 分析。这里给出 InferGlow 内置工具与 Claude.ai 消费级工具的能力级对照。

| 能力类别 | InferGlow 工具 | Claude.ai（Fable 5） | 成熟度差异 |
|---------|---------------|---------------------|-----------|
| Web 搜索 | `web_search` | Web Search | InferGlow 缺结果去重/地域/时间过滤；Claude.ai 产品化更完善 |
| 网页抓取 | `url_fetch` | Web Fetch / Browse | 同级；InferGlow 有字节上限防 DoS |
| 代码执行 | `code_executor` | Code / Assistant | InferGlow 强制沙箱+审批；Claude.ai 模型托管 |
| Shell 执行 | `bash_executor` | Computer Use / Bash | InferGlow 沙箱隔离更强；Claude.ai 无人值守自动执行 |
| 文件读写 | `file_read`/`file_write`/`list_dir`/`grep` | Text Editor | InferGlow 有 AllowedDirs 路径守卫；Claude.ai 有版本回滚 |
| 图像生成 | `image_generate` | 图像生成 | 同级；InferGlow 可注入多后端 |
| 语音 | `text_to_speech`/`speech_to_text` | Claude.ai 无内建 | **InferGlow 领先**（Claude.ai 需模型/插件支持） |
| 记忆 | `remember`/`memory`/`forget` | Memory | 同级架构（type/scope 分类）；Claude.ai 产品化 UI 更强 |
| 子代理 | `spawn_agent` | Multi-Agent | 同级；均有深度/轮次护栏 |
| 技能 | `run_skill` | Skills（Claude Code） | 同级剧本机制 |
| 任务跟踪 | `task_add` 等 4 个 | 无内建（需 MCP） | **InferGlow 领先** |
| MCP 接入 | `action/mcp/` + `mcpserver/` | MCP 连接器 | 均有；InferGlow 支持 stdio/HTTP/流式 |

**结论**：能力类别层面 InferGlow 与 Claude.ai 覆盖高度重合，且在**语音、任务跟踪、沙箱隔离、开源可审计**四项上领先；Claude.ai 在**产品化打磨（版本化、UI、安全门控、搜索结果质量）**上领先。逐字 JSON Schema 对照不成立（层面不同）。

---

## 4. 如何改进/扩展内置工具（Schema 强化指南）

> 让其它用户能"用得上、改得动"，核心是**每个参数都要有清晰的 `description`**。规范如下。

### 4.1 参数 Schema 书写规范
1. **每个参数都写 `description`**：说明"是什么 + 何时用 + 格式示例"。参考 `image_generate` / `remember` / `task_*`（好范本）。
2. **有受限取值就加 `enum`**：如 `op`、`operation`、`status`、`type`、`scope`、`language`。
3. **数值参数给取值范围**：如 `limit` 标 "default 8, max 20"。
4. **必填放 `required` 数组**：不用 `"required": true` 单字段（两者语义不同，见 2.x 表格即源码实际用法）。
5. **返回形状写进 `Returns` 或说明**：让模型知道能拿到什么。

### 4.2 已知待改进清单
| 工具 | 待改进 |
|------|--------|
| `bash_executor` | `command`/`env`/`timeout` 缺 `description` 与格式示例 |
| `code_executor` | `language` 缺 `enum` |
| `file_read`/`list_dir`/`file_write`/`grep`/`url_fetch`/`calculator`/`json_processor`/`web_search` | 参数字段缺 `description` |
| `spawn_agent` | `max_rounds` 应改 `integer`；`task` 缺 `description` |
| `run_skill` | `arguments` 建议改结构化 `object` |
| `speech_to_text` | `audio_data` 需区分 Base64 vs URL |

### 4.3 新增一个工具（三步）
1. 在 `builtins/actions/` 新建 `xxx.go`，定义 `NewXxxAction()`，按上述规范写好 `Schema`。
2. 注册到 `ActionRegistry`（见 [tool-organization.md](./tool-organization.md#2-扁平注册-action--actionregistry)）。
3. 如需安全门控，设置 `SideEffectLevel` / `ApprovalRequired` / `SandboxRequired`（见 [action/spec.go](../../action/spec.go)）。

---

## 5. 关键文件索引

| 文件 | 内容 |
|------|------|
| `builtins/actions/` | 22 个内置工具定义（本目录数据源） |
| `action/spec.go` | `ActionSpec` / `SideEffectLevel` / `ActionPolicy` |
| `action/registry.go` | `ActionRegistry` 注册/查询/执行 |
| `builtins/tools/schema_from_func.go` | 从函数签名自动生成 JSON Schema |
| `docs/guides/tool-organization.md` | 工具注册/分组/过滤/调度使用指南 |