# 还原原型运行态动态增强 Spec

## Why

`inferglow-gui/` 原型已具备「运行态动态增强」（流式逐字回显、工具卡片动态注入、会话实时搜索、⌘K 命令面板、目标模式、终端抽屉）。而 `prototypes/` 下已还原的 openhanako（12 版）与 reasonix v1.19.7 原型目前只有**弱交互**（设置标签联动、主题、onboarding、复制/折叠），缺少**运行态动态增强**——消息是一次性静态显示、工具卡不可动态注入、会话无实时搜索、无命令面板/目标/终端运行态。

目标：把「运行态动态增强」能力移植到这些还原原型，前提是**每个版本按该版本真实支持的能力定制**（openhanako 有流式 token/工具卡，Reasonix 有工具卡/Goal），不臆造、不照搬 inferglow 的整套 UI。

## What Changes

- 为 openhanako 全部 12 版（v0.36.0 … v0.442.0）与 reasonix v1.19.7 的 `index.html` 增加版本内运行态 JS 增强。
- 增强项（按版本真实能力裁剪）：
  - 流式逐字回显：发送后 `setInterval` 模拟打字。
  - 工具卡片动态注入：tool_start/tool_end 四态（运行✓✗⚠）+ 沙箱/副作用徽章，点击展开输出。
  - 会话实时搜索：会话列表输入即过滤，选中态联动主区。
  - ⌘K 命令面板 / 终端抽屉命令回填 / Goal 目标模式切换（按版本能力）。
  - 设置界面齿轮入口的上层联动（打开设置弹层、与主界面状态联动）。
- 增强遵循 `prototypes/原型交互与还原原则.md`：只还原真实能力、演示控件用虚线边框+珊瑚色标注「原型演示」、弱交互完善。
- 不改动每个版本既有的静态结构/主题变量/共享 base.js（openhanako 三组共享资源保持原样），只在各版本 `index.html` 尾部追加一段版本内 `<script>`。

## Impact
- Affected files: `prototypes/openhanako/v{0.36.0,0.50.0,0.75.0,0.150.0,0.198.4,0.250.0,0.300.0,0.350.2,0.403.0,0.421.24,0.433.1,0.441.3,0.442.0}/index.html`、`prototypes/reasonix/v1.19.7/index.html`。
- 不新增共享文件；不新建后端；不新增 .md 文档。
- 只读共享资源，不修改 `_shared/*`。

## ADDED Requirements

### Requirement: 运行态动态增强（openhanako 全版本 + reasonix v1.19.7）
每个目标版本 SHALL 在其 `index.html` 内追加运行态增强 JS，能力按该版本真实支持裁剪。

#### Scenario: 聊天流式回显
- **WHEN** 用户在输入框发送消息
- **THEN** 助手回复以 `setInterval` 逐字流入，期间发送按钮变为「停止」，可中断

#### Scenario: 工具卡片动态注入
- **WHEN** 模拟 `tool_start`/`tool_end` 事件
- **THEN** 消息流动态插入工具卡片，呈现四态（运行/✓/✗/⚠）+ 徽章，点击可展开输出

#### Scenario: 会话实时搜索
- **WHEN** 在会话列表搜索框输入关键字
- **THEN** 会话列表实时过滤，命中项可选中并联动主区

#### Scenario: 命令面板 / 终端 / 目标（按版本能力）
- **WHEN** 触发 ⌘K 或对应按钮
- **THEN** 命令面板可搜索/分组/关闭；终端抽屉可展开并命令回填；Goal 模式可切换（蓝色主题区分）

#### Scenario: 设置齿轮入口上层联动
- **WHEN** 点击设置齿轮入口
- **THEN** 打开设置弹层并联动主界面状态；关闭后状态保留

#### Scenario: 演示标记
- **WHEN** 增强涉及原型演示控件
- **THEN** 带虚线边框 + 珊瑚色「原型演示」标注，与产品语言分离

## MODIFIED Requirements
（无）

## REMOVED Requirements
（无）