# 用源码回溯补全 openhanako 各版本界面设置页面

## Why
本地 `openhanako-history` 各版本只下载了设置骨架文件（SettingsApp.tsx/SettingsNav.tsx），具体 tab 组件（InterfaceTab.tsx 等）全部缺失，导致原型还原靠"猜"。经 v0.421.24 验证，可通过 GitHub raw API 补拉每个版本的完整设置源码，**以源码为准精确还原**。该经验需系统化应用到所有版本。

同时，现有原型为单文件 HTML（45-76KB，500-1000 行），12 个版本重复 90% 公共 CSS/JS，改一次公共样式需改 12 份，操作效率低。需通过共享公共资源重构。

## What Changes
- 新增 `prototypes/openhanako/v0.421.24/` 版本目录，以 v0.442.0 为基础复制并改造
- 将「界面」标签从 3 项扩展为 10 个区块，完整还原真实界面设置
- **系统性源码回溯**：为 openhanako **全部 12 个版本**补拉 `InterfaceTab.tsx` / `SettingsContent.tsx` 及其依赖，以源码为唯一规格来源
- **共享公共资源重构**：抽取所有版本共用的 CSS 与基础 JS 到 `prototypes/openhanako/_shared/`，每个版本 index.html 只保留界面差异与设置模板，改一次公共样式全版本生效
- 主题从下拉 select 改为 3 列网格的 12 个主题卡片（含选中态）
- 新增字体、正文字号、聊天宽度、外观、系统、侧边栏、编辑器、语言和地区、快捷键 9 个区块
- 所有新增控件需具备弱交互能力（卡片点击选中、滑块拖动、开关切换、下拉选择）

## Impact
- Affected specs: ui-prototype-extraction
- Affected code: 新增 `prototypes/openhanako/v0.421.24/index.html`；新增 `prototypes/openhanako/_shared/` 公共资源；重构全部 12 个版本 index.html；回溯 `openhanako-history` 全部版本的设置源码

## What Changes — 共享公共资源架构

**目标**：消除 12 个版本间的重复，让单个版本文件瘦身，改公共样式只改一处。

**目录结构（按组共享）：**
```
prototypes/openhanako/
├── _shared/
│   ├── groupA/           # A 组：v0.36/v0.50/v0.75/v0.150（变量 bg-card/bg-hover/text-light）
│   │   ├── base.css      # 该组公共基础样式
│   │   └── base.js       # 该组公共逻辑
│   ├── groupB/           # B 组：v0.198.4~v0.403（变量 surface/surface-2/muted，view-switcher 顶栏）
│   │   ├── base.css
│   │   └── base.js
│   ├── groupC/           # C 组：v0.421.24/v0.433.1/v0.441.3/v0.442.0（变量 bg2/panel/panel2，proto-bar 顶栏）
│   │   ├── base.css
│   │   └── base.js
│   └── README.md         # 公共资源说明与分组依据
├── v0.36.0/
│   ├── index.html        # 引 <link href="../_shared/groupA/base.css"> + <script src="../_shared/groupA/base.js">
│   └── ...
└── ...
```

**分组依据：** 12 个版本的 CSS 变量命名与布局结构分为 3 组，组内高度一致，组间差异大（见下表），故按组抽取公共资源，避免强行统一的兼容 shim。

| 组 | 版本 | 变量命名 | 顶栏结构 |
|---|---|---|---|
| A | v0.36/v0.50/v0.75/v0.150 | `--bg/--bg-card/--bg-hover/--text-light` | 无 |
| B | v0.198.4/v0.250/v0.300/v0.350.2/v0.403 | `--bg/--surface/--surface-2/--muted` | view-switcher |
| C | v0.421.24/v0.433.1/v0.441.3/v0.442.0 | `--bg/--bg2/--panel/--panel2` | proto-bar |

**拆分原则：**
- 每个 `index.html` 只保留：版本专属的 HTML 结构、版本专属的界面设置模板字符串、版本组内的差异数据
- 每组共用的 CSS 规则 → `_shared/groupX/base.css`
- 每组共用的 JS 逻辑 → `_shared/groupX/base.js`
- 版本差异通过顶部配置对象（`window.VERSION_CONFIG`）声明，base.js 按配置渲染

**file:// 兼容性：** 同目录相对路径的 `<link>`/`<script>` 在 file:// 下可正常加载（仅 fetch/XHR 受限），原型仍可双击打开预览。

**操作效率提升：**
- 改公共样式/逻辑 → 只改 `_shared/` 一处，12 版本同步生效
- 单版本文件从 50-76KB 降至 ~15-25KB
- 新增版本只需写差异部分 + 复用 _shared

## 版本回溯（全版本）

经确认，所有版本均缺失具体 tab 组件，需按组件结构补拉：

| 版本 | 设置组件结构 | 需补拉的核心文件 |
|---|---|---|
| v0.36.0 | 旧式 tab 结构 | `settings/tabs/InterfaceTab.tsx` (6168B) |
| v0.50.0 | 旧式 tab 结构 | `settings/tabs/InterfaceTab.tsx` |
| v0.75.0 | 旧式 tab 结构 | `settings/tabs/InterfaceTab.tsx` |
| v0.150.0 | 旧式 tab 结构 | `settings/tabs/InterfaceTab.tsx` (10938B) |
| v0.198.4 | 旧式 tab 结构 | `settings/tabs/InterfaceTab.tsx` |
| v0.250.0 | 旧式 tab 结构 | `settings/tabs/InterfaceTab.tsx` |
| v0.300.0 | 旧式 tab 结构 | `settings/tabs/InterfaceTab.tsx` |
| v0.350.2 | 旧式 tab 结构 | `settings/tabs/InterfaceTab.tsx` |
| v0.403.0 | SettingsContent 结构 | `settings/tabs/InterfaceTab.tsx` (23410B) |
| v0.421.24 | SettingsContent 结构 | ✅ 已补拉完成 |
| v0.433.1 | SettingsContent 结构 | `settings/SettingsContent.tsx` (12801B) |
| v0.441.3 | SettingsContent 结构 | `settings/SettingsContent.tsx` (12801B) |
| v0.442.0 | SettingsContent 结构 | `settings/SettingsContent.tsx` (12801B) + `tabs/InterfaceTab.tsx` (23410B) |

**回溯的依赖文件（每个版本按需拉取）：**
- `settings/tabs/InterfaceTab.tsx` — 界面设置核心
- `settings/components/SettingsSection.tsx` / `SettingsRow.tsx` / `SettingsGrid.tsx` / `StepSlider.tsx` / `NumberInput.tsx` / `SettingsPrimitives.tsx`
- `shared/theme-registry.ts` + `theme-registry-data.json` — 主题注册表
- `react/utils/font-presets.ts` — 字体预设
- `react/chat/layout.ts` — 聊天布局默认值
- `react/editor/typography.ts` + `shared/editor-typography.ts` — 编辑器排印默认值
- `locales/zh.json` — 中文 i18n 文案

## Source Code Search Clues (from screenshot)

从截图可提取以下源码搜索关键词，用于验证或补充交互细节：

| 截图元素 | 源码关键词/组件 | 已确认存在 |
|---|---|---|
| 界面设置入口 | `InterfaceTab` / `interface` tab | v0.36 SettingsApp.tsx 引用了 InterfaceTab |
| 主题卡片网格 | `ThemeCard` / `OB_THEMES` / `theme-grid` | v0.50 onboarding.js 含主题列表 |
| 衬线/非衬线字体 | `serif` / `setSerifFont` | v0.36 app.js 有 setSerifFont |
| 纸质纹理 | `paperTexture` / `paper-texture` / `rice-paper.png` | v0.403 settings.html 有 paper-texture CSS |
| 会话列表单行 | `rowMode` / `single-line` | v0.403 SessionList.tsx 有 rowMode === 'single-line' |
| 正文字号/聊天宽度 | `fontSize` / `chatWidth` / `maxWidth` | 未确认 |
| 晴天模式 | `sunnyMode` / `sunny` | 未确认 |
| 硬件加速 | `hardwareAcceleration` / `gpuAcceleration` | 未确认 |
| 编辑器字号/行高 | `editorFontSize` / `lineHeight` / `contentPadding` | 未确认 |
| 语言/时区 | `locale` / `timezone` | v0.36 SettingsApp.tsx 有 locale 加载 |
| 快捷键 | `shortcut` / `keybind` / `voiceRecord` | 未确认 |

## Fallback Strategy

**已通过 GitHub raw API 补拉 v0.421.24 全部关键源码，现在以源码为准（源码 > 截图）。**

已下载到 `openhanako-history/v0.421.24/`：
- `settings/tabs/InterfaceTab.tsx` — 界面设置核心组件（23KB）
- `settings/components/SettingsPrimitives.tsx` — SettingsGrid/SettingsSection/SettingsRow 原语
- `settings/Settings.module.css` — 全部样式类（169KB）
- `shared/theme-registry.ts` + `theme-registry-data.json` — 12 主题注册表
- `react/utils/font-presets.ts` — 字体预设
- `react/chat/layout.ts` — 聊天布局默认值
- `react/editor/typography.ts` + `shared/editor-typography.ts` — 编辑器排印默认值
- `locales/zh.json` — 全部中文 i18n 文案（167KB）

**源码确认的精确数据（用于原型还原）：**

| 元素 | 确认数据 |
|---|---|
| 主题卡片 | 11 主题 + 自动 = 12 个，GRID 3 列，经 `theme-registry-data.json` 确认 |
| 主题顺序 | warm-paper→midnight→high-contrast→grass-aroma→contemplation→absolutely→delve→deep-think→new-warm-paper→midnight-contrast→coral→auto |
| 主题中文名 | 暖纸/青夜/素白/草香/沉思/Absolutely/随时准备接住你/用户彻底怒了/新暖纸/青夜·高对比/珊瑚/自动 |
| 主题模式 | 白天/夜间/高对比/Butter/Ming/有一点点熟悉/探究一下/小鲸鱼/纸本/清晰/春日和纸/跟随系统 |
| 字体 | 衬线(适合正文阅读)/非衬线(更接近系统界面)，GRID 2 列 |
| 正文字号滑块 | -2/-1/0/+1/+2，默认 0 |
| 聊天宽度滑块 | 640/720/800/不限制，默认 720 |
| 外观 | 纸质纹理(黑夜模式禁用) + 晴天模式(源码 leavesOverlay，提示"没事儿晒晒太阳") |
| 系统 | 硬件加速(默认开，提示显卡/软件渲染) |
| 侧边栏 | 会话列表单行显示 |
| 编辑器 | 字体(跟随阅读字体/衬线/非衬线)、正文字号16px[12-24]、正文宽度720、一级28px[16-40]、二级21px[15-34]、三级18px[14-30]、行高1.5[1.2-2.2]、内容边距24px[0-64] |
| 语言和地区 | 语言(简体中文/繁體中文/日本語/한국어/English) + 时区(Asia/Shanghai GMT+8) |
| 快捷键 | 语音录制/发送 Ctrl+Shift+M (mac: ⌘⇧M) |

**交互行为（源码确认）：**
- 主题卡片点击 → `window.setTheme(theme)` + 实时切换 + 卡片 active 高亮
- 字体卡片点击 → `window.setSerifFont(serif)` + active 高亮
- 所有编辑器/聊天数值修改 → 即时存配置 + toast "已自动保存"
- 纸质纹理在 midnight/midnight-contrast 主题下禁用（`paperTextureBlockedThemeIds`）
- 快捷键为只读展示（kbd 键帽），不可编辑

**如个别元素截图/源码仍不明确的 fallback：** 以 zh.json i18n 的唯一文案为准，不再猜测。

## ADDED Requirements

### Requirement: 主题卡片网格
界面设置的主题区域 SHALL 使用 3 列网格展示 12 个主题卡片，每个卡片包含主题名和副标题。点击卡片切换选中态（边框高亮），并实时切换全局主题。

#### Scenario: 主题卡片展示与切换
- **WHEN** 用户打开界面设置页面
- **THEN** 主题区域显示 12 个主题卡片（3×4 网格），当前选中主题有粉色边框高亮
- **WHEN** 用户点击任意主题卡片
- **THEN** 该卡片获得选中高亮，全局主题立即切换

### Requirement: 字体选择
界面设置 SHALL 包含字体选择区域，提供衬线/非衬线两个卡片选项，点击切换选中态。

### Requirement: 正文字号与聊天宽度滑块
界面设置 SHALL 包含正文字号滑块（-2/-1/0/+1/+2 五档）和聊天宽度滑块（640/720/800/不限制），滑块拖动时数值实时更新。

### Requirement: 外观区块
界面设置 SHALL 包含外观区块，含纸质纹理和晴天模式两个开关控件。

### Requirement: 系统区块
界面设置 SHALL 包含系统区块，含硬件加速开关控件。

### Requirement: 侧边栏区块
界面设置 SHALL 包含侧边栏区块，含会话列表单行显示开关控件。

### Requirement: 编辑器区块
界面设置 SHALL 包含编辑器区块，含字体下拉、正文字号输入、正文宽度滑块、一/二/三级标题字号输入、行高输入、内容边距输入。

### Requirement: 语言和地区区块
界面设置 SHALL 包含语言和地区区块，含语言下拉和时区下拉。

### Requirement: 快捷键区块
界面设置 SHALL 包含快捷键区块，展示语音录制/发送的快捷键组合（Ctrl+Shift+M）。

## MODIFIED Requirements

### Requirement: 主题控件（原下拉改为卡片网格）
**Reason**: 原版使用 3×4 网格卡片而非下拉 select
**Migration**: 移除原 `<select class="sel" data-theme>` 控件，替换为 12 个 `.theme-card` 卡片网格
