# macOS Containers（Apple Container）信息储备与实现方案 — Spec

> 编写时间：2026-08-15
> 状态：储备（信息调研 + 实现规划，不实施代码）
> 总纲：[Seatbelt Loader 迁移规划](seatbelt-loader-spec.md)（进程级沙箱对照）

---

## 1. 事实储备（2026-08 调研确认）

### 1.1 项目概况

| 项 | 值 |
|----|----|
| 发布 | WWDC 2025（2025-06）发布 **Containerization Framework** + **Container CLI** |
| 开源 | [apple/container](https://github.com/apple/container)（Swift）；2026-06 达 **1.0 正式版**（36k+ stars） |
| 底层库 | `apple/containerization`（独立 Swift 包，可被其他工具直接调用，不限于 CLI） |
| 镜像格式 | **OCI 兼容**（Docker Hub / GHCR 直接拉取） |
| Daemon | 无常驻 daemon，按需启停 |
| 启动性能 | 轻量 VM 亚秒级启动（官方宣称接近容器启动速度） |

### 1.2 架构：VM-per-Container

与传统方案的关键区别：

```
传统（Docker Desktop / OrbStack / Lima）：
  macOS → 单个 Linux VM → 多个容器共享内核

Apple Container：
  macOS ├─ VM(Container A)
        ├─ VM(Container B)      ← 每个容器一个独立轻量级 VM
        └─ VM(Container C)      ← 虚拟级隔离，非共享内核
```

六层架构（自顶向下）：

```
1. container CLI（Swift，Docker 兼容语义）
2. Containerization Framework（Swift 包，容器管理）
3. Virtualization.framework（Linux VM 管理）    ← 虚拟化核心
4. vmnet 框架（容器虚拟网络）
5. XPC（进程间通信）/ launchd（服务管理）
6. Keychain（注册表凭据）/ 统一日志（应用日志）
```

### 1.3 平台限制（关键边界）

- **仅 Apple Silicon**（M1/M2/M3/M4 系列），Intel Mac 不可用
- **macOS 26（Tahoe）为最佳目标**：虚拟化调度优化、vmnet 网络增强（容器间通信）、子网分配修复
- macOS 15.5+ 可用但 **Apple 明确不修复旧系统问题**
- 网络：macOS 15 仅支持容器网络隔离；容器间通信需 macOS 26

## 2. 与现有沙箱体系的关系

对齐 dsh 架构结论（`docs/subsystems/sandbox.md` L5）：**Containers/microVMs/远程执行是 whole capability seams 的兄弟实现，不是 `ctx.sandbox` 的 provider**。

```
进程级沙箱（ctx.sandbox）        容器/VM 级（能力缝兄弟实现）
├─ Linux: bwrap / Landlock       ├─ macOS: Apple Container（本 spec）
├─ macOS: Seatbelt（进程级）      ├─ Docker（inferglow DockerProvider）
└─ Windows: ACL token / AppContainer ├─ E2B（远程沙箱）
                                   └─ gVisor（runsc 容器运行时）
```

互补定位：进程级沙箱隔离"单命令的文件效果"；容器级提供完整执行环境（依赖、系统库、网络栈）隔离——后者适合"整个 Agent 执行世界"（对应 dsh 的 e2b 家族语义）。

## 3. 实现方案

### 3.1 inferglow（Go）

方案一：**DockerProvider 语义映射**（最小改动）

- Apple Container 镜像 OCI 兼容 → Docker 镜像可直接 `container run`
- 在 DockerProvider 的客户端抽象层（`DockerClient` 接口）旁新增 `ContainerRunner` 接口，darwin 实现走 `container run --rm <image> <cmd>`

方案二：**独立 `container_runner.go`**（`//go:build darwin`）

```
探测：exec.LookPath("container") + `container version` 输出校验
创建：container create --name <id> <image>
执行：container exec <id> <cmd>       （stdout/stderr 管道捕获，对齐 launchProcessWithIO 语义）
清理：container rm -f <id>
```

- 探测条件：`runtime.GOOS == darwin` && Apple Silicon && `container` CLI 存在（`container version` 或 `container info` 校验 macOS 26+）
- 失败即 `ErrProviderUnavailable`（fail-closed，与现有 Provider 一致）

### 3.2 dsh（TypeScript）

可做 `packages/container/` 家族（对齐 e2b 家族结构）：

| 包 | ctx key | 职责 |
|----|---------|------|
| `container` | `ctx.container` | 容器生命周期所有者（create/exec/rm，CLI 子进程） |
| `fs-container` | `ctx.fs` | 文件系统 seam 的 container 实现（`container cp` / 挂载卷） |
| `subprocess-container` | `ctx.subprocess` | 进程 seam 的 container 实现（`container exec`） |

消费方（bash-local/terminal-bash/lsp-stdio）因"可移植执行世界"架构零改造（与 e2b 家族相同机制）。

## 4. 边界与风险

| 项 | 说明 |
|----|------|
| Apple Silicon 限定 | Intel Mac 不可用；探测必须显式（`uname -m` = arm64） |
| 系统版本限定 | macOS 26 最佳；15.5 可用但不修复——文档化支持矩阵 |
| VM 级开销 | 每容器一个 VM（即使轻量），批量短命令场景开销高于进程级沙箱；定位为"执行世界"而非"单命令包装" |
| 与 Seatbelt 互补 | 进程级（seatbelt loader）负责单命令文件效果；容器负责完整环境——两者并存，不互相替代 |
| 镜像拉取网络 | 首次拉取需网络；离线环境需镜像预缓存（`container pull`） |
| 与 Docker 并存 | OCI 兼容但语义不同（VM-per-Container），不可直接复用 Docker daemon 状态 |

## 5. 决策建议

| 场景 | 选择 |
|------|------|
| inferglow 需要完整执行环境隔离（macOS 26 + Apple Silicon） | 方案二（独立 container_runner.go，探测 fail-closed） |
| 仅需进程级单命令隔离 | Seatbelt loader（见总纲），不需要容器 |
| dsh 生态需要执行世界级隔离 | packages/container/ 家族（对齐 e2b） |
| 多语言共用 | 方案一（DockerProvider 映射，`container run` 统一 CLI 语义） |

## 6. 验收标准（未来实施时）

- 探测：非 darwin / 非 Apple Silicon / 无 CLI → `ErrProviderUnavailable`
- 生命周期：create → exec（stdout/stderr 捕获）→ rm 全链路真实运行
- 隔离断言：容器内写宿主路径失败；容器内环境（依赖/网络栈）完整
- 文档：支持矩阵（macOS 26+ / Apple Silicon）与限制与实现一致

## 7. 参考来源

- WWDC25：Containerization Framework + Container CLI 发布
- [apple/container](https://github.com/apple/container)（Swift 开源，1.0 已发布）
- 2026 实践文章：cnblogs「Apple Container实践」、chenxutan「apple/container 深度拆解」、腾讯云开发者社区「Apple Container 开箱实践」
- dsh `docs/subsystems/sandbox.md`（容器 = 能力缝兄弟实现的定位论述）
