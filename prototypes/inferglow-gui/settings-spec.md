# InferGlow · 设置面板规范（Sync 标注）

> 本文件是 InferGlow 设置面板的**单一事实来源（source of truth）**。
> 前端 `settingsTemplates` 与导航 `#settingsNav` 必须与本文件逐项对齐；后续任何「同步 / 重构 / 版本差异」都以本标注为准。
>
> 约定：
> - 每个标签 = 一个 `data-tab` key（导航 `navitem` 与 `settingsTemplates[tab]` 共用）。
> - 每个字段 = 一个数据 key（`key`），用于入库/持久化与双向同步。
> - 控件类型：`switch`(开关) / `seg`(分段) / `range`(滑块) / `select`(下拉) / `input`(文本) / `kbd`(键位) / `row`(列表行)。

---

## 一、导航分组与标签清单

导航按 **4 个分组** 组织，共 **15 个标签**。

| data-tab | 名称 | 副标题 | 分组 |
|----------|------|--------|------|
| `general` | 通用 | 偏好 · 启动 | 常规 |
| `appearance` | 外观 | 主题 · 排版 | 常规 |
| `interface` | 界面 | 布局 · 显示 | 常规 |
| `shortcuts` | 快捷键 | 键位映射 | 常规 |
| `providers` | 提供方 | 模型提供方 · 用量 | 模型与智能 |
| `models` | 模型 | 默认模型 · 上下文窗口 | 模型与智能 |
| `credentials` | 凭证 | API Keys | 模型与智能 |
| `memory` | 推理与上下文 | 思考等级 · 压缩 · RTK | 模型与智能 |
| `permissions` | 权限 | 执行 · 审批 · 沙箱 | 执行与安全 |
| `security` | 安全 | 审计 · 浏览器指纹 | 执行与安全 |
| `skills-tools` | 技能与工具 | 技能库 · 工具开关 | 集成 |
| `connectors` | 连接器 | MCP · 桥接 · 浏览器 | 集成 |
| `schedules` | 调度 | 定时任务 · Webhook | 集成 |
| `experiments` | 实验与插件 | 实验特性 · 插件 | 高级 |
| `about` | 关于 | 版本 · 许可 | 高级 |

---

## 二、各标签结构

> 每个标签 = 若干「区块（card）」，每个区块 = 若干「字段（field）」。

### 1. `general` 通用
- **偏好**
  - `default_model` 默认模型 — `select`（候选：deepseek-chat / deepseek-reasoner / openai-compatible）
  - `web_search` 服务端网页搜索 — `switch`（on）
  - `write_probe` 写入权限探测 — `switch`（on）
  - `auto_compact` 自动缩略历史 — `switch`（on，接近上下文上限时 compact）
- **启动**
  - `restore_last` 启动时恢复上次会话 — `switch`（on）
  - `show_onboarding` 首次启动显示引导 — `switch`（off）
- **语言与更新**
  - `language` 界面语言 — `select`（简体中文 / English）
  - `auto_update` 自动更新 — `switch`（on）
  - `telemetry` 匿名遥测 — `switch`（off）

### 2. `appearance` 外观
- **主题**（核心区块，见「三、主题系统」）
  - `theme_mode` 主题模式 — `seg`（自动 / 浅色 / 深色）
  - `theme_card` 主题（SpaceX 主题卡片网格）— 点选卡片切换整套配色
  - `accent` 强调色 — 跟随主题卡片内置，可单独微调
- **排版**
  - `font_ui` 界面字体 — `select`
  - `font_mono` 等宽字体 — `select`
  - `font_size` 字号 — `range`（12–20，默认 14）
- **缩放**
  - `zoom` 显示缩放 — `range`（50–200，默认 100）

### 3. `interface` 界面
- **布局**
  - `content_width` 会话内容宽度 — `seg`（标准 960px / 全宽 90%）
  - `dock_visible` 右侧 dock 默认显示 — `switch`（on）
  - `sidebar_visible` 左侧会话栏默认显示 — `switch`（on）
- **消息**
  - `show_timestamps` 显示消息时间戳 — `switch`（on）
  - `show_thought` 显示思考帧 — `switch`（on）
  - `tool_auto_expand` 工具卡片自动展开输出 — `switch`（off）
- **终端**
  - `term_drawer` 终端抽屉吸附 — `switch`（on）
  - `term_font` 终端字体 — `select`

### 4. `shortcuts` 快捷键
- **全局**
  - `k_cmd_palette` 打开命令面板 `⌘ K`
  - `k_goal_mode` 切换目标模式 `⌘ G`
  - `k_settings` 打开设置 `⌘ ,`
  - `k_new` 新建会话 `⌘ N`
  - `k_term` 切换终端抽屉 `⌘ T`
  - `k_send` 发送消息 `Enter`
  - `k_newline` 换行 `Shift + Enter`
- **导航**
  - `k_next` 下一个会话 `⌘ ↓`
  - `k_prev` 上一个会话 `⌘ ↑`
  - `k_focus` 聚焦输入框 `⌘ L`

### 5. `providers` 提供方
- **提供方列表**（`row` 列表）
  - `deepseek` — 已连接 / 配置
  - `openai-compatible` — 未配置 / 配置
- **用量统计**
  - `tokens_month` 本月 tokens
  - `cache_hit` 缓存命中率
  - `cost_month` 本月费用

### 6. `models` 模型
- **模型列表**（`row` 列表：名称 / 上下文窗口 / 温度）
  - `deepseek-chat` — ctx 128k
  - `deepseek-reasoner` — ctx 128k
  - `openai-compatible/gpt` — ctx 取决于提供方
- **默认**
  - `model_default` 默认对话模型 — `select`
  - `model_temp` 温度 — `range`（0–2，默认 1）
  - `model_max_tokens` 最大输出 tokens — `range`

### 7. `credentials` 凭证
- **已保存凭证**（`row` 列表：名称 / 状态 / 测试 / 删除）
  - `deepseek` — 可用
  - `openai` — 无效 / 需重填
- **操作**
  - `cred_add` ＋ 添加凭证（触发加密存储流程）
- 说明：本地加密存储，不落明文；`test_btn` 模拟「测试连接」。

### 8. `memory` 推理与上下文
- **推理**
  - `reason_level` 思考等级 — `seg`（低 / 中 / 高）
  - `stream_echo` 流式回显 — `switch`（on）
  - `show_reason` 显示推理帧 — `switch`（on）
- **上下文压缩**（对接 InferGlow L0→L3 单向压缩）
  - `compress_level` 压缩级别 — `seg`（L0 原文 / L1 摘要 / L2 语义 / L3 极简）
  - `rtk_enable` 启用 RTK 过滤 — `switch`（off）
  - `lock_l0` 允许锁定 L0 — `switch`（off，开启后可用 `context_lock_l0` 工具）
  - `auto_compact_threshold` 自动压缩阈值 — `range`（60–95%，默认 85）

### 9. `permissions` 权限
- **执行**
  - `write_probe` 写入文件前探测 — `switch`（on）
  - `cmd_approval` 执行命令需审批 — `switch`（off）
  - `sandbox_tools` 沙箱内运行工具 — `switch`（on）
- **网络**
  - `net_outbound` 允许出站网络请求 — `switch`（on）
  - `net_download` 允许下载可执行文件 — `switch`（off）

### 10. `security` 安全
- **审计**
  - `audit_enabled` 启用审计日志 — `switch`（on）
  - `audit_path` 日志路径 — `input`（默认 `audit.log`）
  - `audit_before_validate` 审计先于校验 — `switch`（on，硬约束）
- **浏览器**
  - `cdp_stealth` 指纹规避 — `switch`（on）
  - `cdp_automation` 自动化标记隐藏 — `switch`（on）
- **密钥**
  - `encrypt_at_rest` 本地落盘加密 — `switch`（on）

### 11. `skills-tools` 技能与工具
- **技能库**（`row` 列表：名称 / 来源 / 启用）
  - `skill_web_search` 网页搜索
  - `skill_file_ops` 文件操作
  - `skill_browser` 浏览器自动化
- **工具**
  - `tool_auto_expand` 工具卡片自动展开 — `switch`（off）
  - `tool_confirm_destructive` 破坏性操作需确认 — `switch`（on）

### 12. `connectors` 连接器
- **MCP 服务器**（`row` 列表：名称 / 传输 / 启用）
  - `mcp_filesystem` 文件系统 — stdio
  - `mcp_browser` 浏览器 — sse
- **桥接**
  - `bridge_feishu` 飞书 — 未连接
  - `bridge_social` 社交平台 — 未连接
- **浏览器**
  - `browser_profile` 复用真实登录态 profile — `switch`（on）
  - `browser_partition` 隔离上下文分区 — `switch`（on）

### 13. `schedules` 调度
- **定时任务**（`row` 列表：名称 / cron / 状态 / 编辑）
  - `sch_night_release` 夜间发布检查 `0 2 * * *`
  - `sch_memory_archive` 每周记忆归档 `0 3 * * 0`
- **操作**
  - `schedule_add` ＋ 新建调度

### 14. `experiments` 实验与插件
- **实验特性**（`switch` 列表）
  - `exp_goal_autoresearch` 目标自动研究
  - `exp_cross_step_fusion` 跨步骤语义融合（上下文压缩）
  - `exp_bridge_composer` 富输入 Composer（文件 chip）
- **插件**
  - `plugin_market` 插件市场入口
  - 已装插件列表（`row`）

### 15. `about` 关于
- **版本**
  - `app_version` 版本号 `v0.1.17`
  - `app_name` InferGlow · 三层桌面 GUI
- **更新**
  - `update_check` 检查更新（`btn`）
- **许可**
  - 开源许可 / 致谢

---

## 三、主题系统（SpaceX 无厘头主题）

> hanako 的暖纸色系启发我们：主题不应只是「换强调色」，而应是**整套命名配色**。
> 下面设计一套 **SpaceX 式无厘头主题**：名字荒诞、有梗，但**配色品味要高**（低饱和、克制、有记忆点）。
> 每个主题 = 完整的 CSS 变量集（`bg/bg2/panel/panel2/border/text/accent`），可直接落在 `:root[data-theme="<codename>"]` 或 `--theme:<codename>`。

### 主题清单

> 已实装为 `appearance` 标签的主题卡片网格（`.theme-card`），**两组对比**：
> - **OpenHana 组**（12 套）：直接平移 openhanako v0.442.0 原封 themes。
> - **原创组**（8 套）：伊恩·班克斯《文明》系列飞船名（OCISLY / JRTI / ASOG 同源）。

**OpenHana 主题（12 套）**

| codename | 名称 | mode | 气质 |
|----------|------|------|------|
| `midnight` | 青夜 | 夜间（默认） | 开放式深蓝 |
| `auto` | 自动 | 跟随系统 | 深色 · 草木 |
| `warm-paper` | 暖纸 | 白天 | 暖纸 · 蜜茶 |
| `high-contrast` | 素白 | 高对比 | 素白 |
| `grass-aroma` | 草香 | Butter | 皂绿 |
| `contemplation` | 沉思 | Ming | 雾蓝 |
| `absolutely` | Absolutely | 有一点点熟悉 | 米白 · 极简 |
| `delve` | 随时准备接住你 | 探究一下 | 纯白 · 简明 |
| `deep-think` | 用户彻底怒了 | 小鲸鱼 | 净白 · 靛蓝 |
| `new-warm-paper` | 新暖纸 | 纸本 | 焦糖 |
| `midnight-contrast` | 青夜·高对比 | 清晰 | 高对比 |
| `coral` | 珊瑚 | 春日和纸 | 珊瑚 |

**原创主题（8 套，班克斯飞船名）**

| codename | 名称 | 出处 β | 气质 |
|----------|------|--------|------|
| `ocisly` | 当然我还爱你 | OCISLY · 《The Player of Games》 | 靛蓝夜 + 落海燃料橙 |
| `jrti` | 读一下说明书 | JRTI · 《The Player of Games》 | 香草纸 + 枫糖（浅色） |
| `gravitas` | 重力不足 | A Shortfall of Gravitas · 《文明》 | 深紫 + 信号绿 |
| `lover` | 新恋人将至的期待 | The Anticipation of a New Lover's Arrival · 《Use of Weapons》 | 陶土红 + 玫瑰金 |
| `attitude` | 态度调节器 | Attitude Adjuster · 《Consider Phlebas》 | 冷钢灰 + 空调青 |
| `killing` | 消磨时光 | Killing Time · 《Consider Phlebas》/《Excession》 | 碳黑 + 琥珀 |
| `funny` | 奇怪，上次还好使 | Funny, It Worked Last Time... · 《Look to Windward》 | 石板灰 + 酸橙 |
| `windward` | 迎风远眺 | Look To Windward · 《Look to Windward》 | 炭灰 + 月银蓝 |

### 主题调色板（CSS 变量）

**`ocisly` 当然我还爱你**
```css
--bg:#141319; --bg2:#1a1921; --panel:#1f1e27; --panel2:#262431;
--border:#322f3e; --border-soft:#2a2836;
--text:#ece9f2; --text-dim:#a29db0; --text-faint:#6f6a7d;
--accent:#ff9e5e; --accent-fg:#2a1508; --accent-strong:#ffb37d; --accent-soft:rgba(255,158,94,.16);
```

**`attitude` 态度调节器**
```css
--bg:#131518; --bg2:#191c20; --panel:#1e2126; --panel2:#25292f;
--border:#31363d; --border-soft:#292d34;
--text:#e8ecf1; --text-dim:#9aa2ad; --text-faint:#666e79;
--accent:#6fd3e8; --accent-fg:#0a1a1f; --accent-strong:#8fe0f0; --accent-soft:rgba(111,211,232,.16);
```

**`lover` 新恋人将至的期待**
```css
--bg:#171316; --bg2:#1d181c; --panel:#221d21; --panel2:#2a2429;
--border:#382f36; --border-soft:#2f282e;
--text:#efe6ea; --text-dim:#a89aa1; --text-faint:#74666e;
--accent:#e08a9a; --accent-fg:#2a1116; --accent-strong:#f0a3b1; --accent-soft:rgba(224,138,154,.16);
```

**`killing` 消磨时光**
```css
--bg:#0e0f10; --bg2:#141517; --panel:#181a1c; --panel2:#1f2124;
--border:#2c2f33; --border-soft:#26282b;
--text:#ecebe7; --text-dim:#9d9b95; --text-faint:#6a6863;
--accent:#d9a45b; --accent-fg:#2a1c08; --accent-strong:#ecc383; --accent-soft:rgba(217,164,91,.16);
```

**`jrti` 读一下说明书**（浅色系）
```css
--bg:#f5f1e8; --bg2:#ede7d9; --panel:#faf7f0; --panel2:#f1ecdf;
--border:#ddd6c4; --border-soft:#e6e0d1;
--text:#2b2822; --text-dim:#6f6a5e; --text-faint:#9c967f;
--accent:#a8793f; --accent-fg:#fff; --accent-strong:#c2925a; --accent-soft:rgba(168,121,63,.13);
```

**`funny` 奇怪，上次还好使**
```css
--bg:#121417; --bg2:#181b1f; --panel:#1c1f24; --panel2:#23262c;
--border:#2f333a; --border-soft:#272b31;
--text:#e9ece9; --text-dim:#9aa69c; --text-faint:#667068;
--accent:#a8d66a; --accent-fg:#1a2408; --accent-strong:#bfe08a; --accent-soft:rgba(168,214,106,.15);
```

**`windward` 迎风远眺**
```css
--bg:#14161a; --bg2:#1a1d22; --panel:#1f2228; --panel2:#262a31;
--border:#32363e; --border-soft:#2a2e35;
--text:#e9ebef; --text-dim:#9ba0aa; --text-faint:#676c76;
--accent:#9fb4d8; --accent-fg:#101827; --accent-strong:#bccbe8; --accent-soft:rgba(159,180,216,.16);
```

**`gravitas` 重力不足**
```css
--bg:#141218; --bg2:#1a1820; --panel:#1f1c26; --panel2:#262332;
--border:#322e3e; --border-soft:#2a2736;
--text:#ebe7f0; --text-dim:#a29cae; --text-faint:#6d6678;
--accent:#7fd8a0; --accent-fg:#0a2415; --accent-strong:#9ce6ba; --accent-soft:rgba(127,216,160,.16);
```

### 接入方式

- **已实装**：`index.html` 内 `THEMES` 对象集中定义 **20 套主题**（OpenHana 12 套 + 原创 8 套），`appearance` 标签按分组渲染主题卡片网格（`.theme-group` + `.theme-card`），点击 `applyThemeCard(codename)` 一次性写入全套 CSS 变量到 `documentElement`，并自动切换 `data-theme` 明暗。
- 每张卡片含 5 个色板样块（bg/panel2/accent/text/gold）、名称、气质、出处、明暗徽标。
- OpenHana 组字段缺省（`accentFg/accentStrong/goal/avatar`）由 `applyThemeCard` 统一派生兜底。
- 深色为主：OpenHana 组中 `midnight/midnight-contrast/auto` 为深色，其余浅色；原创组除 `jrti` 外均为深色。选中卡片高亮描边 + accent-soft 光晕。
- 与 `resolveTheme()`/`applyAccent()` 解耦：主题卡片一次性写入整套变量，替代「仅改 accent」。

---

> **同步约定**：导航 `data-tab` 与 `settingsTemplates` 的 key 必须与本表一致；新增 / 删除标签时先改本文件，再改前端，避免「占位标签」遗漏。