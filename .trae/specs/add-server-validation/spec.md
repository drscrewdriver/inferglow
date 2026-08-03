# Server Validation Enhancement Spec

## Why

server 模块 42+ 个 handler 中大量重复的手动 if-else 字段校验（`if cfg.Name == ""`），且当前 `/openapi.json` 是硬编码的 stub，无法用于文档生成或客户端 SDK 生成。

## What Changes

### P0 — 引入 go-playground/validator
- 给 server 模块的请求/响应结构体添加 `validate` struct tag
- 在 handler 层用 `validate.Struct()` 替换手动 if-else 检查
- 保留无法用 tag 表达的业务逻辑校验（如 flowdef 的 DAG 环检测、OutputValidator 的 JSON schema 校验）

### P1 — 编写真正的 OpenAPI 3.0 spec 文件
- 用 YAML 编写 `api/openapi.yaml`，覆盖 server 所有 42+ 路由
- 包含完整的 request body schema、response schema、path parameters
- 不做运行时校验，不引入 kin-openapi 运行时依赖

### P2 — 评估代码生成方案
- 调研 kin-openapi / ogen / oapi-codegen 在 inferglow 场景下的适用性
- 形成文档记录，不下代码

## Impact
- Affected code: `server/` 模块下的 handler 文件、结构体定义
- 新增文件: `api/openapi.yaml`
- 新增依赖: `github.com/go-playground/validator/v10`（仅 server 模块）
- No breaking changes to API contract

## ADDED Requirements

### Requirement: Struct Tag Validation
server 模块的请求结构体 SHALL 使用 `validate` tag 声明字段级校验规则。

#### Scenario: 替换手动 if-else
- **WHEN** handler 接收到请求体并解码到结构体
- **THEN** 调用 `validate.Struct()` 校验，失败返回 400 错误

### Requirement: OpenAPI Spec 文件
API 的 OpenAPI 描述 SHALL 从硬编码 map 迁移到独立的 YAML 文件。

#### Scenario: 完整的 API 文档
- **WHEN** 开发者需要查阅 API 接口
- **THEN** `api/openapi.yaml` 包含所有路由的路径、参数、请求/响应结构

## MODIFIED Requirements

### Requirement: 现有 handler 校验逻辑
- 字段级 if-else 检查 → 替换为 validator tag
- 业务逻辑校验（如资源是否存在、权限检查）→ 保持不变