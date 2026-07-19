# Graphify 代码理解工具使用指南

## 一、Graphify 是什么

Graphify 是一个**代码知识图谱生成工具**，它通过 tree-sitter AST 解析将代码库转换为可查询的知识图谱。与让 AI 直接画架构图不同，Graphify 是**确定性解析**（不靠猜），支持 40+ 种语言，代码全程本地处理不外传。

### 核心能力
| 能力 | 说明 |
|------|------|
| AST 解析 | tree-sitter 解析代码语法树，提取函数/类/方法/接口 |
| 关系提取 | 识别跨文件调用、导入、继承、实现关系 |
| 社区检测 | Leiden 算法自动将代码库切分为子系统 |
| God Nodes | 识别最核心的架构枢纽节点 |
| 路径查询 | 查询两个概念间的最短路径 |
| 影响分析 | 修改某节点后，反向查找受影响范围 |

### 输出文件
```
graphify-out/
├── graph.json          # 完整图谱数据（可查询）
├── GRAPH_REPORT.md     # 分析报告（社区/God Nodes/意外连接）
└── manifest.json       # 文件清单和元数据
```

---

## 二、安装

### 2.1 安装 CLI 工具

```bash
# 方式一：uv（推荐）
uv tool install graphifyy

# 方式二：pipx
pipx install graphifyy
```

> 注意：PyPI 包名为 `graphifyy`（双 y），命令名为 `graphify`

### 2.2 安装为 IDE Skill

Graphify 支持作为 Skill 注入到多种 AI 编码助手中：

```bash
# Trae-CN（当前 IDE）
graphify install --platform trae-cn

# 其他平台
graphify install --platform claude      # Claude Code
graphify install --platform cursor      # Cursor
graphify install --platform copilot     # GitHub Copilot
graphify install --platform codex       # Codex
graphify install --platform gemini      # Gemini CLI
graphify install --platform aider       # Aider
```

安装后，Skill 文件位于：
- **Trae-CN**: `~/.trae-cn/skills/graphify/SKILL.md`
- **Claude Code**: `~/.claude/skills/graphify/SKILL.md`

---

## 三、在 IDE 中使用

### 3.1 基本用法

安装 Skill 后，在 IDE 的 AI 对话中直接使用：

```
/graphify .                    # 分析当前目录
/graphify path/to/project      # 分析指定目录
/graphify --update             # 增量更新（只处理变更文件）
/graphify --cluster-only       # 重新聚类（不重新解析）
```

### 3.2 查询用法

图谱构建完成后，可以自然语言查询：

```
/graphify query "Session 模块如何与 Engine 交互？"
/graphify path "NewSession" "Engine"          # 两点间最短路径
/graphify explain "ModelPool"                 # 解释某个节点
/graphify affected "Session"                  # Session 变更影响范围
/graphify god-nodes                           # 列出核心枢纽节点
```

### 3.3 当前 IDE（Trae-CN）集成方式

当前 IDE 通过 **Skill 系统** 集成 Graphify：

1. **全局 Skill 目录**: `~/.trae-cn/skills/` — 所有项目可用
2. **项目级 Skill**: `.trae/skills/` — 仅当前项目可用
3. **触发方式**: 在 AI 对话中输入 `/graphify` 前缀

Skill 本质是一份 **Markdown 指令文件**（SKILL.md），它告诉 AI 助手：
- 何时触发 Graphify 工具
- 如何解读 graphify-out/ 中的输出
- 如何将图谱信息融入代码理解

---

## 四、Inferglow 项目分析结果

### 4.1 分析概况

对 `inferglow/` 子项目运行 Graphify 后的结果：

| 指标 | 数值 |
|------|------|
| 节点数 | 5,607 |
| 边数 | 12,918 |
| 社区数 | 276 |
| EXTRACTED 边占比 | 77% |
| INFERRED 边占比 | 23% |
| Import Cycles | 无 |

### 4.2 God Nodes（架构枢纽）

这些是连接最多的核心抽象，理解它们就理解了项目骨架：

| 排名 | 节点 | 边数 | 含义 |
|------|------|------|------|
| 1 | `NewSession()` | 130 | 会话创建 — 最大的耦合中心 |
| 2 | `DefaultPolicy()` | 98 | 默认安全策略 |
| 3 | `NewActionExtension()` | 84 | 动作扩展注册 |
| 4 | `New()` | 70 | Agent 构造 |
| 5 | `NewFlow()` | 69 | Flow 创建 |
| 6 | `NewModelPool()` | 65 | 模型池 |
| 7 | `NewSignalNet()` | 61 | 信号网络 |
| 8 | `NewStep()` | 60 | Flow 步骤 |
| 9 | `NewSessionExtension()` | 47 | 会话扩展 |
| 10 | `newMockProvider()` | 44 | 测试用 Mock |

### 4.3 关键社区（子系统划分）

| 社区 | 核心节点 | 内聚度 | 对应模块 |
|------|---------|--------|---------|
| Community 0 | TriggerFlow 相关 | 0.05 | orchestrator/triggerflow |
| Community 1 | Action Policy/Approval | 0.05 | action/approval |
| Community 3 | Rate Limiter | 0.06 | security/ratelimit |
| Community 7 | Operator Handler | 0.05 | orchestrator/operator |
| Community 11 | AttemptRunner (重试) | 0.07 | model/retry |
| Community 12 | TurnLoop (取消) | 0.07 | orchestrator/agent/cancel |
| Community 21 | Checkpoint 持久化 | 0.09 | flow/persistence |
| Community 26 | ModelPool | 0.08 | model/pool |
| Community 29 | Engine | 0.09 | orchestrator/agent/engine |
| Community 31 | Agent | 0.08 | orchestrator/agent |
| Community 32 | ActionExtension | 0.13 | action/extension |

### 4.4 架构洞察

**发现 1: NewSession 是最大 God Node（130 edges）**
- 说明 Session 创建与过多组件耦合，是重构的优先目标

**发现 2: 社区内聚度普遍偏低（0.05-0.13）**
- 说明模块间交叉引用较多，边界不够清晰

**发现 3: 无 Import Cycles**
- 依赖方向健康，没有循环依赖

**发现 4: 276 个社区**
- 说明项目功能分散度较高，模块化程度好但需要关注跨社区通信

---

## 五、对代码优化的指导价值

### 5.1 重构优先级排序

```
God Nodes 边数越高 → 耦合越重 → 重构收益越大
社区内聚度越低 → 边界越模糊 → 越需要梳理
```

### 5.2 影响分析场景

修改 `NewSession` 前：
```bash
graphify affected "NewSession"
# 反向遍历找出所有受影响的下游节点
```

### 5.3 路径查询场景

理解两个模块如何关联：
```bash
graphify path "Session" "Engine"
# 找出 Session 和 Engine 之间的最短调用链
```

### 5.4 增量更新

代码变更后无需重新全量分析：
```bash
graphify update .
# 只重新解析变更文件，更新图谱
```

---

## 六、命令速查

| 命令 | 用途 |
|------|------|
| `graphify .` | 全量分析当前目录 |
| `graphify update .` | 增量更新图谱 |
| `graphify cluster-only .` | 重新聚类（不重新解析） |
| `graphify query "问题"` | BFS 查询 |
| `graphify query "问题" --dfs` | DFS 查询（追踪路径） |
| `graphify path "A" "B"` | 两节点最短路径 |
| `graphify explain "X"` | 解释某节点 |
| `graphify affected "X"` | 影响范围分析 |
| `graphify god-nodes` | 列出核心枢纽 |
| `graphify install --platform trae-cn` | 安装到 Trae-CN |
| `graphify uninstall` | 从所有平台卸载 |
| `graphify uninstall --purge` | 卸载并删除 graphify-out/ |

---

## 七、与其他工具对比

| 维度 | Graphify | AI 直接画图 | IDE 内置索引 |
|------|----------|------------|-------------|
| 解析方式 | tree-sitter AST（确定性） | LLM 推测（有幻觉） | 语言服务器（不完整） |
| 代码隐私 | 本地处理 | 可能上传 | 本地 |
| 跨文件关系 | 完整提取 | 部分遗漏 | 有限 |
| 社区检测 | Leiden 算法 | 无 | 无 |
| 增量更新 | 支持 | 不支持 | 支持 |
| 可视化 | HTML 交互图谱 | Mermaid 静态图 | 无 |

---

*生成时间: 2026-07-29*
*Graphify 版本: 0.9.29*
*分析目标: inferglow/ 子项目*
