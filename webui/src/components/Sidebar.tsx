/* InferGlow Web UI — 侧栏（工作区 + 会话树） */

import { useState } from 'react'
import type { Session } from '../api/types'
import { SidebarEnhance, BrandRow } from '../plugin/sidebar-enhance'

interface SidebarProps {
  sessions: Session[]
  activeSessionId: string | null
  onSelectSession: (id: string) => void
  connected: boolean
}

export function Sidebar({ sessions, activeSessionId, onSelectSession, connected }: SidebarProps) {
  const [query, setQuery] = useState('')

  const filtered = sessions.filter(s =>
    s.title.toLowerCase().includes(query.toLowerCase())
  )

  // 按 group 分组
  const groups = new Map<string, Session[]>()
  for (const s of filtered) {
    const g = s.group ?? '默认'
    if (!groups.has(g)) groups.set(g, [])
    groups.get(g)!.push(s)
  }

  return (
    <SidebarEnhance
      header={<BrandRow />}
      rail={<BrandRow collapsed />}
    >
      <div className="sidebar-header">
        <button className="new-session-btn" disabled={!connected}>
          + 新会话
        </button>
      </div>

      <div className="sidebar-search">
        <input
          className="search-input"
          placeholder="搜索会话..."
          value={query}
          onChange={e => setQuery(e.target.value)}
        />
      </div>

      <div className="sidebar-list">
        {filtered.length === 0 && (
          <div style={{ padding: '16px', color: 'var(--text-faint)', fontSize: 12, textAlign: 'center' }}>
            {connected ? '暂无会话' : '未连接后端'}
          </div>
        )}

        {Array.from(groups.entries()).map(([group, items]) => (
          <div key={group}>
            <div className="session-group-head">
              {group}
            </div>
            {items.map(s => (
              <div
                key={s.id}
                className={`session-item ${s.id === activeSessionId ? 'active' : ''}`}
                onClick={() => onSelectSession(s.id)}
              >
                <span className="session-item-title">{s.title || '未命名会话'}</span>
                <span className="session-item-time">
                  {formatRelativeTime(s.updated_at)}
                </span>
              </div>
            ))}
          </div>
        ))}
      </div>
    </SidebarEnhance>
  )
}

function formatRelativeTime(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime()
  const mins = Math.floor(diff / 60000)
  if (mins < 1) return '刚刚'
  if (mins < 60) return `${mins}分钟`
  const hours = Math.floor(mins / 60)
  if (hours < 24) return `${hours}小时`
  const days = Math.floor(hours / 24)
  return `${days}天`
}
