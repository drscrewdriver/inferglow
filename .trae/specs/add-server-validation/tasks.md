# Tasks

- [x] Task 1: 引入 go-playground/validator 依赖并初始化
  - [x] 在 `server/` 模块添加 `github.com/go-playground/validator/v10` 依赖
  - [x] 在 `server/` 包中创建全局 `validate` 实例（`var validate = validator.New()`）
  - [x] 编写单元测试验证 validator 实例初始化正确

- [x] Task 2: 给核心请求结构体添加 validate tag
  - [x] `AgentConfig` — `Name` required, `Model` required
  - [x] `ChatRequest` — `Message` required
  - [x] `MemoryRecord` — 保留自定义 if-else（`Content`/`Facts` 是"或"关系）
  - [x] 匿名结构体（`handleCreateSession`、`handleCreateKnowledgeBase`、`handleSearchKnowledgeBase`、`handleMemorySemanticSearch`、`handleCreateCredential`、`handleCreateRun`）— 添加 required tag
  - [x] `TeamConfig` — `Name` required

- [x] Task 3: 替换 handler 层的手动 if-else 校验
  - [x] `handleCreateAgent` — 替换 `if cfg.Name == ""` 为 `validate.Struct(cfg)`
  - [x] `handleChat` / `handleStream` / `handleInput` — 替换 `if req.Message == ""`
  - [x] `handleCreateMemory` — 保留自定义 if-else（`Content || Facts` 无法用 tag 表达）
  - [x] `handleCreateCredential` — 添加 `validate.Struct(req)` 检查
  - [x] `handleCreateRun` — 替换 `if req.Flow == ""`
  - [x] `handleSearchKnowledgeBase` / `handleMemorySemanticSearch` — 替换 `if req.Query == ""`

- [x] Task 4: 验证所有测试通过
  - [x] 运行 `go test ./server/...` — 除预存问题（`TestWorkspaceHandlerCRUD` JSON 转义）外全部通过
  - [x] 确认错误消息格式一致（validator 错误 → 400 JSON 响应）

- [x] Task 5: 编写 OpenAPI 3.0 spec 文件 `api/openapi.yaml`
  - [x] 覆盖 server 模块所有 87 路由
  - [x] 包含完整 request body schema、response schema、path parameters
  - [x] 通过 `yaml.safe_load` 验证格式正确

- [x] Task 6: 评估代码生成方案（P2，调研不落地）
  - [x] 调研 kin-openapi 在 inferglow 场景下的适用性和成本
  - [x] 调研 ogen 的代码生成方案和侵入性
  - [x] 调研 oapi-codegen 的方案和框架兼容性
  - [x] 输出评估文档 `docs/plans/codegen-evaluation.md`

# Task Dependencies
- Task 1 无依赖
- Task 2 依赖 Task 1
- Task 3 依赖 Task 2
- Task 4 依赖 Task 3
- Task 5 无依赖（可并行于 Tasks 1-4）
- Task 6 无依赖（可并行于 Tasks 1-5）