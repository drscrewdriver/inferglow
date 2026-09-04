/**
 * TabbedPane — a self-contained, per-area panel of tabs.
 * Used by both the right (details) column and the bottom panel, each area keeps
 * its own tab list. The "+" button, its menu, and the 6 empty-state cards all
 * add a tab to THIS area; clicking a tab activates its content.
 */

import { useState, type ReactNode } from 'react'
import { PaneEmptyCards, TabIcon, TAB_TYPES, type TabKind } from './PaneEmptyCards.tsx'
import { PaneContent } from './PanePanels.tsx'

export interface PanelTab {
  id: string
  kind: TabKind
  label: string
  active: boolean
}

export function TabbedPane({
  defaultTabs = [],
  headerActions,
}: {
  defaultTabs?: PanelTab[]
  headerActions?: ReactNode
}) {
  const [tabs, setTabs] = useState<PanelTab[]>(defaultTabs)
  const [menuOpen, setMenuOpen] = useState(false)
  const active = tabs.find(t => t.active) ?? tabs[0]

  function activate(id: string) {
    setTabs(ts => ts.map(t => ({ ...t, active: t.id === id })))
  }
  function closeTab(id: string) {
    setTabs(ts => {
      const idx = ts.findIndex(t => t.id === id)
      if (idx === -1) return ts
      const next = ts.filter(t => t.id !== id)
      if (ts[idx].active && next.length > 0) {
        const fallback = next[Math.min(idx, next.length - 1)]
        return next.map(t => ({ ...t, active: t.id === fallback.id }))
      }
      return next
    })
    setMenuOpen(false)
  }
  function addTab(kind: TabKind, label: string) {
    setTabs(ts => {
      const existing = ts.find(t => t.kind === kind)
      if (existing) return ts.map(t => ({ ...t, active: t.id === existing.id }))
      const id = `${kind}-${Date.now()}`
      return [...ts.map(t => ({ ...t, active: false })), { id, kind, label, active: true }]
    })
    setMenuOpen(false)
  }

  return (
    <div className="dsh-tabbed-pane">
      <div className="dsh-bottom-panel-tabs dsh-tab-bar">
        {tabs.map(tab => (
          <div
            key={tab.id}
            className={`dsh-bottom-panel-tab${tab.active ? ' dsh-bottom-panel-tab-active' : ''}`}
            title={tab.label}
            onClick={() => activate(tab.id)}
          >
            <TabIcon kind={tab.kind} />
            <span>{tab.label}</span>
            <button
              type="button"
              className="dsh-bottom-panel-tab-close"
              aria-label="关闭"
              onClick={e => { e.stopPropagation(); closeTab(tab.id) }}
            >
              <svg width="14" height="14" viewBox="0 0 14 14" fill="none">
                <path d="M10.6074 4.40278L8.00975 6.99973L10.6074 9.59739L9.59736 10.6074L6.9997 8.00978L4.40274 10.6074L3.3927 9.59739L5.98966 6.99973L3.3927 4.40278L4.40274 3.39273L6.9997 5.98969L9.59736 3.39273L10.6074 4.40278Z" fill="currentColor"/>
              </svg>
            </button>
          </div>
        ))}
        <span className="dsh-bottom-panel-add-wrap">
          <button
            type="button"
            className="dsh-pane-add-btn"
            aria-label="新建标签页"
            title="新建标签页"
            onClick={() => setMenuOpen(o => !o)}
          >
            <svg width="14" height="14" viewBox="0 0 16 16" fill="none">
              <path d="M8.64453 1.5V7.34961H14.5V8.65039H8.64453V14.5H7.34473V8.65039H1.5V7.34961H7.34473V1.5H8.64453Z" fill="currentColor"/>
            </svg>
          </button>
          {menuOpen && (
            <div className="dsh-add-menu" role="menu">
              {TAB_TYPES.map(item => (
                <button
                  key={item.kind}
                  type="button"
                  role="menuitem"
                  className="dsh-add-menu-item"
                  onClick={() => addTab(item.kind, item.label)}
                >
                  <span className="dsh-add-menu-icon"><TabIcon kind={item.kind} /></span>
                  <span className="dsh-add-menu-label">{item.label}</span>
                </button>
              ))}
            </div>
          )}
        </span>
        {headerActions && (
          <div className="dsh-pane-header-actions" onClick={e => e.stopPropagation()}>
            {headerActions}
          </div>
        )}
      </div>
      <div className="dsh-tab-body">
        {tabs.length === 0 ? (
          <PaneEmptyCards tabTypes={TAB_TYPES} onPick={(k, l) => addTab(k as TabKind, l)} />
        ) : (
          <PaneContent kind={active.kind} />
        )}
      </div>
    </div>
  )
}