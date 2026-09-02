# InferGlow 构建与测试总览

> 全仓库（多 module 仓库）的构建 / 测试 / 静态检查方法。
> 涉及 GUI 前端（`gui/`）的启动与打包见 [gui.md](gui.md)。

## 模块总览

仓库采用 **多 module 结构**：根目录 + 29 个子目录各持独立 `go.mod`
（总计 30 个 go.mod）。其中 23 个为核心模块（README 架构图四层）：

| 层 | 模块 |
|---|---|
| 基础层（12） | model, schema, session, sandbox, context, audit, approval, rag, rerank, observability, workspace, resource |
| 中间层（5） | components, flow, action, mcpserver, builtins |
| 编排层（3） | orchestrator, security, eval |
| 应用层（3） | server, cli, examples |

另有 6 个辅助模块（不进四层图）：`desktop`（Wails 壳，独立 build tag）、
`imbridge`、`memory`、`messagebus`、`skill`、`storage`。

> 各子模块的 `go.mod` 以 `replace` 指向仓库内兄弟模块，构建无需联网拉取内部依赖。

## make 目标总览（Linux / macOS）

根 `Makefile` 对所有 module 批量执行同一命令（根模块 + `find -mindepth 2` 子模块）：

```bash
make build           # go build ./...（所有模块）
make build-sandbox   # 带 with_sandbox build tag 构建（启用沙箱隔离）
make test            # go test ./...（所有模块）
make test-sandbox    # 带 with_sandbox tag 测试
make test-all        # test + test-sandbox
make vet             # go vet ./...（所有模块）
make lint            # golangci-lint run ./...（所有模块，需安装 golangci-lint）
make clean           # go clean -cache -testcache
```

CI（`.github/workflows/ci.yml`）即这三类 job：`test`（含 sandbox）、`vet`、`lint`，
外加 `web` job（前端构建，见下）。

## Windows（无 make）替代

PowerShell 逐个模块构建/测试（等价 `make build` / `make test`）：

```powershell
cd e:\test\rewrite-agently\inferglow-github
$mods = Get-ChildItem -Recurse -Filter go.mod | Where-Object { $_.FullName -notmatch 'node_modules' }
foreach ($m in $mods) {
  Push-Location $m.DirectoryName
  go build ./...          # 或 go test ./... / go vet ./...
  Pop-Location
}
```

## 前端（gui/）构建

```bash
cd web
npm install
npm run lint && npm run test && npm run build   # 产物 → ../server/webui（入库）
```

`server/webui` 产物随 commit 提交（`//go:embed` 依赖），**改动前端后必须重新
`npm run build` 并提交产物**，否则 server 与 CI 编译的是旧界面。
GUI 启动/打包/发布完整流程见 [gui.md](gui.md)。

## 已知注意事项（实测）

| 现象 | 原因 | 处理 |
|---|---|---|
| `examples` 模块 `go build` 报"no Go files" | 包文件带 build tag（`with_sandbox` 等），默认 tag 下为空包 | 属预期，用 `make build-sandbox` 覆盖 |
| `orchestrator` 个别测试失败 | 基线既有问题（任务起点 commit 同样失败），非本次改动引入 | 比对基线 worktree 后忽略，单独定位 |
| 全量并发跑时 `builtins` 偶发失败、单独跑通过 | 多模块并发时序抖动 | 单独重跑确认 |
| Windows 测试中路径拼 JSON 报 400（`\U` 非法转义） | `t.TempDir()` 返回 `C:\...` 反斜杠 | 测试内 `strings.ReplaceAll(path, \`\\\`, \`\\\\\`)` |

## 验证基线

每个 commit 的验收标准：

- server 模块：`cd server && go build ./... && go test ./...`
- web 模块：`cd web && npm run lint && npm run test && npm run build`
- 多模块改动：`make build && make test`（Windows 用上方 PowerShell 循环）
# InferGlow 构建与测试总览

> 全仓库（多 module 仓库）的构建 / 测试 / 静态检查方法。
> 涉及 GUI 前端（`gui/`）的启动与打包见 [gui.md](gui.md)。

## 模块总览

仓库采用 **多 module 结构**：根目录 + 29 个子目录各持独立 `go.mod`
（总计 30 个 go.mod）。其中 23 个为核心模块（README 架构图四层）：

| 层 | 模块 |
|---|---|
| 基础层（12） | model, schema, session, sandbox, context, audit, approval, rag, rerank, observability, workspace, resource |
| 中间层（5） | components, flow, action, mcpserver, builtins |
| 编排层（3） | orchestrator, security, eval |
| 应用层（3） | server, cli, examples |

另有 6 个辅助模块（不进四层图）：`desktop`（Wails 壳，独立 build tag）、
`imbridge`、`memory`、`messagebus`、`skill`、`storage`。

> 各子模块的 `go.mod` 以 `replace` 指向仓库内兄弟模块，构建无需联网拉取内部依赖。

## make 目标总览（Linux / macOS）

根 `Makefile` 对所有 module 批量执行同一命令（根模块 + `find -mindepth 2` 子模块）：

```bash
make build           # go build ./...（所有模块）
make build-sandbox   # 带 with_sandbox build tag 构建（启用沙箱隔离）
make test            # go test ./...（所有模块）
make test-sandbox    # 带 with_sandbox tag 测试
make test-all        # test + test-sandbox
make vet             # go vet ./...（所有模块）
make lint            # golangci-lint run ./...（所有模块，需安装 golangci-lint）
make clean           # go clean -cache -testcache
```

CI（`.github/workflows/ci.yml`）即这三类 job：`test`（含 sandbox）、`vet`、`lint`，
外加 `web` job（前端构建，见下）。

## Windows（无 make）替代

PowerShell 逐个模块构建/测试（等价 `make build` / `make test`）：

```powershell
cd e:\test\rewrite-agently\inferglow-github
$mods = Get-ChildItem -Recurse -Filter go.mod | Where-Object { $_.FullName -notmatch 'node_modules' }
foreach ($m in $mods) {
  Push-Location $m.DirectoryName
  go build ./...          # 或 go test ./... / go vet ./...
  Pop-Location
}
```

## 前端（gui/）构建

```bash
cd web
npm install
npm run lint && npm run test && npm run build   # 产物 → ../server/webui（入库）
```

`server/webui` 产物随 commit 提交（`//go:embed` 依赖），**改动前端后必须重新
`npm run build` 并提交产物**，否则 server 与 CI 编译的是旧界面。
GUI 启动/打包/发布完整流程见 [gui.md](gui.md)。

## 已知注意事项（实测）

| 现象 | 原因 | 处理 |
|---|---|---|
| `examples` 模块 `go build` 报"no Go files" | 包文件带 build tag（`with_sandbox` 等），默认 tag 下为空包 | 属预期，用 `make build-sandbox` 覆盖 |
| `orchestrator` 个别测试失败 | 基线既有问题（任务起点 commit 同样失败），非本次改动引入 | 比对基线 worktree 后忽略，单独定位 |
| 全量并发跑时 `builtins` 偶发失败、单独跑通过 | 多模块并发时序抖动 | 单独重跑确认 |
| Windows 测试中路径拼 JSON 报 400（`\U` 非法转义） | `t.TempDir()` 返回 `C:\...` 反斜杠 | 测试内 `strings.ReplaceAll(path, \`\\\`, \`\\\\\`)` |

## 验证基线

每个 commit 的验收标准：

- server 模块：`cd server && go build ./... && go test ./...`
- web 模块：`cd web && npm run lint && npm run test && npm run build`
- 多模块改动：`make build && make test`（Windows 用上方 PowerShell 循环）
