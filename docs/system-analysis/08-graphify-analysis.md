# 08 · Graphify 知识图谱分析

## 一、图谱概览

- 节点数: 8017
- 边数: 17577
- 社区数: 414
- 边类型分布: 80% EXTRACTED + 20% INFERRED
- 循环依赖: 无

## 二、God Nodes（架构枢纽）

Graphify 识别的最高连接度节点，反映架构核心抽象。Top 10 表格:

| 排名 | 节点 | 边数 | 含义 |
|------|------|------|------|
| 1 | `NewSession()` | 148 | Session 创建：跨模块最大连接枢纽 |
| 2 | `DefaultPolicy()` | 101 | 默认策略：沙箱策略配置中心 |
| 3 | `NewActionExtension()` | 100 | Action 扩展：Agent 初始化核心 |
| 4 | `New()` | 84 | 通用构造器 |
| 5 | `NewFlow()` | 74 | Flow 构造器 |
| 6 | `Request` | 68 | HTTP 请求模型 |
| 7 | `NewModelPool()` | 65 | 模型池构造器 |
| 8 | `NewStep()` | 64 | Flow Step 构造器 |
| 9 | `NewSignalNet()` | 61 | 信号网络构造器 |
| 10 | `NewSessionExtension()` | 52 | Session 扩展构造器 |

## 三、社区结构

Graphify 将 8017 个节点聚类为 414 个社区。主要社区表格:

| 社区 | 节点数 | 内聚度 | 内容 |
|------|--------|--------|------|
| Store | 20 | 0.04 | 存储相关 |
| NewSession | 70 | 0.08 | Session 创建+测试 |
| NewTurnLoop | 49 | 0.06 | 轮次管理 |
| Operator | 34 | 0.05 | Flow 算子 |
| DefaultPolicy | 56 | 0.08 | 沙箱策略 |
| Agent | 28 | 0.08 | Agent 核心 |
| Engine | 26 | 0.09 | Engine 测试 |
| .executeLoop | 21 | 0.21 | 核心循环 |
| FlowContext | 11 | 0.55 | 最高内聚 |
| Chain | 7 | 0.54 | Middleware 链 |

## 四、跨社区桥接

| 节点 | 桥接社区 | 中介中心性 |
|------|---------|-----------|
| `ReadAll()` | 多个社区 | 0.101 |
| `NewSession()` | 多个社区 | 0.091 |
| `buildAgent()` | 多个社区 | 0.086 |

## 五、出乎意料的连接

Graphify INFERRED 边揭示了跨模块连接:

| 连接 | 跨模块 | 说明 |
|------|--------|------|
| `NewMemoryBridge()` → `NewStore()` | cli → skill | 终端记忆桥接技能存储 |
| `newTestManager()` → `NewManager()` | action → sandbox | Action 测试依赖沙箱管理器 |
| `TestSandboxExecutor*` → `NewPolicyApprovalManager()` | action → approval | 沙箱执行器测试触发审批管理器 |

## 六、架构健康度评估

- **模块化评分**: 高（23 独立 module，无循环依赖）
- **内聚度**: FlowContext 社区 0.55 最高，.executeLoop 社区 0.21
- **枢纽集中度**: Top 10 God Nodes 覆盖核心构造器
- **建议**: 关注低内聚社区（Store 0.04，Operator 0.05）