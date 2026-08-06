# Tasks — 增强 openhanako 主题切换视觉效果（全版本）

> 全程保持"即时更新 tasks.md 勾选"：每完成一个 task 立即勾选，禁止最后一次性补全。每个子代理返回后先核对勾选状态再继续。

- [x] Task 1: 全版本源码回溯确认主题机制（已完成初步调查，补充细节）
  - [x] 1.1 确认 13 个版本真实源码的主题 UI 形式（全部为 theme-card 卡片网格，无下拉）
  - [x] 1.2 确认各版本真实主题数量与 id（v0.36=6 / v0.50~v0.75=9 / v0.150~v0.403=registry动态 / C组=12）
  - [x] 1.3 提取主题配色参考（theme-registry-data.json 的 12 主题 backgroundColor）
  - [x] 1.4 对 registry 动态版本（v0.150~v0.403）确认其实际应展开的主题 id 列表（= 12 主题，含 new-warm-paper/midnight-contrast/coral/auto）

- [x] Task 2: 为 C 组 4 个版本补齐 `:root[data-theme=...]` 配色（并行子代理）
  - [x] 2.1 v0.421.24/index.html：补齐 12 套配色（现有 dark/light/midnight 后补其余 11 套）
  - [x] 2.2 v0.433.1/index.html：同上
  - [x] 2.3 v0.441.3/index.html：同上
  - [x] 2.4 v0.442.0/index.html：同上

- [x] Task 3: A 组（v0.36/v0.50/v0.75/v0.150）多主题还原（并行子代理，按真实源码主题数量）
  - [x] 3.1 v0.36.0/index.html：主题从下拉改为 6 主题卡片，补齐配色与切换设置
  - [x] 3.2 v0.50.0/index.html：改为 9 主题卡片，补齐配色与切换设置
  - [x] 3.3 v0.75.0/index.html：改为 9 主题卡片，补齐配色与切换设置
  - [x] 3.4 v0.150.0/index.html：改为 registry 主题卡片（12 主题），补齐配色与切换设置

- [x] Task 4: B 组（v0.198.4/v0.250/v0.300/v0.350.2/v0.403）多主题还原（并行子代理）
  - [x] 4.1 v0.198.4/index.html：主题从下拉改为 theme-card 网格（12 主题），补齐配色与切换设置
  - [x] 4.2 v0.250.0/index.html：同上
  - [x] 4.3 v0.300.0/index.html：同上
  - [x] 4.4 v0.350.2/index.html：同上
  - [x] 4.5 v0.403.0/index.html：同上

- [x] Task 5: 浏览器验证（并行抽验各组代表版本）
  - [x] 5.1 验证 v0.421.24：12 主题点击界面颜色明显变化、`.active` 高亮同步（warm-paper/high-contrast/midnight/midnight-contrast/coral/auto 抽验通过，bg 色均变化，activeCount=1）
  - [x] 5.2 验证 v0.36.0：6 主题卡片切换生效（warm-paper/midnight/high-contrast/grass-aroma/auto 抽验通过）
  - [x] 5.3 验证 v0.403.0：theme-card 网格多主题切换生效（12 主题，抽验 warm-paper/midnight/midnight-contrast/coral/auto 通过）
  - [x] 5.4 复核未破坏原有设置标签联动/onboarding/视图切换（v0.403.0 13 tab 联动、视图切换；v0.421.24 6 步 onboarding 进度点+下一步均正常）

# Task Dependencies
- [Task 2] 依赖 [Task 1]（配色参考）
- [Task 3] 依赖 [Task 1]（A 组主题数量）
- [Task 4] 依赖 [Task 1]（B 组主题数量）
- [Task 5] 依赖 [Task 2/3/4]（先补齐才能验证）
- Task 2/3/4 内部各版本相互独立可并行；Task 2/3/4 三组之间相互独立可并行