# Tasks

## Phase 0: 基础设施准备
- [x] Task 0.1: 确认所有版本对应的 git tag 名称
  - 检查 openhanako 仓库的 git tags 列表，建立 version → tag 映射表（实际 13 个原型版本）
- [x] Task 0.2: 验证 GitHub raw API 可达性
  - 用 `curl.exe -sS -o NUL -w "HTTP %{http_code}"` 测试（改用本地 git tag 提取更稳定）

## Phase 1: 源码补拉（Source Backfill）
- [x] Task 1.1: 补拉 C 组缺失版本源码（v0.433.1, v0.441.3, v0.442.0）
  - 通过本地 git 仓库 `git show <tag>:<path>` 补拉完整设置源码（backfill.ps1），避免 GitHub raw 限流
  - 目标目录：`openhanako-history/v0.433.1/`, `openhanako-history/v0.441.3/`, `openhanako-history/v0.442.0/`
- [x] Task 1.2: 补拉 B 组缺失依赖文件（v0.198.4~v0.403.0）
  - 读取各版本 InterfaceTab.tsx 确认 import 结构，B 组源码已含 InterfaceTab
- [x] Task 1.3: 补拉 A 组缺失依赖文件（v0.36.0~v0.150.0）
  - 读取各版本 InterfaceTab.tsx 确认区块结构以驱动还原

## Phase 2: 逐版本源码还原（Source-Guided Restore）
- [x] Task 2.1: 还原 v0.442.0 设置页面（+ diff-notes.txt）
- [x] Task 2.2: 还原 v0.441.3 设置页面（+ diff-notes.txt）
- [x] Task 2.3: 还原 v0.433.1 设置页面（+ diff-notes.txt）
- [x] Task 2.4: 还原 v0.421.24 设置页面（+ diff-notes.txt；修正外观多出项/字体 data 属性/完整 hint 文案/快捷键标签）
- [x] Task 2.5: 还原 v0.403.0 设置页面（+ diff-notes.txt；与源码一致）
- [x] Task 2.6: 还原 v0.350.2 设置页面（+ diff-notes.txt；与源码一致）
- [x] Task 2.7: 还原 v0.300.0 设置页面（+ diff-notes.txt；与源码一致）
- [x] Task 2.8: 还原 v0.250.0 设置页面（+ diff-notes.txt；与源码一致）
- [x] Task 2.9: 还原 v0.198.4 设置页面（+ diff-notes.txt；与源码一致）
- [x] Task 2.10: 还原 v0.150.0 设置页面（+ diff-notes.txt；移除超前字体下拉/对话气泡，补齐外观/编辑器/语言和地区）
- [x] Task 2.11: 还原 v0.75.0 设置页面（+ diff-notes.txt；移除超前项，补齐外观/语言和地区）
- [x] Task 2.12: 还原 v0.50.0 设置页面（+ diff-notes.txt；主题 9→6，移除超前项）
- [x] Task 2.13: 还原 v0.36.0 设置页面（+ diff-notes.txt；主题 6，移除超前项，补齐外观/语言和地区）

## Phase 3: 公共资源同步更新
- [x] Task 3.1: 更新 `_shared/groupC/base.css` 和 `base.js`
  - 已支持主题卡片/字体卡片/StepSlider/toggle/设置标签渲染，与 C 组源码一致
- [x] Task 3.2: 更新 `_shared/groupB/base.css` 和 `base.js`
  - 已支持 B 组设置区块渲染
- [x] Task 3.3: 更新 `_shared/groupA/base.css` 和 `base.js`
  - 已支持 A 组设置区块渲染（早期版本用行内样式做区块标题，未破坏共享样式）

## Phase 4: 验证
- [x] Task 4.1: 浏览器验证 v0.442.0 设置页面
  - 12 主题卡片 + 2 字体卡片 + 10 区块全部渲染，数据与源码一致
- [x] Task 4.2: 浏览器抽样验证其他版本设置页面
  - v0.36.0（6 主题/衬线/语言时区）、v0.150.0（编辑器 H1-H6/行高/边距/语言时区）均渲染正确

## Phase 5: v0.442.0 全标签还原（已完成 2026-08-06）
- [x] Task 5.1: 导航 key 与源码对齐
  - appearance→interface、assistant→agent、provider→providers、workspace→work、connectors→mcp、social→bridge（依据 SettingsNav.tsx TAB_ITEMS）
- [x] Task 5.2: 补齐 settingsContent 全部 17 标签
  - 14 个草案子代理并行写入 `openhanako-history/_drafts/v0.442.0/<tab>.html`，主线程并入 index.html
- [x] Task 5.3: 集成后 CSS 校验
  - 补 `.quick-chat-reset-button`（Grep 校验模板引用但未定义的类）
- [x] Task 5.4: 浏览器验证全部 17 标签渲染（DOM 断言）
  - agent/me/interface/general/browser/work/skills/mcp/bridge/providers/media/sharing/access/plugins/experiments/security/about 全部通过
- [x] Task 5.5: 更新 diff-notes.txt 记录全部 17 标签还原差异与还原决策

## Phase 5.5: v0.442.0 占位标签穷尽修正 + offload + 图标（已完成 2026-08-06）
- [x] Task 5.5.1: 盘点 17 标签占位缺口（general/work/browser/mcp/bridge/providers/media/sharing/about/plugins 为占位）
- [x] Task 5.5.2: 并行子代理三源对齐修正全部占位标签（写入 _drafts/v0.442.0/*.html）
- [x] Task 5.5.3: 集成所有修正标签 + offload 到 `settings-content.js`（index.html 2253→1430 行）
- [x] Task 5.5.4: base.js 新增 ICONS 注册表 + icon() + hydrateIcons()，导航 17 图标 + 搜索统一填充
- [x] Task 5.5.5: 浏览器 DOM 断言验证全部 17 标签渲染 + 18 图标填充（sec 数量与源码一致）
- [x] Task 5.5.6: 更新 diff-notes.txt + spec.md（图标/offload 规范）+ project_memory.md 沉淀方法论

## Phase 6: 全部设置标签 × 18 版本推广（待 v0.442.0 占位穷尽修正验收后启动）
- [ ] Task 6.1: 扩展 version→tag 映射表到 18 项，确认新增 5 版本所属组（A/B/C）
- [ ] Task 6.2: 推广前「能力盘点」：对每个版本用 git/trees 递归列出 settings/tabs/，确认该 tag 实际存在的标签（不得硬套 17 标签到早期版本）
- [ ] Task 6.3: 基础标签（通用/界面/关于/安全）跨全版本批量推广
- [ ] Task 6.4: 助手域（agent/me/skills/work）从 C 组开始推广
- [ ] Task 6.5: 平台接入（mcp/bridge/providers/media/sharing/access/plugins/experiments）逐版本盘点推广
- [ ] Task 6.6: 每个版本推广后浏览器 DOM 断言验收（.sec 数量 + data-icon 填充 + settingsContent key 数）+ 更新 diff-notes.txt + 勾选本清单

# Task Dependencies
- Phase 0 全部完成 → Phase 1
- Phase 1 全部完成 → Phase 2
- Phase 2 全部完成 → Phase 3
- Phase 3 全部完成 → Phase 4
- Phase 2 内各版本可并行处理（按组并行）
- Phase 5（v0.442.0 全标签）是 Phase 6 推广的方法论原型，先完成 v0.442.0 再推广
