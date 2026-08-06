# 用源码回溯补全 openhanako 各版本界面设置页面 + 共享公共资源重构

## Phase A: 系统性源码回溯（全部 13 个版本）
- [x] Task A1: 批量补拉 openhanako 旧式 tab 结构版本（v0.36/v0.50/v0.75/v0.150/v0.198.4/v0.250/v0.300/v0.350.2）的 `settings/tabs/InterfaceTab.tsx`，并探测各版本实际引用的组件/依赖
- [x] Task A2: 批量补拉 SettingsContent 结构版本（v0.403/v0.433.1/v0.441.3/v0.442.0）的 `settings/SettingsContent.tsx` + `tabs/InterfaceTab.tsx`
- [x] Task A3: 为每个版本按需拉取依赖：SettingsSection/SettingsRow/SettingsGrid/StepSlider/NumberInput/SettingsPrimitives、theme-registry(+data.json)、font-presets、chat/layout、editor/typography(+shared)、locales/zh.json
- [x] Task A4: 汇总各版本界面设置的元素差异（哪些版本有主题卡片/字体/文字号/宽度/外观/系统/侧边栏/编辑器/语言地区/快捷键），生成差异对照表

## Phase B: 共享公共资源重构（按组共享）
> 13 个版本分 3 组（A: bg-card 变量 / B: surface 变量+view-switcher / C: bg2+panel 变量+proto-bar），组内高度一致、组间差异大，故按组抽取公共资源。
- [x] Task B1: 分析各组代表版本（A:v0.36 / B:v0.403 / C:v0.442）的单文件结构，识别公共 CSS 与公共 JS 边界
- [x] Task B2: 创建 `prototypes/openhanako/_shared/groupA/`（base.css + base.js），抽取 A 组共用样式与逻辑
- [x] Task B3: 创建 `prototypes/openhanako/_shared/groupB/`（base.css + base.js），抽取 B 组共用样式与逻辑
- [x] Task B4: 创建 `prototypes/openhanako/_shared/groupC/`（base.css + base.js），抽取 C 组共用样式与逻辑
- [x] Task B5: 设计 `window.VERSION_CONFIG` 差异配置协议（主题列表、区块有无、界面设置模板）
- [x] Task B6: 每组分内试点 1 个版本重构（A:v0.36 / B:v0.403 / C:v0.442），验证 <link>/<script> 相对引用在 file:// 与 http server 下均正常
- [x] Task B7: 将全部 13 个版本接入所属组的 _shared，每个版本 index.html 瘦身至只保留差异部分
- [x] Task B8: 验证 13 个版本在 http://localhost:8899 下渲染正常、交互可用

## Phase C: 补全界面设置（以源码为准，先在 v0.421.24 实现）
- [x] Task C1: 复制 v0.442.0/index.html 到 v0.421.24/index.html，更新版本号标题和 meta 信息
- [x] Task C2: 替换 v0.421.24 的 appearance 内容模板，将主题从下拉 select 改为 3×4 网格的 12 个主题卡片（暖纸/青夜/素白/草香/沉思/Absolutely/随时准备接住你/用户彻底怒了/新暖纸/青夜·高对比/珊瑚/自动），点击卡片切换选中态并调用 applyTheme
- [x] Task C3: 新增字体区块（衬线/非衬线两卡片选择）
- [x] Task C4: 新增正文字号滑块（-2/-1/0/+1/+2 五档）和聊天宽度滑块（640/720/800/不限制），实现滑块拖动时数值高亮更新
- [x] Task C5: 新增外观区块（纸质纹理开关，黑夜模式禁用 + 晴天模式开关）
- [x] Task C6: 新增系统区块（硬件加速开关）
- [x] Task C7: 新增侧边栏区块（会话列表单行显示开关）
- [x] Task C8: 新增编辑器区块（字体下拉 + 正文字号输入 + 正文宽度滑块 + 一/二/三级标题字号输入 + 行高输入 + 内容边距输入，默认值 16/28/21/18/1.5/24）
- [x] Task C9: 新增语言和地区区块（语言下拉 + 时区下拉）
- [x] Task C10: 新增快捷键区块（语音录制/发送 Ctrl+Shift+M，只读 kbd 键帽）
- [x] Task C11: 为所有新增控件添加弱交互 CSS 样式（卡片选中边框、滑块轨道/拇指、开关、输入框）

## Phase D: 将界面设置应用到其他版本
- [x] Task D1: 基于 Phase A 差异对照表，为每个旧版本（v0.36~v0.403）按各自源码补全/校正「界面」设置区块
- [x] Task D2: 为 v0.433.1/v0.441.3/v0.442.0 补全界面设置区块（若其 InterfaceTab 与 v0.421.24 一致则复用，否则按各自源码）
- [x] Task D3: 为主界面原型加入主题切换联动（主题卡片点击 → 全局 data-theme 切换）

## Phase E: 验证
- [x] Task E1: 浏览器验证 v0.421.24：点击「界面」标签，确认 10 个区块完整展示且交互正常
- [x] Task E2: 浏览器抽验其他版本（至少 v0.403.0 与 v0.442.0），确认界面设置区块与源码一致
- [x] Task E3: 验证 _shared 重构后全部版本公共功能（视图切换/弱交互/主题切换）正常

# Task Dependencies
- Phase A 是数据基础，需先完成
- Phase B 与 Phase A 可部分并行，但 B6（全版本接入）依赖 A4 差异表
- Phase C 依赖 A 的 v0.421.24 源码（已完成）+ B 的公共资源
- Phase D 依赖 A4 差异表 + C 的还原模式 + B 的公共资源
- Phase E 依赖 B/C/D 完成