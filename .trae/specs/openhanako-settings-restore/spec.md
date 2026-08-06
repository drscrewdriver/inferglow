# openhanako 各版本界面设置源码还原方法论

## Why

当前 `prototypes/openhanako/` 下 14 个版本的设置页面是"猜出来"的，没有以源码为唯一规格来源。用户明确要求：**不要直接照着截图改，要对应相应版本的界面源码来恢复**。

经排查：
- `openhanako-history` 中 **仅 v0.421.24 拥有完整源码**（InterfaceTab.tsx + SettingsContent.tsx + 全部依赖文件）
- v0.36.0~v0.403.0 只有旧式 `InterfaceTab.tsx`，缺少 SettingsContent/Primitives/theme-registry/zh.json 等依赖
- v0.433.1 / v0.441.3 / v0.442.0 在 history 中**完全没有源码文件**

因此需要一套系统化的方法论：**以源码为准 → 按版本演进还原 → 差异记录入 txt**。

## What Changes

- 建立「源码回溯 → 原型还原 → 差异记录」三步方法论
- 为每个版本从 GitHub raw API 补拉缺失的源码文件到 `openhanako-history/`
- 以源码中的 InterfaceTab.tsx + 依赖文件为唯一规格，还原各版本原型设置页面
- 每个版本目录下生成 `diff-notes.txt` 记录还原过程中的源码与原型差异
- 按版本演进分组处理（A/B/C 三组），同组版本共享公共还原逻辑

## Impact

- Affected specs: ui-prototype-extraction（本 spec 是其方法论升级）
- Affected code:
  - `openhanako-history/` 各版本补拉源码
  - `prototypes/openhanako/` 各版本 index.html 设置模板
  - `prototypes/openhanako/_shared/` 公共资源
  - 各版本目录下新增 `diff-notes.txt`

## 方法论：三步流程

### Step 1: 源码回溯（Source Backfill）

**目标**：确保每个版本在 `openhanako-history/` 下拥有完整的设置相关源码。

**补拉文件清单**（按版本组件结构区分）：

| 版本组 | 组件结构 | 需补拉文件 |
|---|---|---|
| A 组 (v0.36~v0.150) | 旧式 tab | `InterfaceTab.tsx` + 该版本 SettingsApp.tsx 中 import 的依赖 |
| B 组 (v0.198.4~v0.403) | 旧式 tab → 过渡 | `InterfaceTab.tsx` + 该版本 SettingsApp.tsx 中 import 的依赖 |
| C 组 (v0.421.24~v0.442.0) | SettingsContent 结构 | `InterfaceTab.tsx` + `SettingsContent.tsx` + `SettingsPrimitives.tsx` + `Settings.module.css` + `theme-registry.ts` + `theme-registry-data.json` + `font-presets.ts` + `layout.ts` + `typography.ts` + `editor-typography.ts` + `zh.json` + `appearance-preferences.ts` |

**补拉方式**：通过 GitHub raw API (`raw.githubusercontent.com/drscrewdriver/Agently/<tag>/desktop/src/<path>`) 下载，使用 `curl.exe`（不用 `Invoke-WebRequest`，见 project_memory 教训）。

**版本 tag 映射**：需确认每个版本对应的 git tag（如 `v0.442.0` → tag `v0.442.0`）。

### Step 2: 源码还原（Source-Guided Restore）

**核心原则**：源码 > 截图 > 猜测。

**还原流程**：
1. 读取该版本的 `InterfaceTab.tsx`，提取：
   - 区块列表（哪些设置区块存在）
   - 控件类型（卡片/滑块/开关/下拉/输入框）
   - 数据源（主题列表/字体列表/默认值）
2. 读取依赖文件确认精确数据：
   - `theme-registry-data.json` → 主题顺序、名称、副标题、模式
   - `zh.json` → 中文 i18n 文案
   - `font-presets.ts` → 字体选项
   - `layout.ts` → 聊天宽度默认值
   - `editor-typography.ts` → 编辑器默认值
3. 对比该版本原型 `index.html` 的设置模板，按源码修正
4. 只还原该版本**源码中实际存在**的区块和控件，不超前添加后续版本才有的功能

**版本演进注意事项**：
- 早期版本（A 组）可能只有主题+字体，没有编辑器/快捷键等区块
- 中期版本（B 组）逐步增加外观/系统/侧边栏
- 晚期版本（C 组）才有完整的 10 区块
- **每个版本只还原它那个时代真实有的控件**

### Step 3: 差异记录（Diff Notes）

**目标**：每个版本目录下生成 `diff-notes.txt`，记录还原过程中的关键发现。

**记录格式**：
```
# [版本号] 设置还原差异记录
生成时间: YYYY-MM-DD
源码版本: openhanako-history/vX.Y.Z/

## 源码确认的区块列表
- [区块名]: [控件类型] - [关键数据]

## 与原型(index.html)的差异
- [差异描述]: 原型有 X，源码实际是 Y

## 缺失/无法确认的内容
- [说明哪些内容源码中没有或无法确认]

## 还原决策
- [记录关键还原决策及依据]
```

## ADDED Requirements

### Requirement: 源码补拉完整性
每个版本 SHALL 在 `openhanako-history/` 下拥有该版本 git tag 对应的完整设置源码文件。补拉使用 `curl.exe`，失败时记录到 diff-notes.txt。

### Requirement: 源码驱动还原
每个版本原型的设置页面 SHALL 仅包含该版本源码中实际存在的区块和控件。禁止将后续版本的功能超前添加到早期版本。

### Requirement: 差异记录
每个版本目录下 SHALL 存在 `diff-notes.txt`，记录源码与原型之间的差异、缺失内容和还原决策。

### Requirement: 版本演进忠实性
还原 SHALL 遵循版本演进顺序：A 组（基础）→ B 组（扩展）→ C 组（完整）。每个版本只拥有它那个时代真实存在的设置项。

## MODIFIED Requirements

### Requirement: 原型还原规格来源
**原**: 以截图 + 猜测还原
**新**: 以 `openhanako-history/` 下对应版本的源码为唯一规格来源，截图仅作辅助验证

## 推广阶段：从「界面标签」到「全部设置标签」× 18 版本（2026-08-06 演进）

### 现状
- v0.442.0 已完整还原全部 17 个设置标签（agent/me/interface/general/browser/work/skills/mcp/bridge/providers/media/sharing/access/plugins/experiments/security/about），导航 key 与源码 TAB_ITEMS 严格对齐，浏览器验证通过。
- 其余版本（v0.421.24 及更早）目前只还原了 InterfaceTab（界面）。

### 推广目标
将 v0.442.0 的「全标签还原」方法论推广到其他版本，并扩展到 18 个版本。

### 推广前必做的「能力盘点」（每个版本）
回溯任何版本的其他标签前，必须先确认该版本源码究竟有哪些标签。依据：
- `SettingsContent.tsx` 的 `TAB_COMPONENTS`（id→组件映射，唯一路由源）
- `SettingsNav.tsx` 的 `TAB_ITEMS`（导航 id 数组）
- 用 `git/trees/<tag>?recursive=1` 递归列出该 tag 的 `settings/tabs/` 目录，确认实际存在的 tab 组件

**关键约束**：早期版本（A/B 组）可能根本没有 SettingsContent 结构，也没有后来新增的标签（mcp/bridge/providers/media/plugins/experiments 等）。**不能把 v0.442.0 的 17 标签硬套到早期版本**，必须按各 tag 真实源码盘点。

### 推广批次策略（按标签类型分组跨版本）
为避免 18×17 = 306 组合逐一重复劳动，按「标签类型」分组、跨版本批量推广：
1. **基础标签**（通用/界面/关于/安全）：几乎全版本都有，先推广
2. **助手域**（agent/me/skills/work）：从有 SettingsContent 结构的版本（C 组）开始
3. **平台接入**（mcp/bridge/providers/media/sharing/access/plugins/experiments）：仅 C 组晚期版本有，逐版本盘点再推广

### 推广验收
每个版本推广完成后，用浏览器 DOM 断言（非截图）验证该版本实际拥有的标签都能渲染出对应内容，并把差异记录进该版本 `diff-notes.txt`、勾选 `tasks.md`。

### 18 版本清单（目标）
当前 `prototypes/openhanako/` 已有 13 个版本；推广到 18 个需先补齐 5 个缺失版本（确认 git tag 后补拉源码 + 建原型目录）：需在 Phase 0 中把 version→tag 映射表扩展到 18 项，并确认新增版本所属组（A/B/C）以复用对应 base.css/base.js。

## 「举一反三」穷尽原则（2026-08-06 修订）

**不得把一个标签的三源对齐经验只用于一个标签就宣称完成。** 每个标签都要独立做「组件结构 + i18n + CSS 类名」三源对齐，且要穷尽到该版本全部标签。v0.442.0 复盘发现多个标签是占位（general/work/browser/mcp/bridge/providers/media/sharing/about/plugins 等），仅凭 interface/agent 对齐就宣称「17 标签完成」是错误的。正确流程：**每个标签都回溯其源码组件树 + zh.json 对应 key，逐区块/逐行核对，占位即修正。**

### 图标一致化规范（新增）
- 共享 `_shared/<组>/base.js` 提供 `ICONS` 注册表 + `icon(name)` + `hydrateIcons(root)`（统一 stroke-width=1.75 / fill=none / stroke=currentColor）。
- 静态图标用 `<svg data-icon="<name>" class="<尺寸类>"></svg>` 占位，由 `hydrateIcons(document)` 在 init 时填充。
- 禁止各版本内联 SVG 各自写 stroke-width（1.5/1.8/2 混用），一律走注册表。
- C 组 ICONS 含 17 个导航图标 + search；A/B 组按所需图标建立对应注册表。

### offload 规范（新增）
- settings 标签模板不内联在 index.html，放到独立 `settings-content.js`（IIFE 暴露 `window.__SETTINGS_CONTENT__`），index.html 只保留主框架。
- **加载顺序硬约束**：`<script src="settings-content.js">` 必须在 `<script>` 声明 `VERSION_CONFIG`（其内 `settingsContent: window.__SETTINGS_CONTENT__`）**之前**加载。JS 对象字面量引用是立即求值的，settings-content.js 若在其后加载，`settingsContent` 会捕获 undefined。
- 用 `<script src>` 而非 fetch：静态原型以 `file://` 打开时 fetch 被 CORS 拦截，script 标签不受限制。
- 各版本 settings-content.js 的 key 列表由该版本源码 `TAB_COMPONENTS` 决定（不用 v0.442.0 硬套）。

### 推广验收（更新）
- 三源对齐 + 图标统一（data-icon 填充）+ offload（settings-content.js）三者在每个版本都要验证通过才算完成。
- 用浏览器 DOM 断言：检查每个标签 `.sec` 数量、`data-icon` 填充数量、`VERSION_CONFIG.settingsContent` 的 key 数。
