/**
 * AgentPicker — model/agent selection popover for the composer chip and the
 * hero agent button. Re-fetches GET /v1/agents on every open so the list is
 * always in sync with the server config (llm.providers → config agent store).
 * Selecting applies immediately via settings.model (the send path reads it).
 */

import { useEffect, useState } from 'react'
import { store } from '../store.ts'
import { refreshAgents } from '../bridge/inferglow.ts'
import type { Agent } from '../api/types.ts'

export function AgentPicker({ onClose }: { onClose: () => void }) {
  const [list, setList] = useState<Agent[] | null>(null) // null = loading

  // 每次打开重拉 — 与 server 配置保持同步(config 改后重启即反映)。
  useEffect(() => {
    let alive = true
    void refreshAgents().then(a => {
      if (alive) setList(a)
    })
    return () => { alive = false }
  }, [])

  const current = store.settings.model

  function pick(id: string) {
    store.updateSetting('model', id)
    onClose()
  }

  return (
    <div
      role="listbox"
      aria-label="选择 Agent / 模型"
      style={{
        position: 'absolute', bottom: 'calc(100% + 6px)', right: 0, zIndex: 60,
        minWidth: 260, maxWidth: 340, maxHeight: 320, overflowY: 'auto',
        background: 'var(--dsw-surface-raised, #1e2433)', color: 'inherit',
        border: '1px solid color-mix(in srgb, currentColor 18%, transparent)',
        borderRadius: 10, boxShadow: '0 8px 24px rgba(0,0,0,.35)', padding: 4,
      }}
    >
      {list === null && <div style={{ padding: '8px 10px', opacity: 0.7, fontSize: 12 }}>加载中…</div>}
      {list !== null && list.length === 0 && (
        <div style={{ padding: '10px 12px', fontSize: 12, opacity: 0.75, lineHeight: 1.6 }}>
          后端没有可用 Agent。
          <br />真实模型:在 YAML 的 llm.providers 配置后以 -config 启动;
          <br />本地体验:-demo-agent(echo)。
        </div>
      )}
      {list?.map(a => {
        const active = a.id === current
        return (
          <button
            key={a.id}
            type="button"
            role="option"
            aria-selected={active}
            onClick={() => pick(a.id)}
            style={{
              display: 'block', width: '100%', textAlign: 'left', cursor: 'pointer',
              padding: '8px 10px', margin: 0, border: 'none', borderRadius: 8,
              background: active ? 'color-mix(in srgb, #4f7cff 22%, transparent)' : 'transparent',
              color: 'inherit', font: 'inherit',
            }}
            onMouseEnter={e => { if (!active) (e.currentTarget as HTMLButtonElement).style.background = 'color-mix(in srgb, currentColor 8%, transparent)' }}
            onMouseLeave={e => { if (!active) (e.currentTarget as HTMLButtonElement).style.background = 'transparent' }}
          >
            <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8, alignItems: 'baseline' }}>
              <b style={{ fontSize: 13 }}>{a.name}</b>
              {active && <span style={{ fontSize: 11, color: '#4f7cff' }}>当前</span>}
            </div>
            <div style={{ fontSize: 11, opacity: 0.65, marginTop: 2 }}>
              {a.model || a.id}
              {a.description ? ` · ${a.description.slice(0, 60)}` : ''}
            </div>
          </button>
        )
      })}
      {list !== null && list.length > 0 && (
        <div style={{ padding: '6px 10px 4px', fontSize: 11, opacity: 0.5, borderTop: '1px solid color-mix(in srgb, currentColor 12%, transparent)', marginTop: 4 }}>
          GET /v1/agents · 实时(与 server 配置同步)
        </div>
      )}
    </div>
  )
}
