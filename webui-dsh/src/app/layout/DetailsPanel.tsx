/**
 * DetailsPanel — right-hand details column.
 *
 * One merged top bar (48px, level with the top header): tabs / "+" on the
 * left, and the two "shadow" nav toggles on the right. Tab list is independent
 * of the bottom panel.
 */

import { TabbedPane } from './TabbedPane.tsx'

interface DetailsPanelProps {
  open: boolean
  onToggle: () => void
  onOpenBottom: () => void
  width: number
}

export function DetailsPanel({ open, onToggle, onOpenBottom, width }: DetailsPanelProps) {
  return (
    <aside
      className={`dsh-workspace-details${open ? ' dsh-workspace-details-open' : ''}`}
      style={{ width: open ? width : 0 }}
    >
      {open && (
        <div className="dsh-details">
          <div className="dsh-details-body">
            <TabbedPane
              defaultTabs={[{ id: 'todo', kind: 'tasks', label: '待办', active: true }]}
              headerActions={
                <>
                  <button
                    type="button"
                    className="dsh-details-toggle"
                    aria-label="展开底部面板"
                    title="展开底部面板"
                    onClick={onOpenBottom}
                  >
                    <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
                      <rect x="1.5" y="2" width="13" height="12" rx="2.5" stroke="currentColor" strokeWidth="1.5"/>
                      <rect x="3.25" y="10" width="9.5" height="2.75" rx="1" fill="currentColor" stroke="none"/>
                    </svg>
                  </button>
                  <button
                    type="button"
                    className="dsh-details-toggle"
                    aria-label="折叠侧边栏"
                    title="折叠侧边栏"
                    onClick={onToggle}
                  >
                    <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
                      <rect x="1.5" y="2" width="13" height="12" rx="2.5" stroke="currentColor" strokeWidth="1.5"/>
                      <rect x="10.5" y="3.25" width="2.75" height="9.5" rx="1" fill="currentColor" stroke="none"/>
                    </svg>
                  </button>
                </>
              }
            />
          </div>
        </div>
      )}
    </aside>
  )
}