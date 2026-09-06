/**
 * AgentPicker — model/agent selection popover for the composer chip and the
 * hero agent button. Re-fetches GET /v1/agents on every open so the list is
 * always in sync with the server config (llm.providers → config agent store).
 * Selecting applies immediately via settings.model (the send path reads it).
 *
 * Rendering goes through a portal with fixed coordinates measured from the
 * anchor button: plain absolute positioning inside the composer gets clipped
 * by ancestor overflow/stacking contexts (the popover's first options were
 * cut off). Opens upward when there is headroom, otherwise downward, and
 * always clamps to the viewport.
 */

import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { store } from '../store.ts'
import { refreshAgents } from '../bridge/inferglow.ts'
import type { Agent } from '../api/types.ts'

export function AgentPicker({ anchor, onClose }: { anchor: HTMLElement | null; onClose: () => void }) {
  const [list, setList] = useState<Agent[] | null>(null) // null = loading
  const popRef = useRef<HTMLDivElement | null>(null)
  const [pos, setPos] = useState<{ left: number; top: number; maxHeight: number } | null>(null)

  // 每次打开重拉 — 与 server 配置保持同步(config 改后重启即反映)。
  useEffect(() => {
    let alive = true
    void refreshAgents().then(a => {
      if (alive) setList(a)
    })
    return () => { alive = false }
  }, [])

  // Measure after first paint so the popover flips/clamps with its real height.
  // Re-runs when the agent list arrives (loading → content changes the height).
  const loaded = list !== null
  useLayoutEffect(() => {
    const pop = popRef.current
    if (!anchor || !pop) return
    const a = anchor.getBoundingClientRect()
    const h = pop.offsetHeight
    const w = pop.offsetWidth
    const margin = 8
    const spaceUp = a.top - margin
    const spaceDown = window.innerHeight - a.bottom - margin
    let top: number
    let maxHeight: number
    if (spaceUp >= Math.min(h, 200) || spaceUp >= spaceDown) {
      maxHeight = Math.min(320, spaceUp)
      top = a.top - 6 - Math.min(h, maxHeight)
    } else {
      maxHeight = Math.min(320, spaceDown)
      top = a.bottom + 6
    }
    const left = Math.max(margin, Math.min(a.right - w, window.innerWidth - w - margin))
    setPos({ left, top, maxHeight })
  }, [anchor, loaded])

  // 点外部 / Escape 关闭 — 弹层挡住的区域可点击收起。
  useEffect(() => {
    const onDown = (e: PointerEvent) => {
      const t = e.target as Node
      if (popRef.current?.contains(t) || anchor?.contains(t)) return
      onClose()
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('pointerdown', onDown, true)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('pointerdown', onDown, true)
      document.removeEventListener('keydown', onKey)
    }
  }, [anchor, onClose])

  const current = store.settings.model

  function pick(id: string) {
    store.updateSetting('model', id)
    onClose()
  }

  return createPortal(
    <div
      ref={popRef}
      role="listbox"
      aria-label="选择 Agent / 模型"
      style={{
        position: 'fixed',
        // Pre-measure frame: render at the anchor then correct — avoids flicker.
        left: pos?.left ?? -9999,
        top: pos?.top ?? -9999,
        width: 280,
        maxHeight: pos?.maxHeight ?? 320,
        overflowY: 'auto',
        zIndex: 1000,
        background: 'var(--dsw-surface-raised, #1e2433)', color: 'inherit',
        border: '1px solid color-mix(in srgb, currentColor 18%, transparent)',
        borderRadius: 10, boxShadow: '0 8px 24px rgba(0,0,0,.35)', padding: 4,
      }}
    >
      {list === null && <div style={{ padding: '8px 10px', opacity: 0.7, fontSize: 12 }}>加载中…</div>}
      {list !== null && list.length === 0 && (
        <div style={{ padding: '10px 12px', fontSize: 12, opacity: 0.75, lineHeight: 1.6 }}>
          后端没有可用 Agent。
          <br />共享配置:项目一级 etc/config.json(或 ~/.inferglow/config.json)的 llm/providers;
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
    </div>,
    document.body,
  )
}
