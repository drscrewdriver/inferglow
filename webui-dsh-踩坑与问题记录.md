
## G17 菜单弹层无声失效：setState 更新器里读 e.currentTarget（R9）
- 症状：点击 ⋯/视图选项按钮后弹层不出现，无报错。
- 根因：`setOpenMenu(m => ({ anchor: e.currentTarget }))` —— React 事件派发结束后
  currentTarget 被置空，函数式更新器延迟执行时拿到 null，portal 渲染到 -9999。
- 修复：事件处理器里先 `const el = e.currentTarget` 再进更新器。
- 附带： CU 坐标点击 IAB 时 Playwright role 定位对该按钮超时（svg 命中），用 DOM click 或
  elementFromPoint 诊断后再点。

## G18 同包测试访问 storage.Map 嵌入字段（R9）
- SessionStore 嵌入 `*storage.Map` 且自定义 `Get(id) *T` 单返回值签名遮蔽了泛型 Map 的
  `Get(k) (V, bool)`。要两值形式必须显式 `ss.Map.Get(k)`。

## G19 重启二进制名陷阱（R9 实测）
- 目录里同时存在 inferglow-server.local.exe（旧运行副本）与新编的 inferglow-server.exe；
  按旧命令行重启时 launch 的是旧二进制 → 新路由 404。重启前先 taskkill 精确进程名并确认
  跑的是刚 build 的那个文件。

## G20 TaskStore.List 同秒并列排序 flaky（R9 修复）
- CreatedAt 是 unix 秒：同一秒内的 Add 产生并列，sort.Slice 不稳定 → 顺序随机、测试偶红。
- 修复：CreatedAt 并列时按 ID 决胜。
