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

# Installation Guide

Prerequisites, build, and quality commands for `dsh-transition-webui`.

## Prerequisites

- [Node.js](https://nodejs.org/) >= 18
- npm (bundled with Node.js)

## Quick start

```bash
git clone https://github.com/drscrewdriver/dsh-transition-webui.git
cd dsh-transition-webui

npm install

npm run dev          # dev server → http://localhost:3081
npm run build        # production build → dist/
npm run preview      # preview the production build
```

`npm run dev` starts a Vite dev server. Open the printed URL in your browser.

## Quality

```bash
npm run lint            # ESLint
npm run typecheck       # TypeScript type check (tsc --noEmit)
npm run test            # Vitest unit tests (jsdom)
npm run test:watch      # run tests in watch mode
```

## Connecting a backend

See the "Reusing as a scaffold / talking to a backend" section of the [README](./README.md). Replace the mock `handleSend` in `App.tsx` with a real call to your Agent backend (WS / HTTP / SSE). Boot-time settings can be injected via `window.__DASH_BOOT__`.

## Troubleshooting

- **Port 3081 already in use** — the dev server uses `strictPort`. Free the port or change `server.port` in `vite.config.ts`.
- **ESLint binary missing** — dependencies may be incomplete; run `npm install` again.
- **Tests fail to find `Button.test.tsx`** — the test glob is inherited from Vitest defaults; ensure `vitest.config.ts` is present.

## License

MIT (same as deepseek-harness)