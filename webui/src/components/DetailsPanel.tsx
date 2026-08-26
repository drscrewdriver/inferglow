/* InferGlow Web UI — 右侧详情面板 */

import type { Session } from '../api/types'

interface DetailsPanelProps {
  session: Session | null
  onClose: () => void
}

export function DetailsPanel({ session, onClose }: DetailsPanelProps) {
  return (
    <aside className="details-panel">
      <div className="details-header">
        <span className="details-title">详情</span>
        <button className="details-close" onClick={onClose}>×</button>
      </div>
      <div className="details-body">
        {!session ? (
          <div style={{ color: 'var(--text-faint)', fontSize: 13, textAlign: 'center', marginTop: 40 }}>
            选择会话查看详情
          </div>
        ) : (
          <div style={{ fontSize: 13, lineHeight: 1.8 }}>
            <div style={{ marginBottom: 16 }}>
              <div style={{ color: 'var(--text-faint)', fontSize: 11, marginBottom: 4 }}>会话</div>
              <div style={{ fontWeight: 600 }}>{session.title || '未命名'}</div>
            </div>
            <div style={{ marginBottom: 16 }}>
              <div style={{ color: 'var(--text-faint)', fontSize: 11, marginBottom: 4 }}>Agent</div>
              <div>{session.agent_id}</div>
            </div>
            <div style={{ marginBottom: 16 }}>
              <div style={{ color: 'var(--text-faint)', fontSize: 11, marginBottom: 4 }}>创建时间</div>
              <div>{new Date(session.created_at).toLocaleString('zh-CN')}</div>
            </div>
            {session.group && (
              <div style={{ marginBottom: 16 }}>
                <div style={{ color: 'var(--text-faint)', fontSize: 11, marginBottom: 4 }}>分组</div>
                <div>{session.group}</div>
              </div>
            )}
          </div>
        )}
      </div>
    </aside>
  )
}
