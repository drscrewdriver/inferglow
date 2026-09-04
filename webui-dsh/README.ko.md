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

**DeepSeek Harness 전환판 Web UI** — `deepseek-harness`의 웹 인터페이스를 픽셀 단위로 복제한 것으로, 기술 시연 데모이자 자체 Agent UI를 빠르게 구축하기 위한 스캐폴드(뼈대)입니다.

## 왜 필요한가

`deepseek-harness` 사용자가 다른 Agent 체계로 이전할 때, 화면 차이로 인해**인지 비용**이 발생하기 쉽습니다. 이 프로젝트는 원본 화면을 픽셀 단위로 재현하여, 새 체계로 옮긴 뒤에도 「익숙한 화면과 익숙한 조작」을 유지함으로써 전환을 부드럽게 하고 학습 부담을 줄입니다.

동시에 DSH의 monorepo 의존성을 제거한 **standalone으로 실행되는 독립 React 스캐폴드**로, 이를 바탕으로 자신만의 Agent 화면을 빠르게 만들 수 있습니다.

## 핵심 목표

1. **픽셀 단위 복제** — DSH 원본의 레이아웃·색상·인터랙션(Sidebar, Chat, 콘텍스트/트레이스 패널, 테마 토큰 등)을 충실히 재현
2. **이전 인지 비용 절감** — 이전 후에도 익숙한 UI와 조작 방식을 유지
3. **스캐폴드 데모** — 독립·재사용·커스터마이즈 쉬운 Agent 화면의 출발점

## 기술 시연 (Tech Showcase)

- **독립 빌드** — monorepo에서 분리, `npm install && npm run build`로 `dist/` 생성
- **픽셀 정밀 레이아웃 재현** — 드래그 가능 사이드바(240–520px)·세션/트레이스/콘텍스트 패널·멀티탭 하단 패널
- **설정 가능한 엔트리** — 외부 boot manifest 수용, DSH 백엔드 하드코딩 없음
- **갖춘 엔지니어링** — ESLint + 엄격한 TypeScript + Vitest 단위 테스트（[INSTALL.ko.md](./INSTALL.ko.md)의「품질 보증」참조）

## 아키텍처

```
dsh-transition-webui/
├── package.json          # 독립 npm 패키지 정의
├── tsconfig.json         # TypeScript 설정
├── vite.config.ts        # Vite 빌드 설정
├── vitest.config.ts      # Vitest 테스트 설정
├── eslint.config.js      # ESLint flat config
├── index.html            # 엔트리 HTML
├── src/
│   ├── main.tsx          # 앱 엔트리
│   ├── boot.tsx          # 단순화 부트 커널
│   ├── app/
│   │   ├── App.tsx       # 루트 컴포넌트
│   │   └── layout/       # 레이아웃: Sidebar / DetailsPanel / PanePanels / TabbedPane …
│   ├── chat/             # 채팅 UI: ChatArea / ChatInput / MessageItem
│   ├── components/       # 기본 UI: Button / Icons
│   ├── styles/           # 글로벌 스타일 + 테마 토큰 (components.css / global.css)
│   ├── test/             # 테스트 setup
│   └── types/            # 타입 정의
└── dist/                 # 빌드 산출물 (gitignored)
```

## 스캐폴드 재사용 / 백엔드 연동

현재 `App.tsx`의 `handleSend`는 목(mock) 응답입니다. 실제 Agent 백엔드와 연동하려면 이를 교체합니다:

```typescript
async function handleSend(content: string) {
  // 1. 사용자 메시지 추가
  // 2. Agent 백엔드(WS / HTTP / SSE) 호출
  const response = await fetch('http://localhost:3080/api/chat', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ content }),
  })
  // 3. chunk별 스트리밍 렌더링
}
```

런타임 설정은 boot manifest（`window.__DASH_BOOT__`）로 주입하여 DSH 백엔드 주입을 대체합니다.

→ 빠른 시작·빌드·품질 커맨드는 [INSTALL.ko.md](./INSTALL.ko.md) 참조.

## 라이선스

MIT（deepseek-harness와 동일）