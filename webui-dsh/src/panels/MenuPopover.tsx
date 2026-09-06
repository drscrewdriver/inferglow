/**
 * MenuPopover — generic portal+fixed menu for the sidebar's row actions and
 * the header view-options button (R9). Extracted from ConfigPopover's proven
 * positioning shape (pitfall G6: plain absolute positioning inside the
 * sidebar gets clipped by ancestor overflow/stacking contexts). Opens upward
 * when there is headroom, otherwise downward; clamped to the viewport;
 * click-outside and Escape close.
 *
 * Sections are optional labeled groups (the view menu's 分组方式/排序方式);
 * a section without a label renders as a plain action list (row menus).
 */

import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'

export interface MenuItem {
  id: string
  label: string
  /** Draws the DSH-style check (single-choice menus). */
  selected?: boolean
  /** Destructive rows (删除…). */
  danger?: boolean
  /** Toggle rows: keep the menu open after picking. */
  stayOpen?: boolean
}

export interface MenuSection {
  label?: string
  items: MenuItem[]
}

export function MenuPopover({ anchor, sections, onPick, onClose, width = 200 }: {
  anchor: HTMLElement | null
  sections: MenuSection[]
  onPick: (id: string) => void
  onClose: () => void
  width?: number
}) {
  const popRef = useRef<HTMLDivElement | null>(null)
  const [pos, setPos] = useState<{ left: number; top: number } | null>(null)

  useLayoutEffect(() => {
    const pop = popRef.current
    if (!anchor || !pop) return
    const a = anchor.getBoundingClientRect()
    const h = pop.offsetHeight
    const margin = 8
    const spaceUp = a.top - margin
    const spaceDown = window.innerHeight - a.bottom - margin
    // Prefer opening downward (rows are usually near the bottom of the
    // sidebar); flip up only when there is clearly no room below.
    const top = spaceDown >= Math.min(h, 180) || spaceDown >= spaceUp
      ? Math.min(a.bottom + 4, window.innerHeight - h - margin)
      : Math.max(margin, a.top - h - 4)
    const left = Math.max(margin, Math.min(a.left, window.innerWidth - width - margin))
    setPos({ left, top })
  }, [anchor, width, sections])

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
      role="menu"
      style={{
        position: 'fixed',
        left: pos?.left ?? -9999,
        top: pos?.top ?? -9999,
        width,
        maxHeight: 340,
        overflowY: 'auto',
        zIndex: 1000,
        background: 'var(--dsw-surface-raised, #1e2433)', color: 'inherit',
        border: '1px solid color-mix(in srgb, currentColor 18%, transparent)',
        borderRadius: 10, boxShadow: '0 8px 24px rgba(0,0,0,.35)', padding: 4,
      }}
    >
      {sections.map((sec, i) => (
        <div key={sec.label ?? i} style={i > 0 ? { borderTop: '1px solid color-mix(in srgb, currentColor 12%, transparent)', marginTop: 4, paddingTop: 4 } : undefined}>
          {sec.label && (
            <div style={{ padding: '4px 10px 2px', fontSize: 11, opacity: 0.55 }}>{sec.label}</div>
          )}
          {sec.items.map(item => (
            <button
              key={item.id}
              type="button"
              role="menuitem"
              onClick={() => {
                onPick(item.id)
                if (!item.stayOpen) onClose()
              }}
              style={{
                display: 'flex', width: '100%', alignItems: 'center', justifyContent: 'space-between',
                gap: 8, textAlign: 'left', cursor: 'pointer',
                padding: '7px 10px', margin: 0, border: 'none', borderRadius: 7,
                background: 'transparent', color: item.danger ? '#e0857d' : 'inherit', font: 'inherit', fontSize: 12.5,
              }}
              onMouseEnter={e => { (e.currentTarget as HTMLButtonElement).style.background = 'color-mix(in srgb, currentColor 8%, transparent)' }}
              onMouseLeave={e => { (e.currentTarget as HTMLButtonElement).style.background = 'transparent' }}
            >
              <span>{item.label}</span>
              {item.selected && (
                <svg width="13" height="13" viewBox="0 0 16 16" fill="none" style={{ flexShrink: 0 }}>
                  <path d="M14.6 4.2 6.6 12.6 1.6 8.1l1-1.1 4 3.6 7.2-7.4 0.8 1z" fill="currentColor"/>
                </svg>
              )}
            </button>
          ))}
        </div>
      ))}
    </div>,
    document.body,
  )
}
