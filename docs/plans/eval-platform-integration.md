# 评估平台对接计划：Langfuse + LangSmith 兼容

> **状态（2026-07-30）**：方向确定，待实施。
> 前置依赖：eval 模块现有 `Report` / `CaseResult` 结构（已就绪）。

---

## 设计目标

1. 团队开发时有可视化平台审查评估结果（不自建 Dashboard）
2. 默认对接 **Langfuse**（自托管 Docker，无限额，数据不出内网）
3. 兼容 **LangSmith**（用户配置 API Key 即可切换，零代码改动）
4. InferGlow 只做**数据产出侧**（探针），不做展示侧

---

## 架构

```
InferGlow eval 模块
  │
  ├── FormatText()   → 终端（已有）
  ├── FormatJSON()   → CI 门禁（已有）
  ├── FormatHTML()   → 本地浏览器（待实现，~150 行）
  │
  └── Reporter interface（待实现）
        │
        ├── LangfuseReporter（默认）
        │     POST {LANGFUSE_ENDPOINT}/api/public/ingestion
        │     Auth: Basic base64(public_key:secret_key)
        │
        └── LangSmithReporter（兼容）
              POST https://api.smith.langchain.com/runs
              POST https://api.smith.langchain.com/feedback
              Auth: Bearer {LANGSMITH_API_KEY}
```

---

## Reporter 接口定义

```go
// evalbridge/reporter.go

// Reporter 是评估结果上报的抽象接口。
// 实现方负责将 RunRecord 和 FeedbackScore 上报到目标平台。
type Reporter interface {
    // ReportRun 上报一次执行记录（trace）。
    ReportRun(ctx context.Context, rec RunRecord) error
    // ReportScore 上报评估评分（feedback）。
    ReportScore(ctx context.Context, score FeedbackScore) error
    // Name 返回 Reporter 标识（用于日志）。
    Name() string
}

// RunRecord 是一次 Agent 执行的结构化记录。
type RunRecord struct {
    ID         string         // UUID
    Name       string         // Case 名称 / Suite 名称
    Input      string         // 用户消息
    Output     string         // Agent 最终回复
    ToolCalls  []string       // 工具调用序列
    Latency    time.Duration  // 执行耗时
    Tokens     int            // token 用量
    SessionID  string         // 会话 ID（多轮分组）
    Metadata   map[string]any // 扩展字段
}

// FeedbackScore 是一条评估评分。
type FeedbackScore struct {
    RunID   string  // 关联的 RunRecord.ID
    Key     string  // 评估维度（correctness / hallucination / ...）
    Value   float64 // 分数（1-5）
    Comment string  // 评估器评语
}
```

---

## 平台配置（环境变量）

```bash
# Langfuse（默认，自托管）
EVAL_REPORTER=langfuse
LANGFUSE_ENDPOINT=http://localhost:3000
LANGFUSE_PUBLIC_KEY=pk-xxx
LANGFUSE_SECRET_KEY=sk-xxx

# LangSmith（兼容，切换只需改环境变量）
EVAL_REPORTER=langsmith
LANGSMITH_API_KEY=lsv2-xxx
```

**切换逻辑**：读取 `EVAL_REPORTER` 环境变量，实例化对应 Reporter。未配置时不上报（仅本地报告）。

---

## 数据模型映射

### Langfuse

| InferGlow | Langfuse | 说明 |
|-----------|----------|------|
| RunRecord | Trace | 一次完整执行 |
| RunRecord.ToolCalls | Span (type: tool) | 子步骤 |
| FeedbackScore | Score | 评分 |
| RunRecord.SessionID | Session | 多轮分组 |

```json
POST /api/public/ingestion
{
  "batch": [
    {
      "type": "trace-create",
      "body": {
        "id": "uuid",
        "name": "weather-query",
        "input": "北京天气",
        "output": "今天28度晴",
        "sessionId": "session-123",
        "metadata": {"tokens": 234, "tool_calls": ["get_weather"]}
      }
    },
    {
      "type": "score-create",
      "body": {
        "traceId": "uuid",
        "name": "correctness",
        "value": 4.5,
        "comment": "回答准确"
      }
    }
  ]
}
```

### LangSmith

| InferGlow | LangSmith | 说明 |
|-----------|-----------|------|
| RunRecord | Run | 一次执行 |
| FeedbackScore | Feedback | 评分 |

```json
POST /runs
{
  "id": "uuid",
  "name": "weather-query",
  "run_type": "chain",
  "inputs": {"question": "北京天气"},
  "outputs": {"answer": "今天28度晴"},
  "start_time": "...",
  "end_time": "...",
  "session_id": "session-123"
}

POST /feedback
{
  "run_id": "uuid",
  "key": "correctness",
  "score": 4.5,
  "comment": "回答准确"
}
```

---

## LLM-as-Judge 评估器（移植 OpenEvals Prompt）

```go
// evalbridge/judge.go

// 评估 Prompt 模板（移植自 OpenEvals，MIT 协议）
const (
    CorrectnessPrompt = `You are a teacher grading...Score 1-5...`
    HallucinationPrompt = `...`
    RelevancePrompt = `...`
    ConcisenessPrompt = `...`
    HarmfulnessPrompt = `...`
)

// LLMJudge 使用 LLM 对 Agent 输出打分。
type LLMJudge struct {
    Provider model.ModelRequester // 复用 InferGlow model 模块
    Model    string               // 打分用的模型
}

// Evaluate 对单条输出执行评估，返回分数和评语。
func (j *LLMJudge) Evaluate(ctx context.Context, input, output, reference string) (*FeedbackScore, error)
```

---

## Langfuse 部署（团队用）

```yaml
# deploy/langfuse/docker-compose.yml
services:
  langfuse:
    image: langfuse/langfuse:latest
    ports:
      - "3000:3000"
    environment:
      - DATABASE_URL=postgresql://postgres:postgres@db:5432/langfuse
      - NEXTAUTH_SECRET=${NEXTAUTH_SECRET}
      - SALT=${SALT}
    depends_on:
      - db

  db:
    image: postgres:16
    volumes:
      - langfuse_data:/var/lib/postgresql/data
    environment:
      - POSTGRES_PASSWORD=postgres

volumes:
  langfuse_data:
```

---

## 实施步骤

| 步骤 | 内容 | 工作量 | 前置 |
|------|------|:------:|------|
| 1 | `evalbridge/` 包：Reporter 接口 + RunRecord/FeedbackScore 类型 | ~60 行 | 无 |
| 2 | `LangfuseReporter` 实现（HTTP POST + Basic Auth） | ~80 行 | 步骤 1 |
| 3 | `LangSmithReporter` 实现（HTTP POST + Bearer） | ~80 行 | 步骤 1 |
| 4 | `LLMJudge` 评估器（5 个 Prompt 常量 + 调 model 打分） | ~120 行 | 无 |
| 5 | `FormatHTML()` 本地报告 | ~150 行 | 无 |
| 6 | eval Runner 集成：执行后自动调 Reporter + Judge | ~40 行 | 步骤 1-4 |
| 7 | 环境变量配置 + Reporter 工厂 | ~30 行 | 步骤 2-3 |
| 8 | `deploy/langfuse/docker-compose.yml` | ~30 行 | 无 |

**总计：~590 行新增代码 + 1 个 docker-compose 文件**

---

## 用户切换平台的体验

```bash
# 用 Langfuse（默认）
export EVAL_REPORTER=langfuse
export LANGFUSE_ENDPOINT=http://localhost:3000
export LANGFUSE_PUBLIC_KEY=pk-xxx
export LANGFUSE_SECRET_KEY=sk-xxx
go run ./eval/cmd --suite weather

# 切换到 LangSmith（只改环境变量）
export EVAL_REPORTER=langsmith
export LANGSMITH_API_KEY=lsv2-xxx
go run ./eval/cmd --suite weather

# 不上报（仅本地）
unset EVAL_REPORTER
go run ./eval/cmd --suite weather --html report.html
```

---

## 不建议做的

- ❌ 自建 Dashboard / Web UI——Langfuse 已经够好
- ❌ 仿造 OpenEvals Go SDK——核心只是 5 个 Prompt 文本
- ❌ 同时上报多个平台——一次只激活一个 Reporter，简单可靠
- ❌ 在 eval 模块内硬编码平台逻辑——通过 Reporter 接口解耦
