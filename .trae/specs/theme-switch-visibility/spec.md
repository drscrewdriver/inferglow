# 增强 openhanako 主题切换视觉效果（全版本）

## Why
openhanako 原型目前的主题切换存在两处失真与不足：

1. **配色不完整导致效果不明显**：C 组版本（v0.421.24/v0.433.1/v0.441.3/v0.442.0）的「界面」设置展示了 12 张主题卡片，但 `:root[data-theme=...]` 只定义了 **3 套**颜色（dark/light/midnight），其余 9 个主题点击后界面几乎不变。
2. **多数版本原型失真为 3 套下拉**：真实源码调查显示，从 **v0.36.0 起**界面设置就是多主题 **theme-card 卡片网格**（非下拉），且主题数量随版本演进：v0.36=6 主题、v0.50~v0.75=9 主题、v0.150~v0.403=theme-registry 动态列表、v0.421.24~v0.442=12 主题。但当前原型（A/B 组）全被简化成「下拉 + 仅 dark/light/midnight 三套」，丢失了真实多主题。

## What Changes
- **全版本回溯确认主题机制**：以真实源码为准，确认每个版本应有几个主题、主题 UI 是卡片还是下拉、主题切换入口（界面设置/onboarding/主题切换条）。
- **为每个版本补齐应有主题**：按真实源码主题数量，为每个版本实现对应主题的切换（含 `.theme-card` 卡片网格还原、`:root[data-theme=...]` 配色还原、主题切换设置入口还原）。
- **补齐 `:root[data-theme=...]` 配色**：为每个版本的所有主题提供颜色变量覆盖，使切换产生明显、可感知的视觉变化。
- **还原 CSS 效果**：主题色取值以真实源码 `theme-registry`（主题 `backgroundColor`）+ `themes/*.css` 语义为准；源码缺失的主题从最近似主题推导一套可感知配色。
- 主题卡片点击保持选中态 `.active` 高亮与全局配色同步（`bindThemeCards` 已实现，复核即可）。

## 真实源码主题机制调查结论（已确认）

| 版本 | 真实主题UI形式 | 真实主题数量 | 主题id | 当前原型 |
|---|---|---|---|---|
| v0.36.0 | theme-card 卡片 | 6 | warm-paper/midnight/high-contrast/grass-aroma/contemplation/auto | 失真(下拉3套) |
| v0.50.0 | theme-card 卡片 | 9 | +absolutely/delve/deep-think | 失真(下拉3套) |
| v0.75.0 | theme-card 卡片 | 9 | 同 v0.50 | 失真(下拉3套) |
| v0.150.0 | registry.THEMES 卡片 | 动态(~11+auto) | registry 驱动 | 失真(下拉3套) |
| v0.198.4 | theme-card 卡片 | registry 驱动 | 同 | 失真(下拉3套) |
| v0.250.0 | theme-card 卡片 | registry 驱动 | 同 | 失真(下拉3套) |
| v0.300.0 | theme-card 卡片 | registry 驱动 | 同 | 失真(下拉3套) |
| v0.350.2 | theme-card 卡片 | registry 驱动 | 同 | 失真(下拉3套) |
| v0.403.0 | theme-card 卡片 | registry 驱动 | 同 | 失真(下拉3套) |
| v0.421.24 | theme-card 卡片 | 12 | 完整 theme-registry | 卡片但配色不全(3套) |
| v0.433.1 | theme-card 卡片 | 12 | 完整 | 卡片但配色不全(3套) |
| v0.441.3 | theme-card 卡片 | 12 | 完整 | 卡片但配色不全(3套) |
| v0.442.0 | theme-card 卡片 | 12 | 完整 | 卡片但配色不全(3套) |

**结论**：所有 13 个版本真实源码**都是多主题卡片网格**，无一用下拉。当前 A/B 组原型失真为下拉3套，C 组原型虽有多卡片但配色不全。**全部 13 个版本都需要按真实源码补足主题切换。**

## 主题配色参考（theme-registry-data.json 的 backgroundColor）

| 主题id | 中文名 | backgroundColor |
|---|---|---|
| warm-paper | 暖纸 | #F8F4ED |
| midnight | 青夜 | #3B4A54 |
| high-contrast | 素白 | #FAF8F7 |
| grass-aroma | 草香 | #F5F8F3 |
| contemplation | 沉思 | #F3F5F7 |
| absolutely | Absolutely | #F4F3EE |
| delve | 随时准备接住你 | #FFFFFF |
| deep-think | 用户彻底怒了 | #FCFCFD |
| new-warm-paper | 新暖纸 | #F5EFE4 |
| midnight-contrast | 青夜·高对比 | #26343D |
| coral | 珊瑚 | #FDF6EC |
| auto | 自动 | 跟随系统 |

## Impact
- Affected specs: ui-prototype-extraction（主题机制还原子任务）
- Affected code: 全部 13 个版本 `prototypes/openhanako/v0.*/index.html` 的 `:root` 主题变量块与主题卡片/切换入口
- 各版本已接入 `_shared/groupA|B|C/base.css`，`:root` 主题变量块保留在版本内（不抽公共），故主题色直接在各版本 index.html 的 `<style>` 中补齐

## ADDED Requirements

### Requirement: 全版本多主题切换
每个版本的「主题」切换 SHALL 按该版本真实源码的主题数量，提供对应数量的主题卡片切换，且每个主题都有可感知的 `:root[data-theme=...]` 配色。

#### Scenario: v0.36.0 六主题切换
- **WHEN** 用户在 v0.36.0 界面设置查看主题区域
- **THEN** 显示 6 个主题卡片（暖纸/青夜/素白/草香/沉思/自动），点击每个卡片界面颜色明显变化且 `.active` 高亮同步

#### Scenario: v0.50/v0.75 九主题切换
- **WHEN** 用户在 v0.50.0 或 v0.75.0 查看主题区域
- **THEN** 显示 9 个主题卡片（在 v0.36 基础上 +Absolutely/delve/deep-think），点击切换配色明显

#### Scenario: C 组十二主题切换
- **WHEN** 用户在 v0.421.24/v0.433.1/v0.441.3/v0.442.0 查看主题区域
- **THEN** 显示 12 个主题卡片，点击每个主题界面颜色明显变化，不再"点了没反应"

### Requirement: 主题切换入口还原
各版本的主题切换入口 SHALL 按真实源码还原（界面设置 tab 的 theme-card 网格；若真实源码在 onboarding 或主题切换条也有入口，则同步还原）。

### Requirement: 配色以真实源码为准
主题色 SHALL 优先取自真实源码 `theme-registry` 的 `backgroundColor` 与 `themes/*.css` 语义；源码缺失的主题 SHALL 从最近似主题推导，并保证可感知。

## MODIFIED Requirements

### Requirement: 主题卡片交互（原 v0.421.24 已实现）
**Reason**: 交互已存在，但配色不完整导致效果不明显
**Migration**: 保持 `bindThemeCards` 逻辑不变，仅补齐 `:root[data-theme=...]` 颜色变量