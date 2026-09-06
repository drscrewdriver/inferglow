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

**DeepSeek Harness 移行版 Web UI** — `deepseek-harness` の画面をピクセル単位で再現した Web インターフェースで、技術紹介デモであり、独自の Agent UI を素早く構築するためのスキャフォールド（雛形）です。

## なぜ必要か

`deepseek-harness` の利用者が他の Agent 体系へ移行する際、画面の違いによって**認知コスト**が生じがちです。本プロジェクトは元画面をピクセル単位で再現し、新体系へ乗り換えた後も「見慣れた画面・見慣れた操作」を保つことで、移行をスムーズにし、学習負担を軽減します。

同時に DSH の monorepo 依存を排除した、**単体で動作する独立した React スキャフォールド**でもあり、そのまま独自の Agent 画面の土台にできます。

## 主な目標

1. **ピクセル単位の再現** — DSH 元版のレイアウト・配色・操作感（Sidebar・Chat・コンテキスト/トレース面板・テーマトークン等）を忠実に再現
2. **移行の認知コスト低減** — 移行先でも、見慣れた UI・操作感を維持
3. **スキャフォールド demo** — 独立・再利用・カスタマイズしやすい Agent 画面の出発点

## 技術紹介 (Tech Showcase)

- **単体ビルド** — monorepo から独立し、`npm install && npm run build` で `dist/` を生成
- **ピクセル精度のレイアウト再現** — ドラッグ可能なサイドバー(240–520px)・セッション/トレース/コンテキスト面板・多タブの下部面板
- **設定可能なエントリ** — 外部 boot manifest を受け付け、DSH バックエンドをハードコードしない
- **充実したエンジニアリング** — ESLint + 厳格な TypeScript + Vitest 単体テスト（[INSTALL.ja.md](./INSTALL.ja.md) の「品質保証」参照）

## アーキテクチャ

```
dsh-transition-webui/
├── package.json          # 独立 npm パッケージ定義
├── tsconfig.json         # TypeScript 設定
├── vite.config.ts        # Vite ビルド設定
├── vitest.config.ts      # Vitest テスト設定
├── eslint.config.js      # ESLint flat config
├── index.html            # エントリ HTML
├── src/
│   ├── main.tsx          # アプリエントリ
│   ├── boot.tsx          # 簡略化ブートカーネル
│   ├── app/
│   │   ├── App.tsx       # ルートコンポーネント
│   │   └── layout/       # レイアウト: Sidebar / DetailsPanel / PanePanels / TabbedPane …
│   ├── chat/             # チャット UI: ChatArea / ChatInput / MessageItem
│   ├── components/       # 基本 UI: Button / Icons
│   ├── styles/           # グローバルスタイル + テーマトークン (components.css / global.css)
│   ├── test/             # テスト setup
│   └── types/            # 型定義
└── dist/                 # ビルド成果物 (gitignored)
```

## スキャフォールドとして再利用 / バックエンド接続

現在 `App.tsx` の `handleSend` はモック応答です。実際の Agent バックエンドと繋ぐには、これを置き換えます：

```typescript
async function handleSend(content: string) {
  // 1. ユーザーメッセージを追加
  // 2. Agent バックエンド(WS / HTTP / SSE)を呼ぶ
  const response = await fetch('http://localhost:3080/api/chat', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ content }),
  })
  // 3. chunk ごとにストリーミング描画
}
```

起動時設定は boot manifest（`window.__DASH_BOOT__`）で注入でき、DSH バックエンドの注入を置き換えます。

→ クイックスタート・ビルド・品質コマンドは [INSTALL.ja.md](./INSTALL.ja.md) 参照。

## ライセンス

MIT（deepseek-harness と同様）