/**
 * PaneEmptyCards — the empty-state content shown inside a panel when it has
 * no open tab: a row of cards (文件/源代码管理/…) each opening that pane.
 * Mirrors the reference `.nArs4W_paneEmptyCards`.
 */

export type TabKind = 'files' | 'scm' | 'tasks' | 'subagent' | 'sidechat' | 'terminal' | 'browser'

export const TAB_TYPES: { kind: TabKind; label: string }[] = [
  { kind: 'files', label: '文件' },
  { kind: 'scm', label: '源代码管理' },
  { kind: 'tasks', label: '待办' },
  { kind: 'subagent', label: '任务管理' },
  { kind: 'sidechat', label: '侧边对话(beta)' },
  { kind: 'terminal', label: '终端' },
  { kind: 'browser', label: '浏览器' },
]

export function TabIcon({ kind }: { kind: TabKind }) {
  switch (kind) {
    case 'files':
      return (
        <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
          <path d="M5.2 1.57h4.53c.3 0 .57.14.74.37l.27.4h1.43A1.4 1.4 0 0114.59 5.3l-1.05 3.36a1.4 1.4 0 01-1.33.97h-9.2A1.4 1.4 0 011.6 8.3L2.2 4.4a1.4 1.4 0 011.34-1.1h1.66z" fill="currentColor"/>
        </svg>
      )
    case 'scm':
      return (
        <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
          <circle cx="13" cy="3.2" r="1.6" stroke="currentColor" strokeWidth="1.4"/>
          <circle cx="4" cy="8" r="1.6" stroke="currentColor" strokeWidth="1.4"/>
          <circle cx="13" cy="12.8" r="1.6" stroke="currentColor" strokeWidth="1.4"/>
          <path d="M5.6 8H13M11.6 4.8L5.2 7.2M11.6 11.2L5.2 8.8" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round"/>
        </svg>
      )
    case 'tasks':
      return (
        <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
          <circle cx="8" cy="8" r="5.5" stroke="currentColor" strokeWidth="1.4"/>
          <path d="M8 4.5v3.5l2.5 2" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round"/>
        </svg>
      )
    case 'subagent':
      return (
        <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
          <circle cx="4" cy="4" r="2.2" stroke="currentColor" strokeWidth="1.4"/>
          <circle cx="12" cy="12" r="2.2" stroke="currentColor" strokeWidth="1.4"/>
          <path d="M4 6.2v3.3a2.3 2.3 0 002.3 2.3H9.7" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round"/>
          <path d="M12 9.8V8a2 2 0 00-2-2H6.2" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" opacity="0.55"/>
        </svg>
      )
    case 'sidechat':
      return (
        <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
          <path d="M2 3a1.5 1.5 0 011.5-1.5h9A1.5 1.5 0 0114 3v6a1.5 1.5 0 01-1.5 1.5H6L3 13V3z" stroke="currentColor" strokeWidth="1.4" strokeLinejoin="round"/>
        </svg>
      )
    case 'browser':
      return (
        <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
          <circle cx="8" cy="8" r="6.5" stroke="currentColor" strokeWidth="1.4"/>
          <ellipse cx="8" cy="8" rx="2.8" ry="6.5" stroke="currentColor" strokeWidth="1.4"/>
          <path d="M1.5 8h13" stroke="currentColor" strokeWidth="1.4"/>
        </svg>
      )
    case 'terminal':
    default:
      return (
        <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
          <rect x="1.5" y="2.5" width="13" height="11" rx="2" stroke="currentColor" strokeWidth="1.4"/>
          <path d="M4.5 6.25 6.75 8 4.5 9.75" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round"/>
          <path d="M8.5 10.4h3" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round"/>
        </svg>
      )
  }
}

export function PaneEmptyCards({ tabTypes, onPick }: {
  tabTypes: { kind: string; label: string }[]
  onPick: (kind: string, label: string) => void
}) {
  return (
    <div className="dsh-pane-empty-cards">
      {tabTypes.map(t => (
        <button
          key={t.kind}
          type="button"
          className="dsh-pane-card"
          title={t.label}
          onClick={() => onPick(t.kind, t.label)}
        >
          <span className="dsh-pane-card-icon"><TabIcon kind={t.kind as TabKind} /></span>
          <span className="dsh-pane-card-label">{t.label}</span>
        </button>
      ))}
    </div>
  )
}