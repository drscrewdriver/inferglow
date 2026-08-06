# Tasks

> change-id: openhanako-v0442-split
> 目标：静态拆分 v0.442.0（CSS / 运行态 JS / 设置标签逐 tab）+ 按 `界面差异记录.md` 修正「界面」「分享」标签差异。

## Task 1: 抽出内联样式 → index.css
- [x] 读取 `index.html` 第 24-534 行 `<style>` 内容，原样迁移到新文件 `index.css`
- [x] 移除 `index.html` 内联 `<style>` 块，在 `<head>` 加 `<link rel="stylesheet" href="index.css">`（放在 `_shared/groupC/base.css` 之后）
- [x] 浏览器打开验证：主界面 / 聊天 / 设置 / Onboarding 视觉与拆分前一致

## Task 2: 抽出运行态 JS → app.js
- [x] 将 `index.html` 内「分档滑块补充绑定」（约 1010-1031 行）与「运行态动态增强」（约 1033-1432 行）两个 IIFE 迁移到新文件 `app.js`
- [x] 在 `index.html` 中，`<script src="../_shared/groupC/base.js">` **之后**加 `<script src="app.js"></script>`
- [x] 保留 `VERSION_CONFIG` 内联（须在 base.js 之前声明）
- [x] 浏览器验证：流式回显 / ⌘K / 设置联动等交互正常，无 console 报错

## Task 3: 拆分 settings-content.js 为逐 tab 文件
- [x] 按 17 个 tab（general/interface/agent/providers/me/browser/work/skills/mcp/bridge/media/sharing/access/plugins/experiments/security/about）各生成 `settings-<tab>.js`
- [x] 每个文件用 IIFE 暴露：`window.__SETTINGS_CONTENT__ = window.__SETTINGS_CONTENT__ || {}; window.__SETTINGS_CONTENT__.<tab> = \`...\`;`
- [x] `index.html` 在 VERSION_CONFIG 之前按顺序引入 17 个 `<script src="settings-<tab>.js">`
- [x] 删除原 `settings-content.js`
- [x] 浏览器验证：17 个设置标签逐个点击均渲染，无空白、无报错

## Task 4: 修正「界面」标签差异
> 依赖 Task 3（在 `settings-interface.js` 上改，CSS 进 `index.css`）
- [x] 字体卡片：改为两行（`font-card-name` 大字 + `font-card-desc` 小字），去掉 `字` 字样与 `·` 连接
- [x] 分档滑块（正文字号 / 聊天宽度 / 编辑器正文宽度）：改为 `<input type="range">` + 下方刻度标签，当前值加粗（复用 base.js 的 `bindStepSliders`）
- [x] 移除 `app.js` 中 button 版 `bindSliderButtons` IIFE（不再需要）
- [x] 主题卡片颜色：按 `界面差异记录.md` 颜色规范表，在 `index.css` 中为每张 `theme-card[data-theme="..."]` 添加独立背景色/名称色/副标题色规则（共 12 张卡片 + 自动卡片分色）
- [x] 浏览器验证：12 张主题卡片各有独立颜色，与截图一致

## Task 5: 修正「分享」标签差异
> 依赖 Task 3（在 `settings-sharing.js` 上改，CSS 进 `index.css`）
- [x] 配色卡片：等宽三列、圆角 ~12px、选中项蓝色描边
- [x] 宽度卡片：左右布局（左侧排版缩略图 9:16 / 4:3 + 右侧标题/描述）
- [x] 截图字体：胶囊式右对齐下拉
- [x] 单页字数：输入框右对齐 + 后跟「字」单位
- [x] 浏览器验证：分享标签四项差异均修正

## Task 6: 更新界面差异记录.md
> 依赖 Task 4、5
- [x] 将「界面」标签的字体卡片、分档滑块两项与「分享」标签四项标注为「已修正」
- [x] 保留「无需改动」项（主题卡片网格等）原结论
- [x] 补充各修正项的完成说明与目标样式达成情况

## Task 7: 整体验收
> 依赖 Task 1-6
- [x] 浏览器全量验证：4 个视图可切换、17 个设置标签全渲染、无 console 错误
- [x] 核对 `checklist.md` 全部通过

# Task Dependencies
- [Task 4] depends on [Task 3]
- [Task 5] depends on [Task 3]
- [Task 6] depends on [Task 4]、[Task 5]
- [Task 7] depends on [Task 1]-[Task 6]
- Task 1、2、3 相互独立，可并行