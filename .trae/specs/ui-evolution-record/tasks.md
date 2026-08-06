# 界面演进记录 · 任务清单

## Phase 1: openhanako 版本评价
- [x] Task 1: 评价 openhanako 早期版本（v0.36.0 / v0.50.0 / v0.75.0 / v0.150.0）
  - 输出到 `prototypes/界面演进记录/01-openhanako-v036-v075.md` 和 `02-openhanako-v0150-v0198.md`
- [x] Task 2: 评价 openhanako 中期版本（v0.198.4 / v0.250.0 / v0.300.0 / v0.350.2）
  - 输出到 `02-openhanako-v0150-v0198.md` 和 `03-openhanako-v0250-v0350.md`
- [x] Task 3: 评价 openhanako 后期版本（v0.403.0 / v0.421.24 / v0.433.1 / v0.441.3 / v0.442.0）
  - 输出到 `04a-openhanako-v0403-v0421.md` 和 `04b-openhanako-v0433-v0442.md`

## Phase 2: reasonix 版本评价
- [x] Task 4: 评价 reasonix 版本 Tag（v1.19.4 / v1.19.6 / v1.19.7）
  - 输出到 `05a-reasonix-v119.md` 和 `05b-reasonix-v1197.md`
- [x] Task 5: 评价 reasonix 大改版节点（goal-autoresearch / composer-nav / terminal-session / context-v2）
  - 输出到 `05b-reasonix-v1197.md`
- [x] Task 6: 评价 reasonix 独立界面方向（surfaces/ 下 4 个 HTML）
  - 输出到 `06-reasonix-surfaces.md`

## Phase 3: 演进总结
- [x] Task 7: 撰写演进总结章节
  - 输出到 `07-演进总结.md`

## Phase 4: 文件分段
- [x] Task 8: 将单文件拆分为 10 个分段文件（每个 ≤273 行），存入 `prototypes/界面演进记录/` 文件夹
- [x] Task 9: 修复拆分边界截断问题（04a/04b 在 v0.433.1 边界、05a/05b 在 goal-autoresearch 边界）
- [x] Task 10: 删除原始大文件 `prototypes/界面演进记录.md`

# Task Dependencies
- Task 1/2/3 可并行执行（不同版本范围无依赖）
- Task 4/5/6 可并行执行
- Task 7 依赖 Task 1-6 全部完成
