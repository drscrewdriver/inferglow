/**
 * Root App — assembles the full three-column workspace:
 *   aside.dsh-workspace-sidebar  |  main.dsh-workspace-main  |  aside.dsh-workspace-details
 * + panel toggles, resizable details column, and a bottom tabbed panel.
 */

import { useLayoutEffect, useEffect, useState, useRef, type PointerEvent as ReactPointerEvent } from 'react'
import { store, subscribe } from '../store.ts'
import { bootstrap } from '../bridge/inferglow.ts'
import { Sidebar } from './layout/Sidebar.tsx'
import { DetailsPanel } from './layout/DetailsPanel.tsx'
import { ChatArea } from '../chat/ChatArea.tsx'
import { SettingsModal } from './layout/SettingsModal.tsx'
import { TabbedPane, type PanelTab } from './layout/TabbedPane.tsx'

const DETAILS_MIN = 240
const DETAILS_MAX = 520

const DEFAULT_BOTTOM_TABS: PanelTab[] = [
  { id: 'files', kind: 'files', label: 'Files', active: true },
  { id: 'pwsh', kind: 'terminal', label: 'powershell', active: false },
]

export function AppShell() {
  // Ensure colorScheme is dark before first paint (FOUC prevention)
  useLayoutEffect(() => {
    document.documentElement.style.colorScheme = 'dark'
  }, [])
  const [activeSessionId, setActiveSessionId] = useState(store.activeSessionId)
  const [settingsOpen, setSettingsOpen] = useState(store.settingsOpen)
  const [backendOnline, setBackendOnline] = useState(store.backendOnline)
  const [detailsOpen, setDetailsOpen] = useState(true)
  const [detailsWidth, setDetailsWidth] = useState(284)
  const [bottomOpen, setBottomOpen] = useState(false)
  const [convTab, setConvTab] = useState('对话')

  /* Subscribe to store */
  useEffect(() => {
    const unsub = subscribe(() => {
      setActiveSessionId(store.activeSessionId)
      setSettingsOpen(store.settingsOpen)
      setBackendOnline(store.backendOnline)
    })
    return unsub
  }, [])

  /* Hydrate from the InferGlow backend: health → agents → sessions. */
  useEffect(() => {
    void bootstrap()
  }, [])

  /* Apply initial theme */
  useEffect(() => {
    if (store.settings.darkMode) {
      document.body.setAttribute('data-ds-dark-theme', '')
    }
  }, [])

  function handleOpenSettings() {
    store.openSettings()
    setSettingsOpen(true)
  }

  function handleCloseSettings() {
    store.closeSettings()
    setSettingsOpen(false)
  }

  /* ── Details column resize (drag the seam handle) ── */
  const resizeStart = useRef<{ x: number; w: number } | null>(null)

  function onResizeStart(e: ReactPointerEvent) {
    e.preventDefault()
    resizeStart.current = { x: e.clientX, w: detailsWidth }
    document.addEventListener('pointermove', onResizeMove)
    document.addEventListener('pointerup', onResizeEnd)
  }
  function onResizeMove(e: PointerEvent) {
    if (!resizeStart.current) return
    // Details column sits at the right; dragging left enlarges it.
    const delta = resizeStart.current.x - e.clientX
    const next = Math.min(DETAILS_MAX, Math.max(DETAILS_MIN, resizeStart.current.w + delta))
    setDetailsWidth(next)
  }
  function onResizeEnd() {
    resizeStart.current = null
    document.removeEventListener('pointermove', onResizeMove)
    document.removeEventListener('pointerup', onResizeEnd)
  }

  /* The two panel toggles show whenever the right column is closed; once the
   * right column opens it manages itself (merged top bar). */
  const togglesVisible = !detailsOpen
  const convSession = store.sessions.find(s => s.id === activeSessionId)
  const conv = !!(convSession && convSession.messages.length > 0)
  const convTitle = convSession?.title ?? ''

  return (
    <div className="dsh-app">
      <div className="dsh-workspace">
        {/* ── Left: workspace sidebar ── */}
        <aside className="dsh-workspace-sidebar">
          <Sidebar onOpenSettings={handleOpenSettings} />
        </aside>

        {/* ── Middle: topbar + scroll body + bottom panel ── */}
        <main className="dsh-workspace-main">
          <header className="dsh-topbar">
            {conv && (
              <div className="dsh-topbar-conv">
                <span className="dsh-conv-crumb">{convTitle}</span>
                <span className="dsh-conv-sub">/ 1 个子代理</span>
                <span className="dsh-conv-preset">标准模式 (Windows)</span>
                <button type="button" className="dsh-conv-dag" aria-expanded="false">
                  任务 DAG <span className="dsh-conv-dag-count">1</span>
                </button>
              </div>
            )}
            <div className="dsh-topbar-right">
              {conv && <button type="button" className="dsh-conv-log">Session log</button>}
              <span
                title={backendOnline === null
                  ? '正在连接 InferGlow 后端…'
                  : backendOnline
                    ? 'InferGlow 后端已连接'
                    : 'InferGlow 后端不可达 — 无法加载会话,发送消息将失败'}
                style={{
                  width: 8, height: 8, borderRadius: '50%', flexShrink: 0, margin: '0 6px',
                  background: backendOnline === null ? '#8a8a8a' : backendOnline ? '#3fb96f' : '#d4544a',
                  boxShadow: '0 0 6px currentColor', opacity: 0.9,
                }}
              />
              {togglesVisible && (
                <div className="dsh-topbar-toggles">
                  <button
                    type="button"
                    className="dsh-topbar-toggle"
                    aria-label="展开底部面板"
                    title="展开底部面板"
                    onClick={() => setBottomOpen(o => !o)}
                  >
                    <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
                      <rect x="1.5" y="2" width="13" height="12" rx="2.5" stroke="currentColor" strokeWidth="1.5"/>
                      <rect x="3.25" y="10" width="9.5" height="2.75" rx="1" fill="currentColor" stroke="none"/>
                    </svg>
                  </button>
                  <button
                    type="button"
                    className="dsh-topbar-toggle"
                    aria-label="展开侧边栏"
                    title="展开侧边栏"
                    onClick={() => setDetailsOpen(o => !o)}
                  >
                    <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
                      <rect x="1.5" y="2" width="13" height="12" rx="2.5" stroke="currentColor" strokeWidth="1.5"/>
                      <rect x="10.5" y="3.25" width="2.75" height="9.5" rx="1" fill="currentColor" stroke="none"/>
                    </svg>
                  </button>
                </div>
              )}
            </div>
          </header>

          {conv && (
            <div className="dsh-conv-tabs" role="tablist">
              {['对话', '轨迹', '上下文'].map(tab => (
                <button
                  key={tab}
                  type="button"
                  role="tab"
                  aria-selected={convTab === tab}
                  className={`dsh-conv-tab${convTab === tab ? ' dsh-conv-tab-active' : ''}`}
                  onClick={() => setConvTab(tab)}
                >
                  {tab}
                </button>
              ))}
            </div>
          )}

          <div className="dsh-scroll-body">
            <ChatArea activeSessionId={activeSessionId} convTab={convTab} />
          </div>

          {bottomOpen && (
            <div className="dsh-bottom-panel">
              <TabbedPane defaultTabs={DEFAULT_BOTTOM_TABS} />
            </div>
          )}
        </main>

        {/* ── Details column resize handle (drag seam) ── */}
        {detailsOpen && (
          <div
            className="dsh-workspace-resize"
            title="拖动调整详情宽度"
            style={{ right: detailsWidth + 3 }}
            onPointerDown={onResizeStart}
          />
        )}

        {/* ── Right: details panel ── */}
        <DetailsPanel
          open={detailsOpen}
          width={detailsWidth}
          onToggle={() => setDetailsOpen(o => !o)}
          onOpenBottom={() => setBottomOpen(o => !o)}
        />

        {/* ── Overlay + drag handle ── */}
        <div className="dsh-workspace-overlay" />

        {settingsOpen && <SettingsModal onClose={handleCloseSettings} />}
      </div>
    </div>
  )
}