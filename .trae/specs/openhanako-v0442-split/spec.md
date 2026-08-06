# openhanako v0.442.0 静态拆分 + 界面差异修正 Spec

## Why
`v0.442.0/index.html`（103KB / ~1432 行）把 CSS、HTML、JS 三种语言混在一个文件里，`settings-content.js`（97KB）把 17 个设置标签模板塞成单个对象。做小调整时定位和改动都费劲。同时 `v0.442.0/界面差异记录.md` 记录了 6 处「界面/分享」标签的样式差异，一直未修正。目标是：把 v0.442.0 拆成按语言 / 按标签模块化的多个文件，并顺手修正记录在案的差异。

## What Changes
- 将 `index.html` 内联 `<style>`（第 24-534 行）抽出为同目录 `index.css`，`<head>` 改为 `<link>` 引用
- 将 `index.html` 运行态 JS（分档滑块补充绑定 + 运行态动态增强）抽出为 `app.js`，在 `base.js` 之后加载
- **保留** `VERSION_CONFIG` 内联（约 15 行，须在 `base.js` 之前声明，抽取收益小、顺序风险高）
- 将 `settings-content.js` 拆为 17 个 `settings-<tab>.js`，每个文件只暴露对应 tab 的模板；`index.html` 用 17 个 `<script src>` 引入（须在 VERSION_CONFIG 声明之前）
- 按 `界面差异记录.md` 修正「界面」标签与「分享」标签的样式差异
- **BREAKING**：`settings-content.js` 文件被删除，由 17 个 `settings-<tab>.js` 取代；`index.html` 的 `<script>` 引用随之更新

## Impact
- Affected specs: none（本 spec 独立）
- Affected code:
  - `prototypes/openhanako/v0.442.0/index.html`（精简为 HTML 骨架 + 少量内联 VERSION_CONFIG）
  - `prototypes/openhanako/v0.442.0/settings-content.js`（删除，拆分为 17 个文件）
  - 新增 `index.css`、`app.js`、`settings-<tab>.js` ×17
  - `prototypes/openhanako/v0.442.0/界面差异记录.md`（更新为已修正）
  - 共享只读，不改 `_shared/groupC/base.css` / `base.js`

## 拆分顺序硬约束（沿用既有约定）
- `settings-*.js`（暴露 `window.__SETTINGS_CONTENT__`）→ `VERSION_CONFIG`（引用该对象）→ `base.js`（读取 `VERSION_CONFIG`）→ `app.js`（DOM 增强）
- 全程 `file://` 可用：只用 `<script src>` / `<link>`，不用 `fetch`

## ADDED Requirements

### Requirement: 内联样式抽出为独立 CSS
系统 SHALL 把 `index.html` 内内联 `<style>` 的全部规则原样迁移到 `index.css`，并以 `<link rel="stylesheet" href="index.css">` 替换原 `<style>` 块。

#### Scenario: 拆分后样式不变
- **WHEN** 浏览器打开拆分后的 `index.html`
- **THEN** 所有视图（主界面 / 聊天 / 设置 / Onboarding）的视觉与拆分前一致，无样式丢失

### Requirement: 运行态 JS 抽出为独立 JS
系统 SHALL 把 `index.html` 内「分档滑块补充绑定」与「运行态动态增强」脚本迁移到 `app.js`，并按 `base.js` 之后加载。

#### Scenario: 拆分后交互不变
- **WHEN** 浏览器打开拆分后的 `index.html` 并操作演示控件（流式回显 / ⌘K 命令面板 / 设置联动等）
- **THEN** 交互行为与拆分前一致，无控制台报错

### Requirement: 设置标签模板按 tab 拆分
系统 SHALL 将 `settings-content.js` 拆为 17 个 `settings-<tab>.js`，每个文件通过 `window.__SETTINGS_CONTENT__ <tab> = \`...\`` 暴露一个 tab 模板；`index.html` 在 VERSION_CONFIG 之前按顺序引入全部 17 个文件。

#### Scenario: 17 标签全部渲染
- **WHEN** 打开设置面板并逐个点击 17 个导航项
- **THEN** 每个 tab 渲染出与拆分前完全一致的内容，无空白、无报错

### Requirement: 界面标签差异修正
系统 SHALL 按 `界面差异记录.md` 修正「界面」标签：
- 字体卡片：改为两行（大字名称 + 小字描述），不再用 `字` 字样 + `·` 连接
- 分档滑块：由 `<button>` 组改为真实 `<input type="range">` + 下方刻度标签，当前值标签加粗；相应移除 `app.js` 中 button 版 `bindSliderButtons`
- **主题卡片颜色**：每张 `theme-card` 按 `界面差异记录.md` 颜色规范表指定独立背景色、名称色、副标题色（写在 `index.css` 的 `[data-theme="..."]` 规则里，不内联）

#### Scenario: 界面标签样式达标
- **WHEN** 打开设置 → 界面标签
- **THEN** 字体卡片两行显示；分档滑块呈现轨道 + 圆点 + 刻度标签，当前值加粗；12 张主题卡片各有独立背景/文字色，与截图一致

### Requirement: 分享标签差异修正
系统 SHALL 按 `界面差异记录.md` 修正「分享」标签：
- 配色卡片：等宽三列、较大圆角（~12px）、选中项蓝色描边
- 宽度卡片：左右布局（左侧排版缩略图 9:16 / 4:3 + 右侧标题/描述），不再用上下纯文字占位
- 截图字体：改为胶囊式右对齐下拉
- 单页字数：输入框右对齐，后跟「字」单位

#### Scenario: 分享标签样式达标
- **WHEN** 打开设置 → 分享标签
- **THEN** 四项差异均修正，与 `界面差异记录.md` 目标样式一致

### Requirement: 差异记录文档同步
系统 SHALL 在修正完成后更新 `界面差异记录.md`，将已修正项标注为「已修正」，并补充完成说明。

#### Scenario: 文档反映真实状态
- **WHEN** 读取 `界面差异记录.md`
- **THEN** 已修正项明确标注，未改项（如主题卡片网格）保留原结论

## MODIFIED Requirements
无（本 spec 为新增能力）。

## REMOVED Requirements
无。