# Checklist — R9 验收

## Must Pass（对照用户五元素 + 第七类工具）

侧栏对齐
- [ ] 元素1：视图选项按钮可点击，弹出 portal 菜单，不再被裁切
- [ ] 元素2：菜单含「分组方式：按工作区/单列表」「排序方式：最近更新/手动排序」，
      当前项打勾；选择后列表立即变化且刷新页面后保持（localStorage）
- [ ] 元素3：会话树按 workspace 归组：组行有 folder 图标 + chevron + 组名，
      会话行有标题 + 相对时间（如 3小时）；无独立「注册的 workspace」区块
- [ ] 元素4：会话行 ⋯ 菜单含 归档 / Fork / 重命名 / 删除；归档后行灰显且菜单变「取消归档」；
      Fork 产生复制历史的新会话并选中；重命名走行内输入框（无双击 prompt）
- [ ] 元素5：workspace 行右侧 ⋯ 菜单（重命名工作区/删除工作区）＋ ＋ 按钮在该 workspace 新建会话；
      重命名后其下会话仍归在该组（不掉未分组）
- [ ] 单列表模式下平铺全部会话；「未分组」组正常收纳无 workspace 的旧会话

subagent 监控前置（B0）
- [ ] 裸聊天环（server 聊天路径）模型可实际调 spawn_agent 并返回子 agent 最终回复
- [ ] 嵌套 spawn 在 MaxDepth 截断（递归漏洞修复有测试背书）
- [ ] 模型 spawn 后 GET /v1/subagents?session= 出现该记录（running→done 状态流转、耗时正确）
- [ ] CLI 聊天环同样可用（flow 上下文统一安装的回归确认）

第七类工具与待办
- [ ] ＋ 菜单与空态卡片出现第 7 项「任务管理」，打开后显示真实数据（非静态演示）
- [ ] 「子代理」节：模型 spawn 后轮询可见新行，状态点/任务/耗时/结果正确
- [ ] 「运行记录」节：发一条消息产生 run 后出现该 run 行，状态点/时长/错误信息正确；
      进行中的 run 排在前面；点击行展开 llm/tool spans 子行
- [ ] 待办面板：新增 / 删除 / 状态三态循环 / inline 修改标题 均生效且与 /v1/tasks 一致；
      模型 task_add 的条目经刷新可见
- [ ] 待办 tab 名称保持「待办」，不占用「任务管理」名

工程
- [ ] server 模块 go test 全绿（含新增 rename workspace 测试）
- [ ] vite build 无错误；go build 后重启 server，浏览器实测全链路（坑 G5）
- [ ] 提交 commit（前端 + server 分开或合并一个，视改动体量）

## Should Pass
- [ ] 归档/fork/重命名在 authRequired（未带 key）时给出可见错误而非静默失败
- [ ] 视图选项菜单、会话菜单、工作区菜单均支持 Escape / 点外部关闭
- [ ] 折叠 rail 模式下侧栏不回归（拖动/收起后树状态保留）
