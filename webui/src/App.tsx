/* InferGlow Web UI — 主应用（参考 DeepSeek Harness 布局） */

import { useEffect, useState } from 'react'
import { healthCheck, get } from './api/transport'
import type { Session, Agent } from './api/types'
import { Sidebar } from './components/Sidebar'
import { ChatArea } from './components/ChatArea'
import { DetailsPanel } from './components/DetailsPanel'
import { StatusBar } from './components/StatusBar'
import './App.css'

type Tab = 'chat' | 'trace' | 'context'

export default function App() {
  const [connected, setConnected] = useState(false)
  const [activeTab, setActiveTab] = useState<Tab>('chat')
  const [sessions, setSessions] = useState<Session[]>([])
  const [activeSessionId, setActiveSessionId] = useState<string | null>(null)
  const [agents, setAgents] = useState<Agent[]>([])
  const [showDetails, setShowDetails] = useState(true)

  // 健康检查 + 加载初始数据
  useEffect(() => {
    let cancelled = false
    async function init() {
      const ok = await healthCheck()
      if (cancelled) return
      setConnected(ok)
      if (!ok) return

      try {
        const [sessRes, agentRes] = await Promise.all([
          get<{ sessions: Session[] }>('/v1/sessions'),
          get<{ agents: Agent[] }>('/v1/agents'),
        ])
        if (!cancelled) {
          setSessions(sessRes.sessions ?? [])
          setAgents(agentRes.agents ?? [])
        }
      } catch {
        // 静默降级
      }
    }
    init()
    return () => { cancelled = true }
  }, [])

  const activeSession = sessions.find(s => s.id === activeSessionId) ?? null
  const activeAgent = agents.find(a => a.id === activeSession?.agent_id) ?? null

  return (
    <div className="app-root">
      {/* 顶栏 */}
      <header className="topbar">
        <div className="topbar-left">
          <span className="logo">InferGlow</span>
          <span className="logo-tag">WEB</span>
        </div>
        <nav className="topbar-tabs">
          {(['chat', 'trace', 'context'] as Tab[]).map(tab => (
            <button
              key={tab}
              className={`tab-btn ${activeTab === tab ? 'active' : ''}`}
              onClick={() => setActiveTab(tab)}
            >
              {tab === 'chat' ? '对话' : tab === 'trace' ? '轨迹' : '上下文'}
            </button>
          ))}
        </nav>
        <div className="topbar-right">
          {!connected && <span className="status-dot disconnected" title="未连接后端" />}
          <span className="session-log-btn">Session log</span>
        </div>
      </header>

      {/* 主体三栏 */}
      <div className="main-body">
        <Sidebar
          sessions={sessions}
          activeSessionId={activeSessionId}
          onSelectSession={setActiveSessionId}
          connected={connected}
        />

        <ChatArea
          session={activeSession}
          agent={activeAgent}
          connected={connected}
          activeTab={activeTab}
        />

        {showDetails && (
          <DetailsPanel
            session={activeSession}
            onClose={() => setShowDetails(false)}
          />
        )}
      </div>

      {/* 状态栏 */}
      <StatusBar connected={connected} session={activeSession} />
    </div>
  )
}
