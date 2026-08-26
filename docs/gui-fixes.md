# InferGlow GUI 修复记录

## 2026-08-26 会话修复汇总

### 1. 深色模式文字不可读（黑字）
- **症状**：深色主题下 body 文字色为 rgb(0,0,0)，全部黑色不可读
- **根因**：CSS 变量 `guaranteed-invalid` 缓存陷阱 —— `--igw-text → var(--fg) → var(--text)` 两级间接引用在 Vite 多 `<style>` 标签注入顺序不确定时，`--fg` 和 `--igw-text` 首次计算引用了未定义的 `--text`，被永久冻结为空值
- **修复**：`themes.ts` 的 `resolveThemeVars` 中将文字别名（`--fg`、`--igw-text` 等）直接写为主题实色值，绕开间接引用
- **文件**：`web/src/theme/themes.ts`

### 2. Grid 布局错误 —— main 宽度=0、details 溢出
- **症状**：选中会话后主区域完全空白（无输入框/对话区），"面板"按钮文字竖排，详情面板溢出到主区域
- **根因**：AppFrame 有5个 grid 子元素（sidebar / divider / main / divider / details）但只有3列模板，所有子元素用 `grid-column: auto`。CSS Grid 自动换行把 main 放到第三列（0px），details 放到第二列（1554px）
- **修复**：
  - 给 `.sidebar`、`.main`、`.details` 显式指定 `grid-column: 1/2/3`
  - Dividers 改为 `position: absolute`，不再占 grid 空间，用 CSS 自定义属性定位
  - `.icon-btn` 去掉固定30px宽度，加 `white-space: nowrap` 修复"面板"竖排
- **文件**：`web/src/layout/AppFrame.module.css`、`web/src/layout/AppFrame.tsx`、`web/src/styles/app.css`

### 3. 深色模式卡片背景丢失
- **症状**：深色模式下卡片（投放队列、输入框、上下文窗口）与背景融为一体，无边框/背景对比
- **根因**：`tokens.css` 引用了 `--bg-elev`、`--bg-elev-2` 等变量，但 `themes.ts` 的 `resolveThemeVars` 未定义这些变量，触发 CSS 缓存陷阱
- **修复**：在 `themes.ts` 的 vars 对象中补全 `--bg-elev`、`--bg-elev-2`、`--bg-soft`、`--sidebar-bg`、`--sidebar-hover`、`--avatar-fg`、`--err`、`--radius`、`--font-ui`、`--font-mono` 等缺失变量
- **文件**：`web/src/theme/themes.ts`

### 4. 重影文字（/think 重复渲染）
- **症状**：输入 `/think` 时文字出现两次（ghosting）
- **根因**：MentionInput 的 backdrop 渲染了全部文本（包括纯文本），和透明 textarea 重叠
- **修复**：backdrop 只渲染 chip 片段（`<span className={styles.mention}>`），纯文本用 `&nbsp;` 占位；无 chip 时 backdrop 隐藏
- **文件**：`web/src/filetag/MentionInput.tsx`

### 5. 命令菜单未关闭
- **症状**：输入 `/think `（带空格）后命令菜单仍然显示
- **根因**：`onChange` 中 `if (v.startsWith('/')) setCmdOpen(true)` 没有检测空格（命令完成标志）
- **修复**：改为 `setCmdOpen(v.startsWith('/') && !v.includes(' '))`
- **文件**：`web/src/chat/Conversation.tsx`

### 6. 投放队列重设计
- **症状**：队列 dock 占据 composer 下方大块空间，样式不匹配参考设计
- **修复**：新建 `QueueBar` 组件替代 `SteerQueueDock` —— 浮动在输入框上方（`border-radius: 10px 10px 0 0` + `border-bottom: none`），仅有排队项时显示，每行彩色圆点（🟢稍后/🟡下一步/🔴立即）+ 文本 + 操作按钮
- **文件**：`web/src/traffic/QueueBar.tsx`、`web/src/traffic/slots.tsx`、`web/src/traffic/traffic.module.css`

### 7. Composer 工具栏重排
- **症状**：工具栏过于拥挤，思考级别和上下文开关在错误位置
- **修复**：
  - 工具栏精简为仅 `/` 按钮 + 模型选择器
  - 思考级别 toggle（轻/高/极高）移到 composerBar（textarea 下方）
  - 替换 "更大上下文" toggle 为分档选择器（128k / 256k / 400k / 1M）
- **文件**：`web/src/chat/Conversation.tsx`、`web/src/chat/conversation.module.css`

---

## CI 修复

### prefer-const lint 错误
- **症状**：`web/src/context/fold.ts:58` 报 `'time' is never reassigned. Use 'const'`
- **修复**：`let time = 0` → `const time = 0`
- **文件**：`web/src/context/fold.ts`

### TS 构建错误（Phase 6 遗留）
- `approvalStore.ts`：`justification` 未用 → 补进审批请求体
- `schema.ts`：`SandboxMode['risk']` 不存在 → 改 `SandboxModeInfo['risk']`
- `decide.ts`：`isSimpleCall` 未用参数 → `_name`
- **文件**：对应三个文件
