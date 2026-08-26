/* InferGlow Web UI — 底部状态栏 */

import type { Session } from '../api/types'

interface StatusBarProps {
  connected: boolean
  session: Session | null
}

export function StatusBar({ connected, session }: StatusBarProps) {
  return (
    <footer className="statusbar">
      <div className="statusbar-item">
        <span className={`statusbar-dot ${connected ? 'ok' : 'err'}`} />
        {connected ? '已连接' : '未连接'}
      </div>
      {session && (
        <>
          <div className="statusbar-item">会话: {session.title || session.id.slice(0, 8)}</div>
          <div className="statusbar-item">Agent: {session.agent_id}</div>
        </>
      )}
      <div style={{ flex: 1 }} />
      <div className="statusbar-item">InferGlow Web UI v0.1.0</div>
    </footer>
  )
}
