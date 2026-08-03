# Checklist

- [ ] `docs/guides/development-workflow.md` 已创建，包含 TDD 流程、命名规范、运行方式
- [ ] `memory/` 模块测试文件已创建，覆盖 Memory/Store/Graph/Bridge 核心路径
- [ ] `skill/` 模块测试文件已创建，覆盖 Skill 解析/序列化/Store 操作
- [ ] `imbridge/` 模块测试文件已创建，覆盖 Bridge 路由/Telegram 配置
- [ ] `desktop/` 模块测试文件已创建，覆盖 DesktopBridge 状态方法
- [ ] `go test ./memory/... ./skill/... ./imbridge/... ./desktop/...` 全部通过
- [ ] GitHub CI 测试通过