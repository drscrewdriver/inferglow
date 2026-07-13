# example_workspace - 工作区 / Workspace

## 概述 / Overview

本示例演示如何使用 `workspace` 模块进行安全的文件 IO 操作和文件血缘关系管理。Workspace 提供基于根目录的路径隔离，自动防御路径穿越攻击，同时支持只读模式。文件血缘（Lineage）管理追踪文件的衍生关系，支持血缘记录、祖先/后代查询和环检测。

This example demonstrates how to use the `workspace` module for secure file I/O operations and file lineage management. The Workspace provides root-directory-based path isolation with automatic path traversal attack prevention, and supports read-only mode. File lineage management tracks file derivation relationships, supporting lineage recording, ancestor/descendant queries, and cycle detection.

## 核心概念 / Core Concepts

- **Workspace / 工作区**: 基于根目录的安全文件操作环境，所有路径相对于 RootDir
- **SafePath / 安全路径**: 自动检测并阻止路径穿越（如 `../../etc/passwd`）和绝对路径
- **ReadOnly / 只读模式**: 防止写入操作，返回 `ErrReadOnly` 错误
- **FileLineage / 文件血缘**: 追踪文件的创建/变换关系，形成有向无环图（DAG）
- **LineageNode / 血缘节点**: 记录文件路径、操作类型、创建者、父节点列表和元数据
- **LineageStore / 血缘存储**: 支持 Record、Ancestors、Descendants、Children、Parents 查询
- **环检测 / Cycle Detection**: 自动检测并阻止血缘关系中的循环引用

## 前置条件 / Prerequisites

- Go 1.21+
- 示例使用系统临时目录（`os.TempDir()`）创建 Workspace，运行后自动清理
- The example uses the system temp directory (`os.TempDir()`) to create the Workspace, which is automatically cleaned up after running

## 使用示例 / Usage Example

代码演示了以下 5 个场景：

1. **创建 Workspace**: 使用 `workspace.New` 构造 Workspace，配置 RootDir、MaxFileSize（1MB）和 MaxFileCount（100）。
2. **路径穿越防护**: 测试 4 种路径的 `SafePath` 结果：合法路径通过，`../../etc/passwd` 和绝对路径被拒绝并返回 `ErrPathOutsideRoot`。
3. **安全文件 IO**: 执行 `MkdirAll`、`WriteFile`、`ReadFile`、`ListDir`、`FileCount` 等操作，并展示只读模式下 `WriteFile` 返回 `ErrReadOnly`。
4. **文件血缘管理**: 使用 `NewMemoryLineageStore` 构建一个包含 4 个节点的文件血缘 DAG（raw.txt -> cleaned.txt -> report.txt -> summary.txt），演示 Ancestors、Descendants、Children、Parents 查询，以及环检测（试图将 raw.txt 的父节点设为 summary.txt 时触发 `ErrLineageCycle`）。
5. **血缘持久化**: 使用 `SaveLineageToFile` 将血缘保存到 Workspace 内的 JSON 文件，再用 `LoadLineageFromFile` 加载并验证内容一致性。

The code demonstrates 5 scenarios:

1. **Creating Workspace**: Construct a Workspace with `workspace.New`, configuring RootDir, MaxFileSize (1MB), and MaxFileCount (100).
2. **Path Traversal Protection**: Test `SafePath` on 4 paths: legitimate paths pass, `../../etc/passwd` and absolute paths are rejected with `ErrPathOutsideRoot`.
3. **Secure File I/O**: Perform `MkdirAll`, `WriteFile`, `ReadFile`, `ListDir`, `FileCount` operations, and demonstrate `WriteFile` returning `ErrReadOnly` in read-only mode.
4. **File Lineage Management**: Build a 4-node file lineage DAG using `NewMemoryLineageStore` (raw.txt -> cleaned.txt -> report.txt -> summary.txt), demonstrating Ancestors, Descendants, Children, Parents queries, and cycle detection (attempting to set raw.txt's parent to summary.txt triggers `ErrLineageCycle`).
5. **Lineage Persistence**: Save lineage to a JSON file within the Workspace using `SaveLineageToFile`, then load and verify content consistency with `LoadLineageFromFile`.

## 运行验证 / Running the Example

```
cd examples
go run example_workspace.go
```

预期输出会依次展示：

- Workspace 创建成功，显示 Root 路径、MaxFileSize 和 MaxFileCount
- 路径穿越测试：合法路径返回绝对路径，非法路径返回错误和拦截提示
- 文件 IO 操作成功，ReadFile 读取到写入的内容，只读模式 WriteFile 返回 ErrReadOnly
- 血缘 DAG 构建成功，Ancestors 查询返回完整祖先链，Descendants 查询返回完整后代链，环检测正确触发 ErrLineageCycle
- 血缘持久化保存和加载成功，加载后的 Ancestors 查询结果与原始一致

Expected output shows:

- Workspace created successfully, showing Root path, MaxFileSize, and MaxFileCount
- Path traversal test: legitimate paths return absolute paths, invalid paths return errors with interception hints
- File I/O operations succeed, ReadFile retrieves written content, read-only WriteFile returns ErrReadOnly
- Lineage DAG built successfully, Ancestors returns the full ancestor chain, Descendants returns the full descendant chain, cycle detection correctly triggers ErrLineageCycle
- Lineage persistence saves and loads successfully, loaded Ancestors query matches the original

## 预期输出 / Expected Output

输出着重展示 workspace 模块的安全隔离能力和数据血缘追踪能力：SafePath 确保文件操作不会逃逸根目录，LineageStore 帮助开发者追踪数据的来龙去脉，便于数据溯源和审计。

The output highlights the workspace module's security isolation and data lineage tracking capabilities: SafePath ensures file operations do not escape the root directory, and LineageStore helps developers trace data provenance for data溯源 and auditing.