import { useMemo, useState } from 'react'
import { useSettingsStore } from './settings/settingsStore'
import { ThemeProvider } from './theme/ThemeProvider'
import { SettingsPanel } from './settings/SettingsPanel'

// ─── demo session data (replaced by GET /v1/sessions in a later commit) ───
interface DemoSession {
  id: number
  title: string
  group: string
  pinned: boolean
  rounds: number
  model: string
}

const DEMO_SESSIONS: DemoSession[] = [
  { id: 1, title: '并行子智能体检索发布说明', group: '项目', pinned: true, rounds: 5, model: 'deepseek-reasoner' },
  { id: 2, title: '启用服务端网页搜索', group: '项目', pinned: false, rounds: 2, model: 'deepseek-chat' },
  { id: 3, title: '重构终端会话轨', group: '项目', pinned: false, rounds: 8, model: 'deepseek-chat' },
  { id: 4, title: '目标模式研究', group: '项目', pinned: true, rounds: 12, model: 'deepseek-reasoner' },
  { id: 5, title: 'Global 会话', group: '未归类', pinned: false, rounds: 1, model: 'deepseek-chat' },
  { id: 6, title: '凭证排查', group: '未归类', pinned: false, rounds: 3, model: 'deepseek-chat' },
]

function Sidebar({
  sessions,
  activeId,
  onSelect,
  onNew,
}: {
  sessions: DemoSession[]
  activeId: number
  onSelect: (id: number) => void
  onNew: () => void
}) {
  const [filter, setFilter] = useState('')
  const shown = useMemo(() => {
    const groups = ['项目', '未归类']
    const f = filter.toLowerCase()
    return groups.map((g) => {
      const items = sessions
        .filter((s) => s.group === g && (f === '' || s.title.toLowerCase().includes(f)))
        .sort((a, b) => Number(b.pinned) - Number(a.pinned))
      return { group: g, items }
    })
  }, [sessions, filter])

  return (
    <aside className="sidebar">
      <div className="sidebar__head">
        <span className="t">会话</span>
        <button className="icon-btn" title="折叠侧栏" style={{ width: 24, height: 24, fontSize: 12 }}>◀</button>
      </div>
      <div className="sess-search">
        <span className="ic">🔍</span>
        <input
          placeholder="过滤会话…"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          autoComplete="off"
        />
      </div>
      <div className="sess-list">
        {shown.map(({ group, items }) =>
          items.length === 0 ? null : (
            <div key={group}>
              <div className="sess-group__label">{group}<span className="cnt">{items.length}</span></div>
              {items.map((s) => (
                <div
                  key={s.id}
                  className={`sess-item${s.id === activeId ? ' sess-item--active' : ''}`}
                  onClick={() => onSelect(s.id)}
                >
                  <span className={`s-avatar${s.group === '项目' ? ' proj' : ''}`}>{s.title.trim().charAt(0)}</span>
                  <div className="sess-item__body">
                    <div className="sess-item__t">{s.title}</div>
                    <div className="sess-item__m">{s.rounds} 轮 · {s.model}</div>
                  </div>
                  {s.pinned && <span className="sess-item__pin">📌</span>}
                  <span className={`sess-item__scope${s.group === '项目' ? ' sess-item__scope--proj' : ''}`}>{s.group}</span>
                </div>
              ))}
            </div>
          ),
        )}
        {shown.every(({ items }) => items.length === 0) && (
          <div style={{ padding: 20, textAlign: 'center', color: 'var(--fg-faint)', fontSize: 12 }}>无匹配会话</div>
        )}
      </div>
      <div className="sidebar__foot">
        <button className="btn btn--primary btn--block" onClick={onNew}>＋ 新建会话</button>
      </div>
    </aside>
  )
}

function ChatMain({ active }: { active: DemoSession | undefined }) {
  return (
    <main className="main">
      <div className="chat-head">
        <span className="t">{active?.title ?? '未选择会话'}</span>
        <span className="model">{active?.model ?? 'deepseek-chat'} · reasoner</span>
        <span className="spacer" />
      </div>
      <div className="messages" style={{ display: 'grid', placeItems: 'center', color: 'var(--fg-faint)', fontSize: 13 }}>
        选择左侧会话开始对话，或新建一个会话。
      </div>
      <div className="composer-wrap">
        <div className="composer">
          <textarea placeholder="输入消息，Enter 发送，Shift+Enter 换行…" />
          <div className="composer__bar">
            <span className="seg__lbl">思考</span>
            <div className="seg">
              <button className="seg__btn">低</button>
              <button className="seg__btn">中</button>
              <button className="seg__btn seg__btn--on">高</button>
            </div>
            <span className="seg__lbl" style={{ marginLeft: 8 }}>权限</span>
            <div className="seg">
              <button className="seg__btn">自动</button>
              <button className="seg__btn seg__btn--on">询问</button>
              <button className="seg__btn">计划</button>
            </div>
            <span className="composer__spacer" />
            <button className="send-btn" title="发送">➤</button>
          </div>
          <div className="composer__foot">
            <span className="ctx-chip">上下文 128k</span>
            <span className="ctx-chip">已用 22k (17%)</span>
          </div>
        </div>
      </div>
    </main>
  )
}

function Dock() {
  return (
    <aside className="dock">
      <div className="dock__head">
        <span className="t">面板</span>
        <button className="icon-btn" title="折叠 dock" style={{ width: 24, height: 24, fontSize: 12 }}>▶</button>
      </div>
      <div className="dock__body">
        <div className="dock-panel">
          <div className="dock-panel__head"><span>◎</span><span className="t">上下文窗口</span><span className="chev">▾</span></div>
          <div className="dock-panel__body">
            <div className="ring-wrap">
              <div className="ring">
                <svg width="96" height="96">
                  <defs>
                    <linearGradient id="ringGrad" x1="0" y1="0" x2="1" y2="1">
                      <stop id="rg1" offset="0%" stopColor="var(--accent)" />
                      <stop id="rg2" offset="100%" stopColor="var(--accent-strong)" />
                    </linearGradient>
                  </defs>
                  <circle cx="48" cy="48" r="38" fill="none" stroke="var(--bg-elev-2)" strokeWidth="8" />
                  <circle cx="48" cy="48" r="38" fill="none" stroke="url(#ringGrad)" strokeWidth="8" strokeLinecap="round" strokeDasharray="238.8" strokeDashoffset="143.3" />
                </svg>
                <div className="center"><span className="pct">40%</span><span className="lbl">已用</span></div>
              </div>
              <div className="ring-legend">
                <div className="lr"><span>窗口</span><b>128k</b></div>
                <div className="lr"><span>已用</span><b>51.2k</b></div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </aside>
  )
}

export default function App() {
  const settings = useSettingsStore((s) => s.settings)
  const [activeId, setActiveId] = useState(1)
  const [sessions, setSessions] = useState(DEMO_SESSIONS)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [termOpen, setTermOpen] = useState(false)
  const active = sessions.find((s) => s.id === activeId)

  return (
    <ThemeProvider themeKey={settings.themeKey}>
      <div className="app-root">
        <header className="topbar">
          <div className="brand"><span className="logo">◈</span>Infer<b>Glow</b><span className="ver">v0.2.0</span></div>
          <span className="status"><span className="dot" />运行中</span>
          <div className="channel-tabs"><button className="channel-tab active">聊天</button><button className="channel-tab">频道</button></div>
          <span className="spacer" />
          <button className="cmdk-btn">🔍 搜索命令、会话、设置… <span className="k">⌘ K</span></button>
          <button className="icon-btn" title="目标模式">🎯</button>
          <button className="icon-btn" title="设置" onClick={() => setSettingsOpen(true)}>⚙</button>
        </header>

        <div className="shell">
          <Sidebar
            sessions={sessions}
            activeId={activeId}
            onSelect={setActiveId}
            onNew={() => {
              const id = Date.now()
              setSessions((prev) => [...prev, { id, title: `新会话 ${prev.length + 1}`, group: '未归类', pinned: false, rounds: 0, model: 'deepseek-chat' }])
              setActiveId(id)
            }}
          />
          <ChatMain active={active} />
          <Dock />
        </div>

        <footer className="statusbar">
          <span className="cell">◈ <b>InferGlow</b></span>
          <span className="cell">模型 <b style={{ color: 'var(--accent)' }}>deepseek-chat</b></span>
          <span className="cell">会话 <b>{sessions.length}</b></span>
          <span className="spacer" />
          <span className="cell">模式 <b>普通</b></span>
          <button className="term-toggle" onClick={() => setTermOpen((v) => !v)}>▾ 终端</button>
        </footer>

        {termOpen && (
          <div className="term-drawer">
            <div className="term-drawer__inner">
              <div className="term-rails" />
              <div className="term-view" />
              <div className="term-input"><span className="prompt">sandbox$</span><input placeholder="输入命令，Enter 执行…" autoComplete="off" spellCheck={false} /></div>
            </div>
          </div>
        )}

        {settingsOpen && <SettingsPanel onClose={() => setSettingsOpen(false)} />}
      </div>
    </ThemeProvider>
  )
}
