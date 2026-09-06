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

**DeepSeek Harness transition Web UI** — a pixel-perfect replica of the `deepseek-harness` web interface, serving as a tech-showcase demo and a scaffold for rapidly building your own Agent UI.

## Why this project

When users migrate away from `deepseek-harness` to another Agent system, UI differences typically incur a **cognitive cost**. This project recreates the original interface at pixel level so that, after switching to a new system, users still *see a familiar UI and interact in a familiar way* — easing the transition and reducing the learning burden.

At the same time it strips out DSH's monorepo dependencies and is a **self-contained, runnable React scaffold** you can base your own Agent UI on.

## Core goals

1. **Pixel-perfect replica** — faithfully reproduce DSH's layout, colors, and interactions (Sidebar, Chat, Context/Trace panels, theme tokens, etc.)
2. **Lower migration cognitive cost** — the UI users see stays consistent with what they are used to
3. **Scaffold demo** — an independent, reusable, and customizable Agent-UI starting point

## Tech showcase

- **Standalone build** — decoupled from the monorepo; `npm install && npm run build` produces `dist/`
- **Pixel-fidelity layout** — draggable sidebar (240–520px), session/trace/context panels, multi-tab bottom panel
- **Configurable entry** — accepts an external boot manifest, no hardcoded DSH backend
- **Ready engineering** — ESLint + strict TypeScript + Vitest unit tests, see the "Quality" section in [INSTALL.md](./INSTALL.md)

## Architecture

```
dsh-transition-webui/
├── package.json          # standalone package definition
├── tsconfig.json         # TypeScript config
├── vite.config.ts        # Vite build config
├── vitest.config.ts      # Vitest test config
├── eslint.config.js      # ESLint flat config
├── index.html            # entry HTML
├── src/
│   ├── main.tsx          # app entry
│   ├── boot.tsx          # simplified boot kernel
│   ├── app/
│   │   ├── App.tsx       # root component
│   │   └── layout/       # layout: Sidebar / DetailsPanel / PanePanels / TabbedPane …
│   ├── chat/             # chat UI: ChatArea / ChatInput / MessageItem
│   ├── components/       # base UI: Button / Icons
│   ├── styles/           # global styles + theme tokens (components.css / global.css)
│   ├── test/             # test setup
│   └── types/            # type definitions
└── dist/                 # build output (gitignored)
```

## Reusing as a scaffold / talking to a backend

The current `handleSend` in `App.tsx` is a mock. To connect a real Agent backend, replace it:

```typescript
async function handleSend(content: string) {
  // 1. add the user message
  // 2. call your Agent backend (WS / HTTP / SSE all fine)
  const response = await fetch('http://localhost:3080/api/chat', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ content }),
  })
  // 3. stream-render chunk by chunk
}
```

Run-time settings can be injected via a boot manifest (`window.__DASH_BOOT__`), replacing DSH's backend injection.

→ Quick start, build, and quality commands live in [INSTALL.md](./INSTALL.md).

## License

MIT (same as deepseek-harness)