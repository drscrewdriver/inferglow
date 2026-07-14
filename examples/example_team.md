# example_team.go — Multi-Agent 协作

## 中文说明

演示如何使用 `orchestrator/team` 包实现 Host-Specialist 模式的 Multi-Agent 协作。

### 核心概念
- **Coordinator**：多 Agent 协调器，按轮次调度成员执行任务
- **Member**：团队参与者，包含 Agent、Role 和 Handoff 列表
- **AgentRunner**：Agent 执行接口，`AgentAdapter` 将 `*agent.Agent` 适配为 `AgentRunner`
- **Handoff**：角色委派链，planner → coder → reviewer 逐步传递

### 运行方式
```bash
cd examples
go run example_team.go
```

### 示例输出
```
=== Example 1: Basic Team Coordination ===
Task: Build a REST API for user management
Result: planner processed: ...
Round completed in 1 rounds
...
```

---

## English Description

Demonstrates Multi-Agent collaboration using the `orchestrator/team` package in Host-Specialist mode.

### Key Concepts
- **Coordinator**: Multi-agent orchestrator dispatching tasks in rounds
- **Member**: Team participant with Agent, Role, and Handoff chain
- **AgentRunner**: Agent execution interface adapted via `AgentAdapter`
- **Handoff**: Role delegation chain (planner → coder → reviewer)

### Run
```bash
cd examples
go run example_team.go
```