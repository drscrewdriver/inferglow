/**
 * ConfigPopover — generic portal+fixed popover skeleton for the composer's
 * configuration chips (workspace / mode / permission / context).
 *
 * Extracted from AgentPicker's proven shape (pitfall G6): plain absolute
 * positioning inside the composer gets clipped by ancestor overflow/stacking
 * contexts, so the panel renders through a portal at coordinates measured
 * from the anchor button — opening upward when there is headroom, otherwise
 * downward, always clamped to the viewport. Click-outside and Escape close.
 *
 * The body is caller-supplied: items are the standard case; a custom
 * renderer covers the empty/disabled states (e.g. the mode placeholder).
 */

import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'

export interface PopoverItem {
  id: string
  /** Primary label. */
  label: string
  /** Secondary line (e.g. a root path or one-line description). */
  detail?: string
  selected?: boolean
}

export interface ConfigPopoverProps {
  anchor: HTMLElement | null
  items: PopoverItem[] | null // null = loading
  emptyText?: string
  footer?: string
  onPick: (id: string) => void
  onClose: () => void
  /** Popover width in px (default 280). */
  width?: number
}

export function ConfigPopover({ anchor, items, emptyText, footer, onPick, onClose, width = 280 }: ConfigPopoverProps) {
  const popRef = useRef<HTMLDivElement | null>(null)
  const [pos, setPos] = useState<{ left: number; top: number; maxHeight: number } | null>(null)

  // Measure after first paint so the popover flips/clamps with its real
  // height; re-runs when content resolves (loading → items changes height).
  const loaded = items !== null
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
      maxHeight = Math.min(340, spaceUp)
      top = a.top - 6 - Math.min(h, maxHeight)
    } else {
      maxHeight = Math.min(340, spaceDown)
      top = a.bottom + 6
    }
    const left = Math.max(margin, Math.min(a.right - w, window.innerWidth - w - margin))
    setPos({ left, top, maxHeight })
  }, [anchor, loaded])

  // Click-outside / Escape close — the covered area stays clickable.
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

  return createPortal(
    <div
      ref={popRef}
      role="listbox"
      aria-label="配置选择"
      style={{
        position: 'fixed',
        left: pos?.left ?? -9999,
        top: pos?.top ?? -9999,
        width,
        maxHeight: pos?.maxHeight ?? 340,
        overflowY: 'auto',
        zIndex: 1000,
        background: 'var(--dsw-surface-raised, #1e2433)', color: 'inherit',
        border: '1px solid color-mix(in srgb, currentColor 18%, transparent)',
        borderRadius: 10, boxShadow: '0 8px 24px rgba(0,0,0,.35)', padding: 4,
      }}
    >
      {items === null && <div style={{ padding: '8px 10px', opacity: 0.7, fontSize: 12 }}>加载中…</div>}
      {items !== null && items.length === 0 && (
        <div style={{ padding: '10px 12px', fontSize: 12, opacity: 0.75, lineHeight: 1.6 }}>
          {emptyText ?? '暂无可用选项'}
        </div>
      )}
      {items?.map(it => {
        const active = !!it.selected
        return (
          <button
            key={it.id}
            type="button"
            role="option"
            aria-selected={active}
            onClick={() => onPick(it.id)}
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
              <b style={{ fontSize: 13 }}>{it.label}</b>
              {active && <span style={{ fontSize: 11, color: '#4f7cff' }}>当前</span>}
            </div>
            {it.detail && (
              <div style={{ fontSize: 11, opacity: 0.65, marginTop: 2, wordBreak: 'break-all' }}>{it.detail}</div>
            )}
          </button>
        )
      })}
      {footer && items !== null && items.length > 0 && (
        <div style={{ padding: '6px 10px 4px', fontSize: 11, opacity: 0.5, borderTop: '1px solid color-mix(in srgb, currentColor 12%, transparent)', marginTop: 4 }}>
          {footer}
        </div>
      )}
    </div>,
    document.body,
  )
}
