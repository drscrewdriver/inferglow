# 需求族：严格 lint 范围（保 CI 绿）

> 状态：**已交付 ✅**（`cli/config` 代码域已落地，严格 lint 配置已建）
> 领域：工程基础设施（.golangci / Makefile / CI）
> 来源：从 [rich-input-composer-prd.md](../rich-input-composer-prd.md) §13.3 + §12.1 拆出（v0.4 拆分）
> 关联：input 线 composer 子包 `cli/composer/` 落地时纳入严格域（见 PRD §6）。

## 当前状态（2026-08-22 核查）

`cli/config` 代码域已落地，`cli/.golangci-strict.yaml` **已创建**，Makefile 已有 `lint-strict`
目标，CI 亦已加入对应 step——本需求族已交付。

注意：早期方案曾把严格配置放 `cli/config/` 目录内且只扫 `./config/...`，这**覆盖不到 composer 代码**
（golangci-lint 按包粒度启用 linter，`rich_composer.go` 若放 cli 根包则无法单独严格化）。
现已将 composer 调整为独立子包 `cli/composer/`（PRD §6），严格域统一扫两个新子包。

## 背景

[.golangci.yaml](file:///e:/test/rewrite-agently/inferglow/.golangci.yaml) 全局
`disable: errcheck/staticcheck/unused/ineffassign`，CI 的 `Lint` job 直接
`golangci-lint run ./...`（根 + 所有子模块）。若**全局** enable 这些 linter，
存量代码大量未检查的错误处理会立刻撑爆 CI。因此「扩 lint」必须**限定范围**，而不是全局一把梭。

## 方案：仅为新增的严格代码域（新子包）单独启用 strict linter，不动全局存量

1. **保留全局** `.golangci.yaml` 现状（存量 CI 绿）。
2. **新增分包配置** `cli/.golangci-strict.yaml`（放 `cli/` 根，便于统一扫描新子包），
   `default: none` 后显式 `enable: govet, errcheck, staticcheck, unused, ineffassign, revive`。
3. Makefile 增加 `lint-strict` 目标：
   `cd cli && golangci-lint run -c .golangci-strict.yaml ./config/... ./composer/...`
   （composer 子包落地前先只扫 `./config/...`，落地即纳入）。
4. CI 的 `Lint` job 追加一步 `lint-strict`（在全局 `golangci-lint run ./...` 之后的独立 step），
   仅对新增域校验严格度；**不改变**既有全局 lint step 的通过条件。

## 为什么这样保 CI 绿

- 严格校验只覆盖**新写的、可保证 clean** 的代码域；存量模块完全不碰，全局 step 不新增失败源。
- 新增代码在并入前已满足 errcheck 等，`lint-strict` 稳定通过，不会给 PR 引入新的红。

## 验收（已交付后达成）

`make lint-strict` 通过；全局 `make lint`（go vet + golangci）仍通过；CI 两条 path 均绿。

> 注：未来若把老模块逐块迁移到严格域，可逐步把对应目录纳入 `golangci-strict.yaml` 的扫描范围，
> 每迁一块校验一块，保持「先绿后扩」。
