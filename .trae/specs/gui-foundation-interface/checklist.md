# Checklist

## 基座计划（后端实现）
- [x] 会话支持分组/置顶/重命名/归档字段，`PATCH /v1/sessions/{id}` 可更新
- [x] 会话列表按分组返回、置顶项排前
- [x] `GET /v1/sessions/{id}/messages?before=&limit=` 分页拉取历史消息
- [x] `POST /v1/runs/{id}/input` 可承载审批决策（允许/拒绝）并回填状态
- [x] 用量聚合端点返回跨会话成本/缓存/Token 统计
- [x] `desktop/main.go` 存在，`wails build`（`go build -tags desktop`）通过
- [x] desktop `StartSession/SendChat` 代理到 server REST 并返回真实回复
- [x] 各基座改动均有单元测试覆盖

## 界面计划（HTML 静态原型）
- [x] `prototypes/` 下存在可交互的 InferGlow GUI 静态原型目录（`inferglow-gui/`）
- [x] 聊天主界面可输入、可模拟流式回显、工具卡片可展开
- [x] 会话管理 + 上下文环（SVG）+ 设置面板标签联动可用
- [x] 演示跳转条带虚线边框 + 珊瑚色「原型演示」标注，与产品语言分离
- [x] 弱交互完善（输入聚焦/复制反馈/列表选中态/弹层关闭）
- [x] 浏览器验证通过，预览可交付
- [ ] Composer 从纯 textarea 升级为富输入容器，拖入文件/产物自动生成 `` `dir`` 标记
- [ ] UI 渲染为带 ✕ 可删除标签 chip，与普通文本输入视觉区分
- [ ] 点击 ✕ 删除 chip 并清理底层 `` `dir` `` 结构