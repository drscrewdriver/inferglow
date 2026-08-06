# Checklist — 增强 openhanako 主题切换视觉效果（全版本）

## 源码回溯（Task 1）
- [x] 已确认 13 个版本真实源码均用 theme-card 卡片（非下拉）
- [x] 已确认各版本真实主题数量（v0.36=6 / v0.50~v0.75=9 / registry动态 / C组=12）
- [x] 已提取 12 主题配色参考（backgroundColor）
- [x] 已确认 registry 动态版本（v0.150~v0.403）实际展开的主题 id 列表

## C 组配色补齐（Task 2）
- [x] v0.421.24/index.html 的 `:root[data-theme=...]` 已补齐 12 套配色
- [x] v0.433.1/index.html 的 `:root[data-theme=...]` 已补齐 12 套配色
- [x] v0.441.3/index.html 的 `:root[data-theme=...]` 已补齐 12 套配色
- [x] v0.442.0/index.html 的 `:root[data-theme=...]` 已补齐 12 套配色

## A 组多主题还原（Task 3）
- [x] v0.36.0 主题改为 6 张主题卡片，配色与切换设置生效
- [x] v0.50.0 主题改为 9 张主题卡片，配色与切换设置生效
- [x] v0.75.0 主题改为 9 张主题卡片，配色与切换设置生效
- [x] v0.150.0 主题改为 theme-card 网格多主题，配色与切换设置生效

## B 组多主题还原（Task 4）
- [x] v0.198.4 主题改为 theme-card 网格多主题，配色与切换设置生效
- [x] v0.250.0 主题改为 theme-card 网格多主题，配色与切换设置生效
- [x] v0.300.0 主题改为 theme-card 网格多主题，配色与切换设置生效
- [x] v0.350.2 主题改为 theme-card 网格多主题，配色与切换设置生效
- [x] v0.403.0 主题改为 theme-card 网格多主题，配色与切换设置生效

## 通用
- [x] 各版本主题切换入口（界面设置/onboarding/主题切换条）已按真实源码还原
- [x] 各版本 `:root[data-theme=...]` 配色以真实源码 backgroundColor 为准，模糊主题从最近似推导
- [x] 13 个版本均无语法/诊断错误
- [x] 主题卡片点击后界面颜色明显变化、`.active` 高亮同步
- [x] 未破坏原有设置标签联动/onboarding/视图切换