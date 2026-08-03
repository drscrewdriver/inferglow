# Checklist

- [x] `docs/guides/development-workflow.md` 已创建，包含 TDD 流程、命名规范、运行方式
- [x] `memory/` 模块测试文件已创建，覆盖 Memory/Store/Graph/Bridge 核心路径（17 测试）
- [x] `skill/` 模块测试文件已创建，覆盖 Skill 解析/序列化/Store 操作（20 测试）
- [x] `imbridge/` 模块测试文件已创建，覆盖 Bridge 路由/Telegram 配置（8 测试）
- [x] `desktop/` 模块测试文件已创建，覆盖 DesktopBridge 状态方法（3 测试）
- [x] `go test ./memory/... ./skill/... ./imbridge/... ./desktop/...` 全部通过
- [ ] GitHub CI 测试通过（等待 CI 完成）