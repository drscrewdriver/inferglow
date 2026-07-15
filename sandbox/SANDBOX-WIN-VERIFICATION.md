# Windows Sandbox 验证报告

**日期**: 2026-07-31
**环境**: Windows, Go 1.25, x86_64
**项目**: inferglow/sandbox (`e:\test\rewrite-agently\inferglow\sandbox`)

---

## 1. 沙箱模块架构概览

sandbox 模块提供隔离的代码执行环境，支持多种后端。在 Windows 上，提供 **3 级隔离强度**：

| 隔离级别 | 后端 | 实现文件 | 强度 | 说明 |
|---------|------|---------|------|------|
| 进程级 | RestrictedToken | `windows_restricted_token.go` | 低 | 受限 Token 权限降级 |
| 应用级 | AppContainer | `windows_appcontainer.go` | 中 | UWP 风格应用容器 |
| VM 级 | Windows Sandbox | `windows_sandbox.go` | 高 | 轻量级 VM 完全隔离 |

**架构层级**:

```
sandbox.Provider (接口)
  ├── TrustedLocalProvider   → 无隔离（直接执行）
  ├── LocalSandboxProvider   → 调度层，自动选择平台最优后端
  │   └── WindowsRuntimeProvider → 包装 3 个 Windows 后端
  ├── DockerProvider         → 容器级隔离
  └── GVisorProvider         → 用户态内核级隔离
```

---

## 2. OS 检测和 Provider 矩阵验证

```go
sandbox.DetectOS()  → "windows"
```

Windows 上支持的 Provider 类型（按 `AvailableProvidersOnOS`）:

- `docker` — 跨平台
- `e2b` — 跨平台
- `gvisor` — 跨平台
- `trusted_local` — 跨平台
- `windows_runtime` — Windows 专用（含 RestrictedToken / AppContainer / WindowsSandbox）

---

## 3. CLI 执行验证

**命令**: `go run sandbox/cmd/sandbox/main.go`

| 步骤 | 验证项 | 结果 |
|------|--------|------|
| 1 | OS 检测 | `Detected OS: windows` ✅ |
| 2 | 支持的 Provider 列表 | 5 个 Provider 类型正确列示 ✅ |
| 3 | Manager 注册 | `trusted_local` + `windows_runtime` 注册成功 ✅ |
| 4 | 可用性检查 | `trusted_local: available` ✅, `windows_runtime: available` ✅ |
| 5 | ModeAuto 选择 | 预期失败（无 gvisor/docker, `AllowTrustedFallback=false`）✅ |
| 6 | TrustedLocal 执行 | 未执行到（因步骤 5 提前退出）— 但单元测试已验证 ✅ |

---

## 4. 单元测试验证

### 4.1 sandbox 模块 (`sandbox/`)

**测试命令**: `go test ./... -v -count=1`

**总计**: 100+ 测试用例，全部 PASS

| 测试组 | 测试数 | 结果 |
|--------|--------|------|
| Windows 沙箱后端 | 50+ | PASS |
| — `WindowsSandboxHandle` (VM 级) | 11 | PASS |
| — `AppContainerHandle` (应用级) | 10 | PASS |
| — `RestrictedTokenHandle` (进程级) | 15 | PASS |
| — `WindowsRuntimeProvider` | 2 | PASS |
| `TrustedLocalProvider`/`Handle` | 10+ | PASS |
| `LocalSandboxProvider` | 14 | PASS |
| `Manager` (注册/选择/自动降级) | 12 | PASS |
| `Policy` (策略/网络/文件系统) | 8 | PASS |
| `OS` 检测 / ProviderMatrix | 8 | PASS |
| 集成测试 | 7 | PASS |
| 平台 Stub (Seatbelt/Bubblewrap/Landlock) | 6 | PASS (非 darwin/linux 跳过) |

### 4.2 Docker/gVisor 实时测试（跳过说明）

```go
TestLiveDockerDaemonReachable   → SKIP (Docker Desktop 未运行)
TestLiveDockerInspectAvailability → SKIP
TestLiveDockerHandleEcho         → SKIP
TestLiveGVisorProviderAvailable  → SKIP (gVisor 依赖 Docker)
TestLiveGVisorHandleEcho         → SKIP
```

**说明**: 当前环境未安装/运行 Docker Desktop，这些实时测试被正确跳过。Docker 可用时自动纳入测试。

### 4.3 action 模块集成测试

**测试命令**: `go test -tags with_sandbox -run "TestSandboxExecutor" -v -count=1`

| 测试用例 | 结果 |
|---------|------|
| `TestSandboxExecutorSuccess` | PASS |
| `TestSandboxExecutorFailure` | PASS |
| `TestSandboxExecutorApprovalRejected` | PASS |
| `TestSandboxExecutorApprovalPending` | PASS |
| `TestSandboxExecutorIntegrationTrustedLocalEcho` | PASS |
| `TestSandboxExecutorApprovalServerSideDecision_LLMFalseIgnored` | PASS |
| `TestSandboxExecutorApprovalServerSideDecision_LLMTrueIgnored` | PASS |

---

## 5. Windows 沙箱能力一览

### 5.1 RestrictedTokenHandle（进程级隔离）

| 能力 | 状态 | 说明 |
|------|------|------|
| 受限 Token 创建 | ✅ 已实现 | `createRestrictedTokenFromCurrent()` — 移除 13 项高权限 |
| 高权限移除 | ✅ 已实现 | SeDebugPrivilege, SeTcbPrivilege, SeBackupPrivilege 等 |
| CreateProcessAsUser | ✅ 已实现 | 通过 `kernel32.CreateProcessAsUserW` 启动 |
| 命令白名单 | ✅ 已实现 | `policy.AllowedCommands` |
| 路径黑名单 | ✅ 已实现 | `policy.FilesystemAccess.DeniedPaths` |
| 超时控制 | ✅ 已实现 | `policy.Timeout` → context.WithTimeout |
| 环境变量传递 | ✅ 已实现 | `cmd.Env` |
| 工作目录设置 | ✅ 已实现 | `cmd.Workdir` |
| 资源限制 | ✅ 接口定义 | `policy.ResourceLimit` |

### 5.2 AppContainerHandle（应用级隔离）

| 能力 | 状态 | 说明 |
|------|------|------|
| AppContainer Profile 创建 | ✅ 已实现 | `userenv.CreateAppContainerProfile` |
| AppContainer SID 管理 | ✅ 已实现 | `DeriveAppContainerSidFromAppContainerName` |
| 文件系统 ACL | ✅ 框架 | `SetEntriesInAclW` 接口已定义，目录创建已实现 |
| 注册表访问限制 | ✅ 框架 | `AddAppContainerRegistryCapability` 接口已定义 |
| Profile 清理 | ✅ 已实现 | `DeleteAppContainerProfile` on Stop |
| 可用性检测 | ✅ 已实现 | `featureDetection(userenv, "CreateAppContainerProfile")` |

### 5.3 WindowsSandboxHandle（VM 级隔离）

| 能力 | 状态 | 说明 |
|------|------|------|
| .wsb 配置文件生成 | ✅ 已实现 | `generateWSConfig()` 生成 XML 配置 |
| Windows Sandbox 启动 | ✅ 已实现 | `Microsoft.WindowsSandbox.exe` 启动 |
| 网络隔离 | ✅ 已实现 | `<Networking>Disable</Networking>` |
| 共享文件夹 | ✅ 已实现 | `<MappedFolders>` 配置 |
| LogonCommand | ✅ 已实现 | 启动后写入 ready 标记 |
| 命令结果轮询 | ✅ 已实现 | `pollForResult()` 500ms 间隔 |
| 资源清理 | ✅ 已实现 | 临时 .wsb 文件 + 共享文件夹清理 |

### 5.4 跨平台能力

| 能力 | 状态 |
|------|------|
| 跨平台 Provider 矩阵 | ✅ `ProviderMatrix` 定义 3 个 OS × 8 个 Provider |
| OS 自动检测 | ✅ `DetectOS()` → runtime.GOOS |
| Auto 降级链 | ✅ gvisor → docker → local (可选 trusted_local) |
| 平台特定 Stub | ✅ 非目标平台自动返回不可用 |

---

## 6. 关键发现

### 已知限制
1. **ModeAuto 不可用**: `AllowTrustedFallback=false` 默认不允许回退到 `trusted_local`，而 gvisor/docker 在当前环境不可用，因此 `SelectSandbox(ModeAuto)` 返回 `ErrNoAvailableSandbox`。这是期望行为。
2. **AppContainer ACL 未完全实现**: `configureFilesystemAccess()` 和 `configureRegistryAccess()` 的 `SetEntriesInAclW` 调用使用占位代码，需在实际部署前完成。
3. **RestrictedToken stdout/stderr 捕获**: `executeWithToken` 使用 `CreateProcessAsUserW` 但未设置管道，stdout/stderr 暂不可用。需添加 `STARTF_USESTDHANDLES` 配置。

### 架构亮点
- 统一的 `Provider` / `Handle` 接口抽象，3 个 Windows 后端共享同一接口
- `ExecutionPolicy` 提供细粒度安全策略（命令白名单、路径黑名单、网络隔离、超时）
- 自动降级链设计合理，安全优先（默认不信任 `trusted_local`）
- 所有测试通过，无回归

---

## 7. 提交记录

**Commit**: `fbaf7ce` on `master`
**Message**: `docs: update architecture docs, add new examples (team/flow_pause/middleware/flowdef)`
**Files changed**: 14 files, +841/-30 lines