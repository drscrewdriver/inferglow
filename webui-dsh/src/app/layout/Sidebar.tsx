/**
 * Sidebar - full DSH-style sidebar with:
 * - Logo/brand as new-session shortcut
 * - New session button with label
 * - Searchable session list
 * - Settings button + footer
 * - Collapse to 56px rail mode
 *
 * R9 (DSH list-organization alignment): the former "registry + session
 * groups" two-block layout is now ONE tree — one group per registered
 * workspace (folder row with ⋯ menu + ＋ new-session), session rows with a
 * relative-time label and a ⋯ menu (归档 / Fork / 重命名 / 删除). The header
 * 视图选项 button opens the grouping/sorting menu; preferences persist in
 * store.settings.
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
  IconPanelLeft, IconPlus, IconSettings, IconChat, IconSearch,
} from '../../components/Icons.tsx'
import { store, subscribe, type Settings } from '../../store.ts'
import {
  createSession, deleteSession, renameSession, selectSession,
  forkSession, setSessionArchived, renameWorkspace, deleteWorkspace,
} from '../../bridge/inferglow.ts'
import { MenuPopover } from '../../panels/MenuPopover.tsx'
import { WorkspaceAddModal } from '../../panels/WorkspaceAddModal.tsx'
import { formatRelTime } from '../../lib/relTime.ts'
import styles from './Sidebar.module.css'

/** Wide-content unmount delay; matches the 150ms wide-content fade-out. */
const COLLAPSE_SETTLE_MS = 150

/** How long the column's scrollbars stay drawn after the pointer leaves it.
 * The bar is a pointer affordance here, and hiding it on the leave event
 * itself makes it blink out while the pointer is only crossing the column's
 * edge — on the way to the conversation, or around a portalled menu.
 */
const SCROLLBAR_LINGER_MS = 2000

/** Which popover is open. anchor is the trigger button (fixed positioning). */
type OpenMenu =
  | { kind: 'view'; anchor: HTMLElement }
  | { kind: 'session'; id: string; anchor: HTMLElement }
  | { kind: 'workspace'; id: string; anchor: HTMLElement }
  | null

/** Inline rename editor target (menu → 重命名). */
interface Renaming {
  kind: 'session' | 'workspace'
  id: string
  value: string
}

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
  const [settings, setSettings] = useState<Settings>(store.settings)
  const [addWsOpen, setAddWsOpen] = useState(false)
  const [openMenu, setOpenMenu] = useState<OpenMenu>(null)
  const [renaming, setRenaming] = useState<Renaming | null>(null)
  const [collapsedGroups, setCollapsedGroups] = useState<Set<string>>(new Set())

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
      setSettings({ ...store.settings })
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
   * and for SCROLLBAR_LINGER_MS after it leaves. */
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

  const filteredSessions = useMemo(() => {
    if (!searchQuery) return sessions
    const q = searchQuery.toLowerCase()
    return sessions.filter(s => s.title.toLowerCase().includes(q))
  }, [sessions, searchQuery])

  /** Sort per the view-options preference (recent = updatedAt desc). */
  const sortedSessions = useMemo(() => {
    const list = [...filteredSessions]
    if (settings.sidebarSort === 'recent') {
      list.sort((a, b) => b.updatedAt - a.updatedAt)
    } else {
      list.sort((a, b) => a.createdAt - b.createdAt)
    }
    return list
  }, [filteredSessions, settings.sidebarSort])

  /** One tree: workspace groups + trailing 未分组 (legacy records), or a
   * single flat group when the view options say 单列表. */
  const groups = useMemo(() => {
    if (settings.sidebarGroupBy === 'flat') {
      return [{ id: '__all__', label: '会话', ws: false, sessions: sortedSessions }]
    }
    return [
      ...workspaces.map(w => ({
        id: w.name,
        label: w.name,
        ws: true,
        // Strict match: sessions without a workspace field belong ONLY to
        // 未分组 — the ?? activeWs fallback made them appear in two groups.
        sessions: sortedSessions.filter(s => s.workspace === w.name),
      })),
      { id: '(unassigned)', label: '未分组', ws: false, sessions: sortedSessions.filter(s => !s.workspace) },
    ]
  }, [workspaces, sortedSessions, settings.sidebarGroupBy])

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

  /* ── View options (分组方式 / 排序方式) ── */
  function pickViewOption(id: string) {
    setOpenMenu(null)
    if (id === 'group:workspace') store.updateSetting('sidebarGroupBy', 'workspace')
    else if (id === 'group:flat') store.updateSetting('sidebarGroupBy', 'flat')
    else if (id === 'sort:recent') store.updateSetting('sidebarSort', 'recent')
    else if (id === 'sort:manual') store.updateSetting('sidebarSort', 'manual')
  }

  /* ── Session ⋯ menu (归档 / Fork / 重命名 / 删除) ── */
  function pickSessionAction(action: string) {
    const menu = openMenu
    setOpenMenu(null)
    if (menu?.kind !== 'session') return
    const id = menu.id
    const session = sessions.find(s => s.id === id)
    if (!session) return
    if (action === 'archive' || action === 'unarchive') {
      setSessionArchived(id, action === 'archive')
    } else if (action === 'fork') {
      void forkSession(id)
    } else if (action === 'rename') {
      setRenaming({ kind: 'session', id, value: session.title })
    } else if (action === 'delete') {
      handleDeleteSession(id)
    }
  }

  /* ── Workspace ⋯ menu (重命名 / 删除) ── */
  function pickWorkspaceAction(action: string) {
    const menu = openMenu
    setOpenMenu(null)
    if (menu?.kind !== 'workspace') return
    const name = menu.id
    if (action === 'rename') {
      setRenaming({ kind: 'workspace', id: name, value: name })
    } else if (action === 'delete') {
      if (confirm(`确定删除工作区「${name}」？其下会话将归入未分组。`)) {
        void deleteWorkspace(name)
      }
    }
  }

  /* ── Inline rename commit (Enter) / cancel (Escape / blur empty) ── */
  function commitRename() {
    const cur = renaming
    setRenaming(null)
    if (!cur) return
    const next = cur.value.trim()
    if (!next) return
    if (cur.kind === 'session') {
      store.renameSession(cur.id, next)
      renameSession(cur.id, next)
    } else if (next !== cur.id) {
      void renameWorkspace(cur.id, next)
    }
  }

  const sidebarClasses = [
    styles.root,
    collapsed && styles.collapsed,
    isRailIn && styles.railIn,
    isFading && styles.fading,
    !pointerInside && styles.quietBars,
  ].filter(Boolean).join(' ')

  const viewSections = [
    {
      label: '分组方式',
      items: [
        { id: 'group:workspace', label: '按工作区', selected: settings.sidebarGroupBy === 'workspace' },
        { id: 'group:flat', label: '单列表', selected: settings.sidebarGroupBy === 'flat' },
      ],
    },
    {
      label: '排序方式',
      items: [
        { id: 'sort:manual', label: '手动排序', selected: settings.sidebarSort === 'manual' },
        { id: 'sort:recent', label: '最近更新', selected: settings.sidebarSort === 'recent' },
      ],
    },
  ]

  const menuSession = openMenu?.kind === 'session' ? sessions.find(s => s.id === openMenu.id) : null
  const sessionMenuSections = menuSession ? [{
    items: [
      menuSession.status === 'archived'
        ? { id: 'unarchive', label: '取消归档' }
        : { id: 'archive', label: '归档' },
      { id: 'fork', label: 'Fork' },
      { id: 'rename', label: '重命名' },
      { id: 'delete', label: '删除', danger: true },
    ],
  }] : []

  const workspaceMenuSections = openMenu?.kind === 'workspace' ? [{
    items: [
      { id: 'rename', label: '重命名工作区' },
      { id: 'delete', label: '删除工作区', danger: true },
    ],
  }] : []

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
            {/* ── Section header: 工作区 + view options (R9: menu wired) ── */}
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
                <button
                  type="button"
                  className="dsh-ws-icon-btn"
                  aria-label="视图选项"
                  title="视图选项"
                  aria-haspopup="menu"
                  onClick={e => {
                    const el = e.currentTarget // capture: currentTarget is nulled after dispatch
                    setOpenMenu(m => (m?.kind === 'view' ? null : { kind: 'view', anchor: el }))
                  }}
                >
                  {/* DSH sliders glyph */}
                  <svg width="18" height="18" viewBox="0 0 16 16" fill="none">
                    <path d="M10.32 9.18c.96 0 1.78.65 2.03 1.53h1.06v1.16h-1.07a2.12 2.12 0 01-2.02 1.53 2.12 2.12 0 01-2.03-1.53H0v-1.16h8.3a2.12 2.12 0 012.02-1.53zm0 1.16a.95.95 0 100 1.9.95.95 0 000-1.9zM3.08 4.59c.97 0 1.78.65 2.03 1.53h8.3v1.16h-8.3a2.12 2.12 0 01-2.03 1.53 2.12 2.12 0 01-2.02-1.53H0V6.12h1.06a2.12 2.12 0 012.02-1.53zm0 1.16a.95.95 0 100 1.9.95.95 0 000-1.9z" fill="currentColor"/>
                  </svg>
                </button>
                <button type="button" className="dsh-ws-icon-btn" aria-label="添加工作区" title="添加工作区" onClick={() => setAddWsOpen(true)}>
                  <IconPlus size={18} />
                </button>
              </div>
            </div>

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

            {/* ── Session tree: one group per workspace (DSH-aligned, R9) ── */}
            <div className="dsh-ws-list-area">
              <div className="dsh-ws-tree">
                <div className="dsh-ws-groups">
                  {groups.map(group => {
                    const isCollapsed = collapsedGroups.has(group.id)
                    return (
                      <div className="dsh-ws-group" key={group.id}>
                        {/* Group row = workspace row (folder + ⋯ + ＋) */}
                        <div
                          className={`dsh-ws-row dsh-ws-row-group${group.ws && activeWs === group.id ? ' dsh-ws-row-active' : ''}`}
                          role="treeitem"
                          aria-expanded={!isCollapsed}
                          title={group.ws ? (workspaces.find(w => w.name === group.id)?.root || '(server 默认目录)') : undefined}
                          onClick={() => {
                            if (group.ws) store.setActiveWorkspace(group.id)
                            handleToggleGroup(group.id)
                          }}
                        >
                          <span className="dsh-ws-row-icon">
                            <svg width="13" height="13" viewBox="0 0 16 16" fill="none">
                              <path d="M2.3 3.5a1.2 1.2 0 011.2-1.2h3l1.1 1.4h5.2a1.2 1.2 0 011.2 1.2v1h.2a1.2 1.2 0 011 1.7l-1.1 3.2a1.2 1.2 0 01-1.1.8H3.2a1.2 1.2 0 01-1.2-1.5l.3-4.6z" stroke="currentColor" strokeWidth="1.2" strokeLinejoin="round"/>
                            </svg>
                          </span>
                          <span className={`dsh-ws-group-caret${isCollapsed ? ' dsh-ws-group-caret-collapsed' : ''}`}>
                            <svg width="10" height="10" viewBox="0 0 12 12" fill="none">
                              <path d="M4 2.5L8.5 6 4 9.5" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round"/>
                            </svg>
                          </span>
                          <span className="dsh-ws-row-title">{group.label}</span>
                          {group.ws && (
                            <span className="dsh-ws-row-actions">
                              <button
                                className="dsh-ws-row-delete"
                                title={`工作区「${group.label}」的操作`}
                                aria-label={`工作区「${group.label}」的操作`}
                                onClick={e => {
                                  e.stopPropagation()
                                  const el = e.currentTarget
                                  setOpenMenu(m =>
                                    m?.kind === 'workspace' && m.id === group.id ? null : { kind: 'workspace', id: group.id, anchor: el })
                                }}
                              >
                                <DotsIcon />
                              </button>
                              <button
                                className="dsh-ws-row-delete"
                                title={`在「${group.label}」中新建会话`}
                                aria-label={`在「${group.label}」中新建会话`}
                                onClick={e => {
                                  e.stopPropagation()
                                  store.setActiveWorkspace(group.id)
                                  void createSession(group.id)
                                }}
                              >
                                <svg width="11" height="11" viewBox="0 0 12 12" fill="none">
                                  <path d="M6 1.5v9M1.5 6h9" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round"/>
                                </svg>
                              </button>
                            </span>
                          )}
                        </div>

                        {!isCollapsed && (
                          <div className="dsh-ws-rows">
                            {group.sessions.map(session => (
                              <div
                                key={session.id}
                                className={`dsh-ws-row${activeId === session.id ? ' dsh-ws-row-active' : ''}${session.status === 'archived' ? ' dsh-ws-row-archived' : ''}`}
                                onClick={() => handleSelectSession(session.id)}
                              >
                                <span className="dsh-ws-row-icon"><IconChat size={13} /></span>
                                {renaming?.kind === 'session' && renaming.id === session.id ? (
                                  <input
                                    className="dsh-ws-rename-input"
                                    value={renaming.value}
                                    autoFocus
                                    onClick={e => e.stopPropagation()}
                                    onChange={e => setRenaming({ ...renaming, value: e.target.value })}
                                    onBlur={() => commitRename()}
                                    onKeyDown={e => {
                                      if (e.key === 'Enter') commitRename()
                                      else if (e.key === 'Escape') setRenaming(null)
                                    }}
                                  />
                                ) : (
                                  <span className="dsh-ws-row-title" title={session.title}>{session.title}</span>
                                )}
                                <span className="dsh-ws-row-time">{formatRelTime(session.updatedAt)}</span>
                                <span className="dsh-ws-row-actions">
                                  <button
                                    className="dsh-ws-row-delete"
                                    title={`会话「${session.title}」的操作`}
                                    aria-label={`会话「${session.title}」的操作`}
                                    onClick={e => {
                                      e.stopPropagation()
                                      const el = e.currentTarget
                                      setOpenMenu(m =>
                                        m?.kind === 'session' && m.id === session.id ? null : { kind: 'session', id: session.id, anchor: el })
                                    }}
                                  >
                                    <DotsIcon />
                                  </button>
                                </span>
                              </div>
                            ))}
                            {group.sessions.length === 0 && (
                              <div className="dsh-ws-empty">
                                {group.ws && group.id !== '(unassigned)' ? '在此 workspace 开启对话' : '暂无对话'}
                              </div>
                            )}
                          </div>
                        )}
                      </div>
                    )
                  })}
                  {workspaces.length === 0 && settings.sidebarGroupBy === 'workspace' && (
                    <div className="dsh-ws-empty">server 未注册 workspace</div>
                  )}
                </div>
              </div>
            </div>
          </div>
        )}
      </div>

      {/* ── Portalled menus (view options / row actions) ── */}
      {openMenu?.kind === 'view' && (
        <MenuPopover anchor={openMenu.anchor} sections={viewSections} onPick={pickViewOption} onClose={() => setOpenMenu(null)} />
      )}
      {openMenu?.kind === 'session' && (
        <MenuPopover anchor={openMenu.anchor} sections={sessionMenuSections} onPick={pickSessionAction} onClose={() => setOpenMenu(null)} />
      )}
      {openMenu?.kind === 'workspace' && (
        <MenuPopover anchor={openMenu.anchor} sections={workspaceMenuSections} onPick={pickWorkspaceAction} onClose={() => setOpenMenu(null)} />
      )}

      {/* ── Inline workspace rename editor (outside the tree, so it survives
          re-grouping while the PATCH refreshes the registry) ── */}
      {renaming?.kind === 'workspace' && (
        <div
          style={{
            position: 'fixed', inset: 0, zIndex: 999,
            display: 'flex', alignItems: 'center', justifyContent: 'center',
            background: 'rgba(0,0,0,.35)',
          }}
          onClick={() => setRenaming(null)}
        >
          <div
            style={{
              background: 'var(--dsw-surface-raised, #1e2433)', color: 'inherit',
              border: '1px solid color-mix(in srgb, currentColor 18%, transparent)',
              borderRadius: 12, padding: 16, width: 320, display: 'grid', gap: 10,
            }}
            onClick={e => e.stopPropagation()}
          >
            <div style={{ fontSize: 13, fontWeight: 600 }}>重命名工作区「{renaming.id}」</div>
            <input
              className="dsh-ws-rename-input"
              style={{ padding: '6px 10px' }}
              value={renaming.value}
              autoFocus
              onChange={e => setRenaming({ ...renaming, value: e.target.value })}
              onKeyDown={e => {
                if (e.key === 'Enter') commitRename()
                else if (e.key === 'Escape') setRenaming(null)
              }}
            />
            <div style={{ fontSize: 11, opacity: 0.6 }}>其下会话将跟随改组；根目录不变。</div>
            <button
              type="button"
              className="dsh-pane-primary-btn"
              onClick={() => commitRename()}
            >
              确认重命名
            </button>
          </div>
        </div>
      )}

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

/** ⋯ glyph (three dots) shared by the row-action buttons. */
function DotsIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 16 16" fill="none">
      <path d="M4.55 8a1.15 1.15 0 11-2.3 0 1.15 1.15 0 012.3 0zM9.15 8a1.15 1.15 0 11-2.3 0 1.15 1.15 0 012.3 0zM13.75 8a1.15 1.15 0 11-2.3 0 1.15 1.15 0 012.3 0z" fill="currentColor"/>
    </svg>
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
