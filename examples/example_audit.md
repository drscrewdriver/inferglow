# example_audit - 审计链 / Audit Chain

## 概述 / Overview

本示例演示如何使用 `audit` 模块构建防篡改的审计链（Audit Chain）。审计链是一个仅可追加（append-only）的哈希链式日志，每条条目包含前一条的哈希值（PrevHash），并通过 HMAC-SHA256 签名确保完整性。支持基于 Source、Action 等字段的查询过滤，以及 JSON、CSV、Text 三种格式的导出。

This example demonstrates how to use the `audit` module to build a tamper-proof audit chain. The audit chain is an append-only, hash-chained log where each entry contains the hash of the previous entry (PrevHash) and is signed via HMAC-SHA256 to ensure integrity. It supports query filtering by fields such as Source and Action, and export in JSON, CSV, and Text formats.

## 核心概念 / Core Concepts

- **AuditChain / 审计链**: 仅可追加的哈希链式日志结构，支持签名验证
- **AuditEntry / 审计条目**: 包含 Source、Action、Input、Output、Duration、Metadata 等字段的单个记录
- **HMAC-SHA256 签名 / HMAC-SHA256 Signature**: 使用密钥对每条条目计算签名，防止篡改
- **VerifyChain / 全链验证**: 重算所有条目的哈希链和签名，检测完整性
- **QueryFilter / 查询过滤**: 按 Source、Action 等字段过滤审计条目
- **Export / 导出**: 支持 JSON、CSV、Text 三种序列化格式

## 前置条件 / Prerequisites

- Go 1.21+
- 无需外部依赖，审计链使用内存存储后端（`StorageBackend: "memory"`）
- No external dependencies; the audit chain uses the in-memory storage backend (`StorageBackend: "memory"`)

## 使用示例 / Usage Example

代码演示了以下 6 个场景：

1. **创建 AuditChain**: 使用 `audit.NewAuditChain` 构造审计链，配置签名密钥、存储后端和最大条目数。
2. **追加审计条目**: 通过 `chain.Append` 追加 4 条不同类型的条目（agent decision、action execute、model request、final decision），每条自动获得唯一 ID、时间戳、PrevHash 和签名。
3. **签名与验证**: 使用 `audit.VerifyEntry` 验证单条条目的签名有效性，并演示篡改哈希值后签名验证失败。
4. **全链验证**: 调用 `chain.VerifyChain()` 验证整个链的完整性，包括哈希链连续性和签名。
5. **查询过滤**: 使用 `chain.Query` 按 Source 或 Action 过滤审计条目。
6. **导出**: 使用 `chain.Export` 将审计链导出为 JSON、CSV、Text 格式。

The code demonstrates 6 scenarios:

1. **Creating AuditChain**: Construct an audit chain with `audit.NewAuditChain`, configuring the signature key, storage backend, and max entries.
2. **Appending Entries**: Append 4 entries of different types (agent decision, action execute, model request, final decision) via `chain.Append`, each automatically getting a unique ID, timestamp, PrevHash, and signature.
3. **Signature and Verification**: Verify a single entry's signature with `audit.VerifyEntry`, and demonstrate that tampering with the hash causes verification to fail.
4. **Full Chain Verification**: Call `chain.VerifyChain()` to validate the integrity of the entire chain, including hash chain continuity and signatures.
5. **Query Filtering**: Use `chain.Query` to filter audit entries by Source or Action.
6. **Export**: Export the audit chain to JSON, CSV, and Text formats using `chain.Export`.

## 运行验证 / Running the Example

```
cd examples
go run example_audit.go
```

预期输出会依次展示：

- AuditChain 创建成功，`IsEnabled` 为 true，初始长度为 0
- 追加 4 条条目后，长度变为 4，每条显示截断后的哈希值
- 原始条目签名验证通过（true），篡改后的条目签名验证失败（false）
- `VerifyChain` 返回 PASSED，表示链完整
- 按 Source=agent 过滤命中 2 条，按 Action=execute 过滤命中 1 条
- 三种格式的导出内容（JSON 数组、CSV 表格、文本报告）

Expected output shows:

- AuditChain created successfully, `IsEnabled` is true, initial length is 0
- After appending 4 entries, length becomes 4, each showing a truncated hash
- Original entry signature verification passes (true), tampered entry fails (false)
- `VerifyChain` returns PASSED, indicating chain integrity
- Filter by Source=agent returns 2 entries, filter by Action=execute returns 1 entry
- Exports in three formats (JSON array, CSV table, text report)

## 预期输出 / Expected Output

输出着重展示审计链的完整性保障能力：签名确保每条条目内容未被篡改，哈希链确保条目顺序未被破坏。查询和导出功能帮助开发者将审计日志集成到外部系统中。

The output highlights the audit chain's integrity assurance: signatures ensure each entry's content has not been tampered with, and the hash chain ensures entry order has not been broken. Query and export features help developers integrate audit logs into external systems.