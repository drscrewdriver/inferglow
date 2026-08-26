import { useEffect, useRef, useState } from 'react'
import { useSettingsStore } from './settings/settingsStore'
import { ThemeProvider } from './theme/ThemeProvider'
import { SettingsPanel } from './settings/SettingsPanel'
import { useSessionStore } from './stores/sessionStore'
import { AppFrame } from './layout/AppFrame'
import { SidebarRoot } from './sidebar/SidebarRoot'
import { Conversation } from './chat/Conversation'
import { DetailsPanel } from './details/DetailsPanel'

// ─── backend probe: live mode (REST wired) vs demo mode (backend down) ───
async function probeBackend(): Promise<boolean> {
  try {
    const resp = await fetch('/health', { signal: AbortSignal.timeout(1500) })
    return resp.ok
  } catch {
    return false
  }
}

export default function App() {
  const settings = useSettingsStore((s) => s.settings)
  const sessionCount = useSessionStore((s) => s.sessions.length)
  const activeId = useSessionStore((s) => s.activeId)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [termOpen, setTermOpen] = useState(false)
  const [live, setLive] = useState(false)
  const [liveChecked, setLiveChecked] = useState(false)
  const [detailsOpen, setDetailsOpen] = useState(false)
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false)

  useEffect(() => {
    void probeBackend().then((ok) => {
      setLive(ok)
      setLiveChecked(true)
    })
  }, [])

  // Details panel (Task 10): auto-collapse when the active session changes.
  const prevActive = useRef<string | null>(activeId)
  useEffect(() => {
    if (prevActive.current !== activeId) {
      prevActive.current = activeId
      setDetailsOpen(false)
    }
  }, [activeId])

  return (
    <ThemeProvider themeKey={settings.themeKey}>
      <div className="app-root">
        <header className="topbar">
          <div className="brand">
            <span className="logo">◈</span>Infer<b>Glow</b>
            <span className="ver">v0.2.0</span>
          </div>
          <span className="status">
            <span className="dot" />
            {live ? '运行中' : '未连接'}
          </span>
          <div className="channel-tabs">
            <button className="channel-tab active">聊天</button>
            <button className="channel-tab">频道</button>
          </div>
          <span className="spacer" />
          <button className="cmdk-btn">
            🔍 搜索命令、会话、设置… <span className="k">⌘ K</span>
          </button>
          <button
            className="icon-btn"
            title={detailsOpen ? '收起详情面板' : '打开详情面板'}
            onClick={() => setDetailsOpen((v) => !v)}
          >
            面板
          </button>
          <button className="icon-btn" title="目标模式">
            🎯
          </button>
          <button className="icon-btn" title="设置" onClick={() => setSettingsOpen(true)}>
            ⚙
          </button>
        </header>

        {liveChecked && !live && (
          <div className="demo-note">
            <span>⚠️</span>
            <span>
              <b>Demo 模式</b> · 后端未连接（请先启动{' '}
              <code style={{ fontFamily: 'var(--font-mono)' }}>inferglow-server</code>），当前界面仅展示布局。
            </span>
            <span className="close" title="关闭">
              ✕
            </span>
          </div>
        )}

        <AppFrame
          sidebar={
            <SidebarRoot
              collapsed={sidebarCollapsed}
              onToggle={() => setSidebarCollapsed((v) => !v)}
              onOpenSettings={() => setSettingsOpen(true)}
            />
          }
          details={<DetailsPanel open={detailsOpen} onToggle={() => setDetailsOpen((v) => !v)} />}
          detailsOpen={detailsOpen}
          onDetailsToggle={() => setDetailsOpen((v) => !v)}
        >
          <Conversation live={live} />
        </AppFrame>

        <footer className="statusbar">
          <span className="cell">
            ◈ <b>InferGlow</b>
          </span>
          <span className="cell">
            模型 <b style={{ color: 'var(--accent)' }}>deepseek-chat</b>
          </span>
          <span className="cell">
            会话 <b>{sessionCount}</b>
          </span>
          <span className="spacer" />
          <span className="cell">
            模式 <b>普通</b>
          </span>
          <button className="term-toggle" onClick={() => setTermOpen((v) => !v)}>
            ▾ 终端
          </button>
        </footer>

        {termOpen && (
          <div className="term-drawer term-drawer--open">
            <div className="term-drawer__inner">
              <div className="term-rails" />
              <div className="term-view" />
              <div className="term-input">
                <span className="prompt">sandbox$</span>
                <input placeholder="输入命令，Enter 执行…" autoComplete="off" spellCheck={false} />
              </div>
            </div>
          </div>
        )}

        {settingsOpen && <SettingsPanel onClose={() => setSettingsOpen(false)} />}
      </div>
    </ThemeProvider>
  )
}