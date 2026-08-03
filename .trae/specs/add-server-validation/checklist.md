# Checklist

- [x] go-playground/validator 依赖添加并初始化
- [x] 核心请求结构体已添加 validate tag
- [x] handler 层手动 if-else 校验已被 validator 替换
- [x] 所有 handler 的 400 错误响应格式一致
- [x] `go test ./server/...` 通过（预存问题 `TestWorkspaceHandlerCRUD` 除外）
- [x] `api/openapi.yaml` 文件包含所有 87 路由的完整描述
- [x] OpenAPI spec 文件格式正确（通过 yaml.safe_load 验证）
- [x] P2 评估文档完成（`docs/plans/codegen-evaluation.md`）