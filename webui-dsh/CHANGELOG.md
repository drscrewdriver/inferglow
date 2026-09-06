# Changelog

All notable changes to this project. Follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- Scaffold `dsh-transition-webui` (React 18 + Vite + TypeScript)
- Pixel-perfect reproduction of DeepSeek Harness layout: draggable sidebar (240–520px), session/trace/context panels, multi-tab bottom panel, session management
- Standalone boot kernel `boot.tsx` (`window.__DASH_BOOT__` / `__DASH_PLUGINS__`), decoupled from the DSH monorepo
- Unit tests: Vitest + Testing Library (jsdom); first component tests cover `Button`
- ESLint flat config + strict TypeScript type checks
- Adaptive, draggable conversation content width: `ConversationWidthHandles` + `conversationWidth.ts` — a centered, resizable transcript column with the width preference persisted to `localStorage` and symmetric `col-resize` handles on both sides (mirrors dsh v0.1.2-alpha.3 `ui-conversation` ConversationRoot + WidthHandle)
- Fish logo and a `预览版` (preview) badge in the hero area; the collapsed sidebar rail now renders an SVG logo instead of the `dsh` text

### Changed

- Renamed from `portable-dsh-shell` to `dsh-transition-webui`
- Composer toolbar: re-enabled the command button; the send button's tooltip/label now reads `发送消息`

### Fixed

- Dark theme attribute not in sync when settings change (`SettingsModal` effect deps completed)
- Several lint issues: no-op ternary expressions, `let`→`const`, unused parameters

## [1.0.0] - 2026-08-30

### Added

- First usable release: runs as a scaffold demo for rapidly building your own Agent UI.