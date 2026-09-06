- [English README](./README.md)
- [中文 README](./README.zh.md)
- [日本語 README](./README.ja.md)
- [한국어 README](./README.ko.md)
- [Installation guide](./INSTALL.md)
- [中文安装指南](./INSTALL.zh.md)
- [日本語インストールガイド](./INSTALL.ja.md)
- [한국어 설치 안내](./INSTALL.ko.md)
- [Changelog](./CHANGELOG.md)
- [中文 changelog](./CHANGELOG.zh.md)
- [日本語 changelog](./CHANGELOG.ja.md)
- [한국어 changelog](./CHANGELOG.ko.md)

# dsh-transition-webui

**DeepSeek Harness 过渡版 Web UI** — 像素级复刻 `deepseek-harness` 的网页界面，作为技术展示 demo 与快速搭建自有 Agent 界面的脚手架。

## 为什么需要它

`deepseek-harness` 用户迁移到其他 Agent 体系时，往往因界面差异而产生认知成本。本项目对原版界面做**像素级复刻**，让用户在换到新体系后依然能「看到熟悉的界面、用熟悉的方式交互」，从而平滑过渡、减少学习负担。

同时它剥离了 DSH 的 monorepo 依赖，是一个**可直接运行的独立 React 脚手架**——你可以基于它快速搭建自己的 Agent 界面。

## 核心目标

1. **像素级复刻** — 尽量还原 DSH 原版的布局、配色、交互细节（Sidebar、Chat、上下文/轨迹面板、主题 token 等）
2. **降低迁移认知成本** — 迁移到其他 Agent 体系时，用户看到的 UI 与习惯保持一致
3. **脚手架 demo** — 独立、可复用、易定制的 Agent 界面起点，用于快速搭建自有界面

## 技术演示 (Tech Showcase)

- **独立构建** — 脱离 monorepo，`npm install && npm run build` 即可产出 `dist/`
- **像素级布局还原** — 侧边栏拖拽(`240–520px`)、会话/轨迹/上下文面板、底部面板多 Tab
- **可配置入口** — 接受外部 boot manifest，不硬编码 DSH 后端
- **工程化完备** — ESLint + TypeScript 严格检查 + Vitest 单元测试，见 [INSTALL.md](./INSTALL.zh.md) 的「质量保证」

## 架构

```
dsh-transition-webui/
├── package.json          # 独立 npm 包定义
├── tsconfig.json         # TypeScript 配置
├── vite.config.ts        # Vite 构建配置
├── vitest.config.ts      # Vitest 测试配置
├── eslint.config.js      # ESLint flat config
├── index.html            # 入口 HTML
├── src/
│   ├── main.tsx          # 应用入口
│   ├── boot.tsx          # 简化版启动内核
│   ├── app/
│   │   ├── App.tsx       # 根组件
│   │   └── layout/       # 布局：Sidebar / DetailsPanel / PanePanels / TabbedPane …
│   ├── chat/             # 聊天 UI：ChatArea / ChatInput / MessageItem
│   ├── components/       # 基础 UI 组件：Button / Icons
│   ├── styles/           # 全局样式 + 主题 token (components.css / global.css)
│   ├── test/             # 测试 setup
│   └── types/            # 类型定义
└── dist/                 # 构建产物 (gitignored)
```

## 作为脚手架复用 / 对接后端

当前 `App.tsx` 中 `handleSend` 为模拟响应。对接真实 Agent 后端，替换它即可：

```typescript
async function handleSend(content: string) {
  // 1. 添加用户消息
  // 2. 调用你的 Agent 后端（WS / HTTP / SSE 均可）
  const response = await fetch('http://localhost:3080/api/chat', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ content }),
  })
  // 3. 逐 chunk 流式渲染
}
```

可用启动参数 / boot manifest（`window.__DASH_BOOT__`）注入配置，替换 DSH 后端的注入逻辑。

→ 快速开始、构建与质量保证命令见 [INSTALL.zh.md](./INSTALL.zh.md)。

## License

MIT（同 deepseek-harness）