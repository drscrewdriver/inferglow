import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useSettingsStore } from './settings/settingsStore'
import { ThemeProvider } from './theme/ThemeProvider'
import { SettingsPanel } from './settings/SettingsPanel'
import { useSessionStore } from './stores/sessionStore'
import { useChatStore, type ChatMessage } from './stores/chatStore'

// ─── backend probe: live mode (REST wired) vs demo mode (backend down) ───
async function probeBackend(): Promise<boolean> {
  try {
    const resp = await fetch('/health', { signal: AbortSignal.timeout(1500) })
    return resp.ok
  } catch {
    return false
  }
}

function Sidebar() {
  const [filter, setFilter] = useState('')
  const sessions = useSessionStore((s) => s.sessions)
  const activeId = useSessionStore((s) => s.activeId)
  const select = useSessionStore((s) => s.select)
  const fetchSessions = useSessionStore((s) => s.fetchSessions)
  const createSession = useSessionStore((s) => s.createSession)
  const updateMeta = useSessionStore((s) => s.updateMeta)
  const removeSession = useSessionStore((s) => s.removeSession)

  useEffect(() => {
    void fetchSessions()
  }, [fetchSessions])

  const shown = useMemo(() => {
    const f = filter.toLowerCase()
    const groups = [...new Set(sessions.map((s) => s.group || '未归类'))]
    return groups.map((g) => {
      const items = sessions
        .filter((s) => (s.group || '未归类') === g && (f === '' || (s.title ?? '').toLowerCase().includes(f)))
        .sort((a, b) => Number(b.pinned) - Number(a.pinned))
      return { group: g, items }
    })
  }, [sessions, filter])

  const onNew = useCallback(async () => {
    const rec = await createSession('a1', `新会话 ${sessions.length + 1}`)
    if (rec) select(rec.id)
  }, [createSession, select, sessions.length])

  return (
    <aside className="sidebar">
      <div className="sidebar__head">
        <span className="t">会话</span>
        <button className="icon-btn" title="折叠侧栏" style={{ width: 24, height: 24, fontSize: 12 }}>◀</button>
      </div>
      <div className="sess-search">
        <span className="ic">🔍</span>
        <input placeholder="过滤会话…" value={filter} onChange={(e) => setFilter(e.target.value)} autoComplete="off" />
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
                  onClick={() => select(s.id)}
                  title={`${s.title ?? '未命名'} · 右键菜单：置顶/归档/删除`}
                  onContextMenu={(e) => {
                    e.preventDefault()
                    const action = window.prompt('操作：pin / unpin / archive / delete')
                    if (action === 'pin') void updateMeta(s.id, { pinned: true })
                    else if (action === 'unpin') void updateMeta(s.id, { pinned: false })
                    else if (action === 'archive') void updateMeta(s.id, { status: 'archived' })
                    else if (action === 'delete') void removeSession(s.id)
                  }}
                >
                  <span className={`s-avatar${s.group ? ' proj' : ''}`}>{(s.title ?? '话').trim().charAt(0)}</span>
                  <div className="sess-item__body">
                    <div className="sess-item__t">{s.title ?? '未命名会话'}</div>
                    <div className="sess-item__m">{s.agent_id ?? ''}</div>
                  </div>
                  {s.pinned && <span className="sess-item__pin">📌</span>}
                  <span className={`sess-item__scope${s.group ? ' sess-item__scope--proj' : ''}`}>{s.group || '未归类'}</span>
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
        <button className="btn btn--primary btn--block" onClick={() => void onNew()}>＋ 新建会话</button>
      </div>
    </aside>
  )
}

function ToolCardView({ m, onApprove, onReject }: { m: ChatMessage; onApprove?: () => void; onReject?: () => void }) {
  const status = m.toolStatus ?? 'run'
  const statusIcon = status === 'run' ? '⟳' : status === 'ok' ? '✓' : '✕'
  return (
    <div className={`tool-card${m.toolStatus === 'run' ? ' tool-card--open' : ''}`}>
      <div className="tool-card__head">
        <span className={`tool-status tool-status--${status}`}>{statusIcon}</span>
        <span className="tool-card__name">{m.toolName ?? 'tool'}</span>
        <span className="tool-card__tail">
          {status === 'run' && onApprove && (
            <>
              <button className="btn btn--small" onClick={onApprove}>允许</button>
              <button className="btn btn--small btn--danger" onClick={onReject}>拒绝</button>
            </>
          )}
          <span className="tool-card__dur">{status === 'run' ? '运行中…' : status === 'ok' ? '完成' : '失败'}</span>
          <span className="tool-card__chev">▾</span>
        </span>
      </div>
      {m.content && (
        <div className="tool-card__out" style={{ display: 'block' }}>
          <pre className="tool-out">{m.content}</pre>
        </div>
      )}
    </div>
  )
}

function MessageList({
  messages,
  streaming,
  thoughtVisible,
  onScrollTop,
  onApproveTool,
  onRejectTool,
}: {
  messages: ChatMessage[]
  streaming: boolean
  thoughtVisible: boolean
  onScrollTop?: () => void
  onApproveTool?: () => void
  onRejectTool?: () => void
}) {
  const ref = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (ref.current) ref.current.scrollTop = ref.current.scrollHeight
  }, [messages.length, streaming])
  return (
    <div
      className="messages"
      ref={ref}
      onScroll={(e) => {
        if (e.currentTarget.scrollTop < 80) onScrollTop?.()
      }}
    >
      {messages.map((m) =>
        m.role === 'tool' ? (
          <div key={m.id} className="msg msg--a">
            <span className="msg__avatar">⚙</span>
            <div className="msg__body" style={{ background: 'transparent', border: 'none', maxWidth: 760 }}>
              <ToolCardView m={m} onApprove={onApproveTool} onReject={onRejectTool} />
            </div>
          </div>
        ) : (
          <div key={m.id} className={`msg ${m.role === 'user' ? 'msg--u' : 'msg--a'}`}>
            <span className="msg__avatar">{m.role === 'user' ? '我' : '◈'}</span>
            <div className="msg__body">
              <div style={{ whiteSpace: 'pre-wrap' }}>{m.content || (m.role === 'assistant' ? <span className="caret" /> : '')}</div>
            </div>
          </div>
        ),
      )}
      {thoughtVisible && <div className="thought">正在思考…</div>}
      {messages.length === 0 && !thoughtVisible && (
        <div style={{ display: 'grid', placeItems: 'center', height: '100%', color: 'var(--fg-faint)', fontSize: 13 }}>
          选择左侧会话开始对话，或新建一个会话。
        </div>
      )}
    </div>
  )
}

function Composer({ disabled }: { disabled: boolean }) {
  const [text, setText] = useState('')
  const sendMessage = useChatStore((s) => s.sendMessage)
  const streaming = useChatStore((s) => s.streaming)
  const stop = useChatStore((s) => s.stop)
  const activeId = useSessionStore((s) => s.activeId)
  const sessions = useSessionStore((s) => s.sessions)
  const agentId = sessions.find((s) => s.id === activeId)?.agent_id ?? 'a1'

  const send = useCallback(() => {
    const msg = text.trim()
    if (!msg || !activeId || streaming) return
    setText('')
    void sendMessage(activeId, agentId, msg)
  }, [text, activeId, agentId, streaming, sendMessage])

  return (
    <div className="composer-wrap">
      <div className="composer">
        <textarea
          placeholder={disabled ? '后端未连接 · Demo 模式（消息不会发送）' : '输入消息，Enter 发送，Shift+Enter 换行…'}
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
              e.preventDefault()
              send()
            }
          }}
          disabled={disabled}
        />
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
          {streaming && (
            <button className="btn btn--small btn--danger" onClick={stop}>■ 停止</button>
          )}
          <button className="send-btn" title="发送" onClick={send} disabled={disabled || !text.trim()}>➤</button>
        </div>
        <div className="composer__foot">
          <span className="ctx-chip">上下文 128k</span>
        </div>
      </div>
    </div>
  )
}

function ChatMain({ live }: { live: boolean }) {
  const activeId = useSessionStore((s) => s.activeId)
  const sessions = useSessionStore((s) => s.sessions)
  const active = sessions.find((s) => s.id === activeId)
  const agentId = active?.agent_id ?? 'a1'
  const messages = useChatStore((s) => s.messages)
  const error = useChatStore((s) => s.error)
  const streaming = useChatStore((s) => s.streaming)
  const running = useChatStore((s) => s.running)
  const loadHistory = useChatStore((s) => s.loadHistory)
  const appendHistory = useChatStore((s) => s.appendHistory)
  const loadOlder = useChatStore((s) => s.loadOlder)
  const clear = useChatStore((s) => s.clear)
  const approveInput = useChatStore((s) => s.approveInput)
  const cmdApproval = useSettingsStore((s) => s.settings.cmdApproval)

  // Load history when switching sessions; clear local state otherwise.
  useEffect(() => {
    if (!activeId) {
      clear()
      return
    }
    clear()
    void loadHistory(activeId).then((page) => {
      if (page) appendHistory(page)
    })
  }, [activeId, clear, loadHistory, appendHistory])

  return (
    <main className="main">
      <div className="chat-head">
        <span className="t">{active?.title ?? '未选择会话'}</span>
        <span className="model">{active?.agent_id ?? 'agent'} · {live ? 'live' : 'demo'}</span>
        <span className="spacer" />
      </div>
      <MessageList
        messages={messages}
        streaming={streaming}
        thoughtVisible={streaming && running}
        onScrollTop={activeId ? () => void loadOlder(activeId) : undefined}
        onApproveTool={cmdApproval ? () => void approveInput(agentId, true) : undefined}
        onRejectTool={cmdApproval ? () => void approveInput(agentId, false) : undefined}
      />
      {error && (
        <div style={{ padding: '4px 16px', fontSize: 12, color: 'var(--err)', background: 'rgba(229,83,75,.08)' }}>
          {error}
        </div>
      )}
      <Composer disabled={!live} />
    </main>
  )
}

export default function App() {
  const settings = useSettingsStore((s) => s.settings)
  const sessionCount = useSessionStore((s) => s.sessions.length)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [termOpen, setTermOpen] = useState(false)
  const [live, setLive] = useState(false)
  const [liveChecked, setLiveChecked] = useState(false)

  useEffect(() => {
    void probeBackend().then((ok) => {
      setLive(ok)
      setLiveChecked(true)
    })
  }, [])

  return (
    <ThemeProvider themeKey={settings.themeKey}>
      <div className="app-root">
        <header className="topbar">
          <div className="brand"><span className="logo">◈</span>Infer<b>Glow</b><span className="ver">v0.2.0</span></div>
          <span className="status"><span className="dot" />{live ? '运行中' : '未连接'}</span>
          <div className="channel-tabs"><button className="channel-tab active">聊天</button><button className="channel-tab">频道</button></div>
          <span className="spacer" />
          <button className="cmdk-btn">🔍 搜索命令、会话、设置… <span className="k">⌘ K</span></button>
          <button className="icon-btn" title="目标模式">🎯</button>
          <button className="icon-btn" title="设置" onClick={() => setSettingsOpen(true)}>⚙</button>
        </header>

        {liveChecked && !live && (
          <div className="demo-note">
            <span>⚠️</span>
            <span><b>Demo 模式</b> · 后端未连接（请先启动 <code style={{ fontFamily: 'var(--font-mono)' }}>inferglow-server</code>），当前界面仅展示布局。</span>
            <span className="close" title="关闭">✕</span>
          </div>
        )}

        <div className="shell">
          <Sidebar />
          <ChatMain live={live} />
          <aside className="dock">
            <div className="dock__head"><span className="t">面板</span><button className="icon-btn" title="折叠 dock" style={{ width: 24, height: 24, fontSize: 12 }}>▶</button></div>
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
                      <div className="center"><span className="pct">0%</span><span className="lbl">已用</span></div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </aside>
        </div>

        <footer className="statusbar">
          <span className="cell">◈ <b>InferGlow</b></span>
          <span className="cell">模型 <b style={{ color: 'var(--accent)' }}>deepseek-chat</b></span>
          <span className="cell">会话 <b>{sessionCount}</b></span>
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
