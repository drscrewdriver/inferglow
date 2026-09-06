/* InferGlow Web UI — 主应用（参考 DeepSeek Harness 布局）

   集成 DSH 插件生态：
   - ThemeRuntime 挂载暗/亮/跟随系统三态 + 社区主题叠加
   - 注册 5 个核心插件（input-traffic / thinking-levels / sidebar / @file / theme）
   - 设置面板渲染 `settings.general.item`（外观）+ `settings.plugin.item`（思考级）
   */

import { useEffect, useState } from 'react'
import { healthCheck, get, post } from './api/transport'
import type { Session, Agent } from './api/types'
import { Sidebar } from './components/Sidebar'
import { ChatArea } from './components/ChatArea'
import { DetailsPanel } from './components/DetailsPanel'
import { StatusBar } from './components/StatusBar'
import { SettingsPanel } from './components/SettingsPanel'
import { ThemeRuntime } from './plugin/theme/ThemeRuntime'
import { registerThemePlugin, initCommunityTheme } from './plugin/theme'
import { registerInputTrafficPlugin, setChatAdapter, useTrafficStore } from './plugin/input-traffic'
import { registerThinkingLevelsPlugin } from './plugin/thinking-levels'
import { registerSidebarEnhancePlugin } from './plugin/sidebar-enhance'
import { registerAtFilePlugin } from './plugin/at-file'
import './App.css'

type Tab = 'chat' | 'trace' | 'context'

/** 读取运行时注入的配置（如 dshTheme），缺省走空。 */
declare global {
  interface Window {
    __IGW_CONFIG__?: { dshTheme?: string | null }
  }
}

export default function App() {
  const [connected, setConnected] = useState(false)
  const [activeTab, setActiveTab] = useState<Tab>('chat')
  const [sessions, setSessions] = useState<Session[]>([])
  const [activeSessionId, setActiveSessionId] = useState<string | null>(null)
  const [agents, setAgents] = useState<Agent[]>([])
  const [showDetails, setShowDetails] = useState(true)
  const [showSettings, setShowSettings] = useState(false)

  // 挂载主题运行组件并注册全部插件（幂等，热重载安全）。
  useEffect(() => {
    const unregs = [
      registerThemePlugin(),
      registerInputTrafficPlugin(),
      registerThinkingLevelsPlugin(),
      registerSidebarEnhancePlugin(),
      registerAtFilePlugin(),
    ]
    initCommunityTheme(window.__IGW_CONFIG__?.dshTheme ?? null)
    return () => { unregs.forEach((u) => u()) }
  }, [])

  // 接入流量队列的聊天适配器（让 SteerQueueDock 的 flush / steers / FreezeButton 恢复真正发送）。
  useEffect(() => {
    setChatAdapter({
      chatSend: (sessionId, agentId, text) =>
        post(`/v1/agents/${agentId}/chat`, { message: text, session_id: sessionId }),
      chatStop: () => {
        try { void post('/v1/runs/stop', {}).catch(() => {}) } catch { /* best effort */ }
      },
    })
  }, [])

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

  // 同步流量队列的会话/智能体上下文（供 dock/freeze 使用）。
  useEffect(() => {
    useTrafficStore.getState().setSession(activeSessionId)
  }, [activeSessionId])

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
          <button
            className="settings-trigger-btn"
            onClick={() => setShowSettings(true)}
            title="设置（外观 / 思考级插件）"
          >
            ⚙ 设置
          </button>
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

      {/* 主题运行组件（写入 CSS 变量 + data-theme 切换） */}
      <ThemeRuntime />

      {/* 设置面板 */}
      {showSettings && <SettingsPanel onClose={() => setShowSettings(false)} />}
    </div>
  )
}