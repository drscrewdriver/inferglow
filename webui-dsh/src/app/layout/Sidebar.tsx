/**
 * Sidebar - full DSH-style sidebar with:
 * - Logo/brand as new-session shortcut
 * - New session button with label
 * - Searchable session list
 * - Settings button + footer
 * - Collapse to 56px rail mode
 *
 * Collapse animation (SidebarRoot pattern):
 * - Content freezes at expanded width (inline style) and crossfades out (150ms)
 * - Column slides to 56px via grid track, clips frozen content
 * - At settle, wide content unmounts; rail icons enter via .railIn
 * - quietBars: scrollbars hide 2s after pointer leaves the column
 */

import { useState, useRef, useMemo, useEffect } from 'react'
import { Button } from '../../components/Button.tsx'
import {
  IconPanelLeft, IconPlus, IconSettings, IconTrash, IconChat, IconSearch,
} from '../../components/Icons.tsx'
import { store, subscribe } from '../../store.ts'
import { createSession, deleteSession, renameSession, selectSession } from '../../bridge/inferglow.ts'
import { WorkspaceAddModal } from '../../panels/WorkspaceAddModal.tsx'
import styles from './Sidebar.module.css'

/** Wide-content unmount delay; matches the 150ms wide-content fade-out. */
const COLLAPSE_SETTLE_MS = 150

/** How long the column's scrollbars stay drawn after the pointer leaves it.
 * The bar is a pointer affordance here, and hiding it on the leave event
 * itself makes it blink out while the pointer is only crossing the column's
 * edge — on the way to the conversation, or around a portalled menu.
 */
const SCROLLBAR_LINGER_MS = 2000

/**
 * Workspace tree groups. Sessions come from the InferGlow backend
 * (hydrated by the bridge); the single live group renders the real list
 * filtered by the workspace search.
 */
// Sessions are grouped UNDER their workspace (R8): one group per registered
// workspace, plus an "unassigned" group for records without a workspace field
// (legacy snapshots).
const WS_GROUPS_FALLBACK = [{ id: 'sessions', label: '会话', expanded: true, live: true }]

interface SidebarProps {
  onOpenSettings: () => void
}

export function Sidebar({ onOpenSettings }: SidebarProps) {
  const [collapsed, setCollapsed] = useState(store.sidebarCollapsed)
  const [pointerInside, setPointerInside] = useState(true)
  const [settled, setSettled] = useState(true)
  const [searchQuery, setSearchQuery] = useState('')
  const [sessions, setSessions] = useState(store.sessions)
  const [activeId, setActiveId] = useState(store.activeSessionId)
  const [searchVisible, setSearchVisible] = useState(false)
  const [workspaces, setWorkspaces] = useState(store.workspaces)
  const [activeWs, setActiveWs] = useState(store.activeWorkspace)
  const [addWsOpen, setAddWsOpen] = useState(false)
  const [collapsedGroups, setCollapsedGroups] = useState<Set<string>>(
    () => new Set(WS_GROUPS_FALLBACK.filter(g => !g.expanded).map(g => g.id)),
  )

  /** Track whether the sidebar was ever expanded (for railIn animation). */
  const everWide = useRef(!collapsed)
  const lastWideWidth = useRef(280)
  const lingerTimer = useRef<number | undefined>(undefined)

  useEffect(() => {
    const unsub = subscribe(() => {
      setSessions(store.sessions)
      setActiveId(store.activeSessionId)
      setCollapsed(store.sidebarCollapsed)
      setSearchQuery(store.searchQuery)
      setWorkspaces(store.workspaces)
      setActiveWs(store.activeWorkspace)
    })
    return unsub
  }, [])

  /** When collapsing: keep content mounted, fade it, unmount at settle.
   *  When expanding: clear settled so content remounts immediately. */
  useEffect(() => {
    if (!collapsed) {
      setSettled(false)
      return
    }
    everWide.current = true
    const timer = window.setTimeout(() => { setSettled(true) }, COLLAPSE_SETTLE_MS)
    return () => { window.clearTimeout(timer) }
  }, [collapsed])

  /** wide = visible content (during expand, or during collapse before settle). */
  const wide = !collapsed || !settled

  /** Freeze the content at its expanded width while it fades out (collapsed && wide):
   *  the sliding column then clips it instead of reflowing it. */
  if (!collapsed) {
    lastWideWidth.current = lastWideWidth.current || 280
  }

  /** Determine animation classes per SidebarRoot logic:
   *  - collapsed: add .collapsed + (everWide && .railIn)
   *  - wide && collapsed: add .fading
   *  - collapsed && !wide: no extra classes beyond .collapsed
   */
  const isRailIn = everWide.current && collapsed && !wide
  const isFading = collapsed && wide

  /** Scrollbar quiet mode: drawn while pointer is inside,
   *  and for SCROLLBAR_LINGER_MS after it leaves. */
  const columnRef = useRef<HTMLDivElement>(null)
  const cancelLinger = () => {
    window.clearTimeout(lingerTimer.current)
    lingerTimer.current = undefined
  }
  const armLinger = () => {
    if (lingerTimer.current !== undefined) return
    lingerTimer.current = window.setTimeout(() => {
      lingerTimer.current = undefined
      setPointerInside(false)
    }, SCROLLBAR_LINGER_MS)
  }

  useEffect(() => {
    if (!pointerInside) return
    const onMove = (e: PointerEvent) => {
      const rect = columnRef.current?.getBoundingClientRect()
      if (!rect) return
      const inside = e.clientX >= rect.left && e.clientX < rect.right &&
                     e.clientY >= rect.top && e.clientY < rect.bottom
      if (inside) {
        cancelLinger()
      } else {
        armLinger()
      }
    }
    document.addEventListener('pointermove', onMove)
    return () => { document.removeEventListener('pointermove', onMove); cancelLinger() }
  }, [pointerInside])

  // One sidebar group per registered workspace; sessions without a
  // workspace field (legacy records) land in the trailing unassigned group.
  const wsGroups = useMemo(() => {
    const base = workspaces.map(w => ({ id: w.name, label: w.name, expanded: true, live: true }))
    return [...base, { id: '(unassigned)', label: '未分配', expanded: true, live: true }]
  }, [workspaces])

  const filteredSessions = useMemo(() => {
    if (!searchQuery) return sessions
    const q = searchQuery.toLowerCase()
    return sessions.filter(s => s.title.toLowerCase().includes(q))
  }, [sessions, searchQuery])

  function handleNewSession() { void createSession() }
  function handleSelectSession(id: string) { void selectSession(id) }
  function handleDeleteSession(id: string) {
    if (confirm('确定删除此对话？')) { deleteSession(id) }
  }
  function handleToggleSidebar() { store.toggleSidebar() }

  function handleToggleGroup(id: string) {
    setCollapsedGroups(prev => {
      const next = new Set(prev)
      if (next.has(id)) { next.delete(id) } else { next.add(id) }
      return next
    })
  }

  const sidebarClasses = [
    styles.root,
    collapsed && styles.collapsed,
    isRailIn && styles.railIn,
    isFading && styles.fading,
    !pointerInside && styles.quietBars,
  ].filter(Boolean).join(' ')

  return (
    <div
      ref={columnRef}
      className={sidebarClasses}
      style={wide ? { width: collapsed ? lastWideWidth.current : undefined } : { width: 56 }}
      onPointerEnter={() => { cancelLinger(); setPointerInside(true) }}
      onPointerLeave={() => armLinger()}
    >
      {/* Logo row: brand wordmark (wide only) + collapse toggle */}
      <div className={styles.logoRow}>
        {wide && (
          <button
            type="button"
            className={styles.brand}
            aria-label="新对话"
            title="新对话"
            onClick={handleNewSession}
          >
            <span className={styles.brandLogo}>
              <svg width="24" height="24" viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg">
                <path d="M8.00003 0.3237C3.76075 0.3237 0.32373 3.76072 0.32373 8C0.32373 9.17603 0.589121 10.2922 1.0632 11.2901L1.35291 11.8989L2.5705 11.3205L2.28079 10.7117C1.89079 9.89074 1.67301 8.97167 1.67301 8C1.67301 4.50546 4.50549 1.67298 8.00003 1.67298C11.4946 1.67298 14.3271 4.50546 14.3271 8C14.3271 11.4945 11.4946 14.327 8.00003 14.327C7.28473 14.327 6.76077 14.277 6.29621 14.1487C5.83857 14.0224 5.40441 13.8109 4.88514 13.4488C4.12569 12.919 3.03778 12.7316 2.141 13.2978L2.12682 13.307L2.11264 13.3171L1.34886 13.854L1.79659 15.188L2.86122 14.4384C3.19068 14.2305 3.68325 14.2542 4.11326 14.5539C4.72789 14.9826 5.30042 15.2724 5.93762 15.4484C6.56803 15.6224 7.22776 15.6763 8.00003 15.6763C12.2393 15.6763 15.6763 12.2393 15.6763 8C15.6763 3.76072 12.2393 0.3237 8.00003 0.3237ZM7.32033 4.82535V7.32536H4.82538V8.67464H7.32033V11.1747H8.6696V8.67464H11.1747V7.32536H8.6696V4.82535H7.32033Z" fill="currentColor" />
              </svg>
            </span>
          </button>
        )}
        <Tooltip collapsed={collapsed} label={collapsed ? '展开侧栏' : '收起侧栏'} delayMs={500}>
          <button
            type="button"
            className={`${styles.iconButton} ${styles.toggleBtn}`}
            aria-label={collapsed ? '展开侧栏' : '收起侧栏'}
            onClick={handleToggleSidebar}
          >
            {/* Rail resting: fish logo (brand). Hover: panel icon (expand affordance). */}
            {!wide && <span className={styles.railFish}><svg width="24" height="24" viewBox="0 0 16 16" fill="none" aria-hidden="true"><path d="M8.00003 0.3237C3.76075 0.3237 0.32373 3.76072 0.32373 8C0.32373 9.17603 0.589121 10.2922 1.0632 11.2901L1.35291 11.8989L2.5705 11.3205L2.28079 10.7117C1.89079 9.89074 1.67301 8.97167 1.67301 8C1.67301 4.50546 4.50549 1.67298 8.00003 1.67298C11.4946 1.67298 14.3271 4.50546 14.3271 8C14.3271 11.4945 11.4946 14.327 8.00003 14.327C7.28473 14.327 6.76077 14.277 6.29621 14.1487C5.83857 14.0224 5.40441 13.8109 4.88514 13.4488C4.12569 12.919 3.03778 12.7316 2.141 13.2978L2.12682 13.307L2.11264 13.3171L1.34886 13.854L1.79659 15.188L2.86122 14.4384C3.19068 14.2305 3.68325 14.2542 4.11326 14.5539C4.72789 14.9826 5.30042 15.2724 5.93762 15.4484C6.56803 15.6224 7.22776 15.6763 8.00003 15.6763C12.2393 15.6763 15.6763 12.2393 15.6763 8C15.6763 3.76072 12.2393 0.3237 8.00003 0.3237ZM7.32033 4.82535V7.32536H4.82538V8.67464H7.32033V11.1747H8.6696V8.67464H11.1747V7.32536H8.6696V4.82535H7.32033Z" fill="currentColor"/></svg></span>}
            <span className={styles.panelIcon}><IconPanelLeft size={wide ? 16 : 18} /></span>
          </button>
        </Tooltip>
      </div>

      {/* New Session button */}
      <Tooltip collapsed={!wide} label="新建会话" delayMs={500}>
        <button
          type="button"
          className={styles.newSessionBtn}
          aria-label="新建会话"
          onClick={handleNewSession}
        >
          <svg width={wide ? 14 : 18} height={wide ? 14 : 18} viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg">
            <path d="M8.00003 0.3237C3.76075 0.3237 0.32373 3.76072 0.32373 8C0.32373 9.17603 0.589121 10.2922 1.0632 11.2901L1.35291 11.8989L2.5705 11.3205L2.28079 10.7117C1.89079 9.89074 1.67301 8.97167 1.67301 8C1.67301 4.50546 4.50549 1.67298 8.00003 1.67298C11.4946 1.67298 14.3271 4.50546 14.3271 8C14.3271 11.4945 11.4946 14.327 8.00003 14.327C7.28473 14.327 6.76077 14.277 6.29621 14.1487C5.83857 14.0224 5.40441 13.8109 4.88514 13.4488C4.12569 12.919 3.03778 12.7316 2.141 13.2978L2.12682 13.307L2.11264 13.3171L1.34886 13.854L1.79659 15.188L2.86122 14.4384C3.19068 14.2305 3.68325 14.2542 4.11326 14.5539C4.72789 14.9826 5.30042 15.2724 5.93762 15.4484C6.56803 15.6224 7.22776 15.6763 8.00003 15.6763C12.2393 15.6763 15.6763 12.2393 15.6763 8C15.6763 3.76072 12.2393 0.3237 8.00003 0.3237ZM7.32033 4.82535V7.32536H4.82538V8.67464H7.32033V11.1747H8.6696V8.67464H11.1747V7.32536H8.6696V4.82535H7.32033Z" fill="currentColor" />
          </svg>
          {wide && <span className={styles.newSessionLabel}>新会话</span>}
        </button>
      </Tooltip>

      {/* Collapsed rail: workspace actions (search / add) as icons */}
      {!wide && (
        <div className="dsh-ws-rail">
          <Tooltip collapsed label="添加工作区" delayMs={500}>
            <button type="button" className="dsh-ws-rail-btn" aria-label="添加工作区" title="添加工作区" onClick={() => setAddWsOpen(true)}>
              <IconPlus size={18} />
            </button>
          </Tooltip>
          <Tooltip collapsed label="搜索会话" delayMs={500}>
            <button
              type="button"
              className="dsh-ws-rail-btn"
              aria-label="搜索会话"
              title="搜索会话"
              onClick={() => setSearchVisible(v => !v)}
            >
              <IconSearch size={18} />
            </button>
          </Tooltip>
        </div>
      )}

      {/* Workspace browsing region — always mounted so collapse clips correctly */}
      <div className={styles.regionArea}>
        {wide && (
          <div className="dsh-ws-sidebar-workspace">
            {/* ── Section header: 工作区 + view options ── */}
            <div className="dsh-ws-section-header">
              <span className="dsh-ws-section-title">工作区</span>
              <div className="dsh-ws-search-slot">
                <div className={`dsh-ws-search${searchVisible ? ' dsh-ws-search-open' : ''}`}>
                  <button
                    type="button"
                    className="dsh-ws-search-btn"
                    aria-label="搜索会话"
                    aria-expanded={searchVisible}
                    title="搜索会话"
                    onClick={() => setSearchVisible(v => !v)}
                  >
                    <IconSearch size={18} />
                  </button>
                  {searchVisible && (
                    <input
                      type="text"
                      className="dsh-ws-search-input"
                      placeholder="搜索会话…"
                      value={searchQuery}
                      onChange={e => setSearchQuery(e.target.value)}
                      autoFocus
                    />
                  )}
                  {searchVisible && searchQuery && (
                    <button className="dsh-ws-search-clear" onClick={() => setSearchQuery('')}>
                      <svg width="12" height="12" viewBox="0 0 12 12" fill="none">
                        <path d="M3 3l6 6M9 3l-6 6" stroke="currentColor" strokeWidth="1.2" strokeLinecap="round"/>
                      </svg>
                    </button>
                  )}
                </div>
              </div>
              <div className="dsh-ws-section-actions">
                <button type="button" className="dsh-ws-icon-btn" aria-label="视图选项" title="视图选项">
                  <svg width="18" height="18" viewBox="0 0 16 16" fill="none">
                    <rect x="2" y="2" width="5" height="5" rx="1" stroke="currentColor" strokeWidth="1.3"/>
                    <rect x="9" y="2" width="5" height="5" rx="1" stroke="currentColor" strokeWidth="1.3"/>
                    <rect x="2" y="9" width="5" height="5" rx="1" stroke="currentColor" strokeWidth="1.3"/>
                    <rect x="9" y="9" width="5" height="5" rx="1" stroke="currentColor" strokeWidth="1.3"/>
                  </svg>
                </button>
                <button type="button" className="dsh-ws-icon-btn" aria-label="添加工作区" title="添加工作区" onClick={() => setAddWsOpen(true)}>
                  <IconPlus size={18} />
                </button>
              </div>
            </div>

            {/* ── Workspace registry (real, selectable; 添加工作区 opens the form) ── */}
            <div className="dsh-ws-tree" style={{ marginBottom: 8 }}>
              <div className="dsh-ws-groups">
                <div className="dsh-ws-group">
                  <button type="button" className="dsh-ws-group-title" aria-expanded="true">
                    <span className="dsh-ws-group-caret dsh-ws-group-caret-collapsed">
                      <svg width="10" height="10" viewBox="0 0 12 12" fill="none">
                        <path d="M4 2.5L8.5 6 4 9.5" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round"/>
                      </svg>
                    </span>
                    <span className="dsh-ws-group-label">注册的 workspace</span>
                  </button>
                  {store.authRequired && (
                    <button
                      type="button"
                      className="dsh-ws-auth-hint"
                      title="设置 → API 配置 中填写 Bearer Key"
                      onClick={() => store.openSettings()}
                      style={{
                        display: 'block', width: '100%', textAlign: 'left', cursor: 'pointer',
                        margin: '4px 0', padding: '6px 10px', borderRadius: 8, border: 'none',
                        background: 'color-mix(in srgb, #d4544a 14%, transparent)', color: '#e0857d',
                        font: 'inherit', fontSize: 11.5, lineHeight: 1.5,
                      }}
                    >
                      ⚠ API Key 未配置或无效 — 列表不可用，点击打开设置
                    </button>
                  )}
                  <div className="dsh-ws-rows">
                    {workspaces.map(w => (
                      <div
                        key={w.name}
                        className={`dsh-ws-row${activeWs === w.name ? ' dsh-ws-row-active' : ''}`}
                        title={w.root || '(server 默认目录)'}
                        onClick={() => store.setActiveWorkspace(w.name)}
                      >
                        <span className="dsh-ws-row-icon">
                          <svg width="13" height="13" viewBox="0 0 16 16" fill="none">
                            <path d="M2.3 3.5a1.2 1.2 0 011.2-1.2h3l1.1 1.4h5.2a1.2 1.2 0 011.2 1.2v1h.2a1.2 1.2 0 011 1.7l-1.1 3.2a1.2 1.2 0 01-1.1.8H3.2a1.2 1.2 0 01-1.2-1.5l.3-4.6z" stroke="currentColor" strokeWidth="1.2" strokeLinejoin="round"/>
                          </svg>
                        </span>
                        <span className="dsh-ws-row-title">{w.name}</span>
                        <button
                          className="dsh-ws-row-delete"
                          title={'在 ' + w.name + ' 开启对话'}
                          aria-label={'在 ' + w.name + ' 开启对话'}
                          onClick={e => {
                            e.stopPropagation()
                            store.setActiveWorkspace(w.name)
                            void createSession(w.name)
                          }}
                        >
                          <svg width="11" height="11" viewBox="0 0 12 12" fill="none">
                            <path d="M6 1.5v9M1.5 6h9" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round"/>
                          </svg>
                        </button>
                      </div>
                    ))}
                    {workspaces.length === 0 && (
                      <div className="dsh-ws-empty">server 未注册 workspace</div>
                    )}
                  </div>
                </div>
              </div>
            </div>

            {/* ── List area → tree → groups ── */}
            <div className="dsh-ws-list-area">
              <div className="dsh-ws-tree">
                <div className="dsh-ws-groups">
                  {wsGroups.map(group => {
                    const isCollapsed = collapsedGroups.has(group.id)
                    const groupSessions = filteredSessions.filter(sess =>
                      group.id === '(unassigned)'
                        ? !sess.workspace
                        : (sess.workspace ?? activeWs) === group.id,
                    )
                    return (
                      <div className="dsh-ws-group" key={group.id}>
                        <button
                          type="button"
                          className="dsh-ws-group-title"
                          onClick={() => handleToggleGroup(group.id)}
                          aria-expanded={!isCollapsed}
                        >
                          <span className={`dsh-ws-group-caret${isCollapsed ? ' dsh-ws-group-caret-collapsed' : ''}`}>
                            <svg width="10" height="10" viewBox="0 0 12 12" fill="none">
                              <path d="M4 2.5L8.5 6 4 9.5" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round"/>
                            </svg>
                          </span>
                          <span className="dsh-ws-group-label">{group.label}</span>
                        </button>

                        {!isCollapsed && (
                          <div className="dsh-ws-rows">
                            {groupSessions.map(session => (
                              <div
                                key={session.id}
                                className={`dsh-ws-row${activeId === session.id ? ' dsh-ws-row-active' : ''}`}
                                onClick={() => handleSelectSession(session.id)}
                              >
                                <span className="dsh-ws-row-icon"><IconChat size={13} /></span>
                                <span
                                  className="dsh-ws-row-title"
                                  title={session.title + '（双击重命名）'}
                                  onDoubleClick={e => {
                                    e.stopPropagation()
                                    const next = prompt('重命名会话', session.title)
                                    if (next === null || next.trim() === '' || next === session.title) return
                                    store.renameSession(session.id, next.trim())
                                    renameSession(session.id, next.trim())
                                  }}
                                >
                                  {session.title}
                                </span>
                                <button
                                  className="dsh-ws-row-delete"
                                  title="删除"
                                  onClick={e => { e.stopPropagation(); handleDeleteSession(session.id) }}
                                >
                                  <IconTrash size={11} />
                                </button>
                              </div>
                            ))}
                            {groupSessions.length === 0 && (
                              <div className="dsh-ws-empty">
                                {group.id !== '(unassigned)' ? '在此 workspace 开启对话' : '暂无对话'}
                              </div>
                            )}
                          </div>
                        )}
                      </div>
                    )
                  })}
                </div>
              </div>
            </div>
          </div>
        )}
      </div>

      {addWsOpen && <WorkspaceAddModal onClose={() => setAddWsOpen(false)} />}

      {/* Footer — always mounted, sibling of regionArea (not nested) */}
      <div className={styles.footArea}>
        {wide ? (
          <div className={styles.foot}>
            <div className="dsh-sidebar-foot-actions">
              <Button variant="ghost" size="sm" onClick={() => setSearchVisible(v => !v)} icon={<IconSearch size={14} />}>
                搜索
              </Button>
              <Button
                variant="ghost"
                size="sm"
                onClick={onOpenSettings}
                icon={<IconSettings size={14} />}
              >
                设置
              </Button>
              <Button variant="ghost" size="sm" icon={<IconChat size={14} />}>
                任务板
              </Button>
            </div>
          </div>
        ) : (
          <div className="dsh-ws-rail dsh-ws-rail--foot">
            <Tooltip collapsed label="任务板" delayMs={500}>
              <button type="button" className="dsh-ws-rail-btn" aria-label="任务板" title="任务板">
                <IconChat size={18} />
              </button>
            </Tooltip>
            <Tooltip collapsed label="搜索" delayMs={500}>
              <button type="button" className="dsh-ws-rail-btn" aria-label="搜索" title="搜索">
                <IconSearch size={18} />
              </button>
            </Tooltip>
            <Tooltip collapsed label="设置" delayMs={500}>
              <button type="button" className="dsh-ws-rail-btn" aria-label="设置" title="设置" onClick={onOpenSettings}>
                <IconSettings size={18} />
              </button>
            </Tooltip>
          </div>
        )}
      </div>
    </div>
  )
}

/** Minimal tooltip component for rail hover labels. */
function Tooltip({ collapsed, label, delayMs, children }: {
  collapsed: boolean
  label: string
  delayMs: number
  children: React.ReactNode
}) {
  const [visible, setVisible] = useState(false)
  const timerRef = useRef<number | undefined>(undefined)

  useEffect(() => {
    if (collapsed) return
    setVisible(false)
  }, [collapsed])

  const show = () => {
    if (!collapsed) return
    clearTimeout(timerRef.current)
    timerRef.current = window.setTimeout(() => setVisible(true), delayMs)
  }
  const hide = () => {
    clearTimeout(timerRef.current)
    timerRef.current = window.setTimeout(() => setVisible(false), 50)
  }

  return (
    <div
      className={styles.tooltipWrapper}
      onMouseEnter={show}
      onMouseLeave={hide}
      style={{ position: 'relative' }}
    >
      {children}
      {visible && <span className={styles.tooltipLabel}>{label}</span>}
    </div>
  )
}
