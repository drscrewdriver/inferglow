/**
 * WorkspaceAddModal — register a new workspace by name + absolute root path.
 * Browsers cannot hand out absolute paths from native pickers (webkitdirectory
 * yields relative paths only), so the path is typed and validated server-side
 * by POST /v1/workspaces (Open rejects non-existent roots).
 */

import { useState } from 'react'
import { Button } from '../components/Button.tsx'
import { createWorkspace } from '../bridge/inferglow.ts'

interface WorkspaceAddModalProps {
  onClose: () => void
  onCreated?: (name: string) => void
}

export function WorkspaceAddModal({ onClose, onCreated }: WorkspaceAddModalProps) {
  const [name, setName] = useState('')
  const [root, setRoot] = useState('')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState<string | null>(null)

  async function submit() {
    const n = name.trim()
    const r = root.trim()
    if (!n || !r || busy) return
    setBusy(true)
    setErr(null)
    try {
      await createWorkspace(n, r)
      onCreated?.(n)
      onClose()
    } catch (e) {
      setErr(String((e as Error)?.message ?? e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="dsh-modal-overlay" onClick={onClose}>
      <div className="dsh-modal" style={{ width: 420 }} onClick={e => e.stopPropagation()}>
        <div className="dsh-modal-header">
          <h2 className="dsh-modal-title">添加工作区</h2>
          <Button variant="ghost" size="sm" onClick={onClose} title="关闭">
            <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
              <path d="M4 4l8 8M12 4l-8 8" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/>
            </svg>
          </Button>
        </div>
        <div className="dsh-modal-content">
          <section className="dsh-modal-section">
            <div className="dsh-modal-field">
              <label className="dsh-modal-label">名称</label>
              <input
                type="text"
                className="dsh-modal-input"
                value={name}
                onChange={e => setName(e.target.value)}
                placeholder="例如 rewrite-agently"
                autoFocus
              />
            </div>
            <div className="dsh-modal-field">
              <label className="dsh-modal-label">绝对路径</label>
              <input
                type="text"
                className="dsh-modal-input"
                value={root}
                onChange={e => setRoot(e.target.value)}
                placeholder="例如 E:\test\rewrite-agently"
              />
              <span className="dsh-modal-label" style={{ opacity: 0.6, fontSize: 12 }}>
                浏览器无法选择服务器侧绝对路径,请直接输入;server 会校验目录存在
              </span>
            </div>
            {err && <div className="dsh-modal-label" style={{ color: '#d4544a', fontSize: 12 }}>⚠ {err}</div>}
            <div className="dsh-modal-row" style={{ justifyContent: 'flex-end', gap: 8 }}>
              <Button variant="ghost" size="sm" onClick={onClose}>取消</Button>
              <Button
                variant="primary"
                size="sm"
                onClick={() => void submit()}
                disabled={busy || !name.trim() || !root.trim()}
              >
                {busy ? '注册中…' : '注册工作区'}
              </Button>
            </div>
          </section>
        </div>
      </div>
    </div>
  )
}
