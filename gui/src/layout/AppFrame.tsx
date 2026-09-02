import { useEffect, useState, type CSSProperties, type ReactNode } from 'react'
import styles from './AppFrame.module.css'

/** Viewport (px) below which the sidebar auto-collapses to a rail. */
export const SIDEBAR_AUTO_COLLAPSE = 960

const SIDEBAR_MIN = 200
const SIDEBAR_MAX = 520
const DETAILS_MIN = 240
const DETAILS_MAX = 560

function clamp(v: number, lo: number, hi: number): number {
  return Math.max(lo, Math.min(hi, v))
}

/** Read a --igw-* length var, falling back to the supplied default px. */
function readPx(name: string, fallback: number): number {
  const v = getComputedStyle(document.documentElement).getPropertyValue(name).trim()
  const n = v ? Number.parseFloat(v) : Number.NaN
  return Number.isFinite(n) && n > 0 ? n : fallback
}

export interface AppFrameProps {
  /** Left pane (SidebarRoot). Rendered inside an auto-collapsible column. */
  sidebar: ReactNode
  /** Center conversation pane, fills remaining grid space. */
  children: ReactNode
  /** Right details pane; hidden unless `detailsOpen`. */
  details?: ReactNode
  /** Manually force the sidebar rail (overrides auto-collapse once used). */
  sidebarCollapsed?: boolean
  onSidebarToggle?: () => void
  /** Show/hide the details column. */
  detailsOpen?: boolean
  onDetailsToggle?: () => void
}

/**
 * Three-column application frame: `sidebar | conversation | details`.
 * Grid columns are user-resizable via pointer capture (rAF-throttled) and the
 * sidebar auto-collapses to a rail below SIDEBAR_AUTO_COLLAPSE.
 */
export function AppFrame({
  sidebar,
  children,
  details,
  sidebarCollapsed,
  detailsOpen = false,
}: AppFrameProps): ReactNode {
  const [sidebarW, setSidebarW] = useState<number>(() => readPx('--igw-sidebar-width', 258))
  const [detailsW, setDetailsW] = useState<number>(() => readPx('--igw-details-width', 300))
  const [autoCollapsed, setAutoCollapsed] = useState(false)

  // Narrow viewport → auto-collapse the sidebar to its rail.
  useEffect(() => {
    const mq = window.matchMedia(`(max-width: ${SIDEBAR_AUTO_COLLAPSE - 1}px)`)
    const onChange = (e: MediaQueryListEvent) => setAutoCollapsed(e.matches)
    setAutoCollapsed(mq.matches)
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }, [])

  const railCollapsed = sidebarCollapsed ?? autoCollapsed

  const gridTemplateColumns = `${railCollapsed ? 44 : Math.round(sidebarW)}px minmax(0, 1fr) ${detailsOpen ? Math.round(detailsW) : 0}px`
  const style = {
    gridTemplateColumns,
    '--sidebar-w': `${railCollapsed ? 44 : Math.round(sidebarW)}px`,
    '--details-w': `${detailsOpen ? Math.round(detailsW) : 0}px`,
  } as CSSProperties

  /** Drag a divider to resize its adjacent pane (pointer capture + rAF). */
  const startResize =
    (which: 'sidebar' | 'details') => (e: React.PointerEvent<HTMLDivElement>) => {
      const handle = e.currentTarget
      handle.setPointerCapture(e.pointerId)
      const startX = e.clientX
      const startW = which === 'sidebar' ? sidebarW : detailsW
      let raf = 0
      const move = (ev: PointerEvent) => {
        cancelAnimationFrame(raf)
        raf = requestAnimationFrame(() => {
          const dx = ev.clientX - startX
          if (which === 'sidebar') setSidebarW(clamp(startW + dx, SIDEBAR_MIN, SIDEBAR_MAX))
          else setDetailsW(clamp(startW - dx, DETAILS_MIN, DETAILS_MAX))
        })
      }
      const up = () => {
        cancelAnimationFrame(raf)
        handle.removeEventListener('pointermove', move)
        handle.removeEventListener('pointerup', up)
        handle.removeEventListener('pointercancel', up)
        try {
          handle.releasePointerCapture(e.pointerId)
        } catch {
          // pointer already released
        }
      }
      handle.addEventListener('pointermove', move)
      handle.addEventListener('pointerup', up)
      handle.addEventListener('pointercancel', up)
    }

  return (
    <div className={styles.frame} style={style}>
      <section
        className={`${styles.pane} ${styles.sidebar}${railCollapsed ? ` ${styles.sidebarRail}` : ''}`}
      >
        {sidebar}
      </section>
      <div
        className={styles.divider}
        onPointerDown={startResize('sidebar')}
        title="拖拽调整侧边栏宽度"
        aria-hidden
      />
      <main className={styles.main}>{children}</main>
      <div
        className={styles.divider}
        onPointerDown={startResize('details')}
        title="拖拽调整详情面板宽度"
        aria-hidden
      />
      <section
        className={`${styles.pane} ${styles.details}${detailsOpen ? ` ${styles.detailsOpen}` : ''}`}
        aria-hidden={!detailsOpen}
      >
        {details}
      </section>
    </div>
  )
}