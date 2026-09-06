import { useCallback, useRef, useState } from 'react'
import { useSessionStore } from '../stores/sessionStore'
import { SlotOutlet, useSession, useSessions } from '../framework'
import { SessionTree, type SortMode, type GroupMode } from './SessionTree'
import styles from './sidebar.module.css'
import './sidebarSlots' // registers the sidebar.* plugin slots (side effect)

export interface SidebarRootProps {
  /** Rail-collapsed state (driven by AppFrame). */
  collapsed?: boolean
  onToggle?: () => void
  onOpenSettings?: () => void
}

function SearchBox({ q, onChange }: { q: string; onChange: (v: string) => void }) {
  return (
    <div className={styles.search}>
      <span className={styles.searchIcon}>🔍</span>
      <input
        placeholder="搜索会话（标题/工作区）…"
        value={q}
        onChange={(e) => onChange(e.target.value)}
        autoComplete="off"
      />
      {q && (
        <button className={styles.searchClear} onClick={() => onChange('')} title="清空">
          ✕
        </button>
      )}
    </div>
  )
}

function ViewToggles({
  groupMode,
  sortMode,
  onGroupMode,
  onSortMode,
}: {
  groupMode: GroupMode
  sortMode: SortMode
  onGroupMode: (m: GroupMode) => void
  onSortMode: (m: SortMode) => void
}) {
  return (
    <div className={styles.viewBar}>
      <div className={styles.seg}>
        <button
          className={groupMode === 'group' ? styles.segBtnOn : styles.segBtn}
          onClick={() => onGroupMode('group')}
        >
          分组
        </button>
        <button
          className={groupMode === 'flat' ? styles.segBtnOn : styles.segBtn}
          onClick={() => onGroupMode('flat')}
        >
          不分组
        </button>
      </div>
      <select
        className={styles.sortSelect}
        value={sortMode}
        onChange={(e) => onSortMode(e.target.value as SortMode)}
        title="排序"
      >
        <option value="pinned">置顶优先</option>
        <option value="recent">最近更新</option>
        <option value="title">标题</option>
      </select>
    </div>
  )
}

function Rail({ onExpand }: { onExpand?: () => void }) {
  return (
    <div className={styles.rail} onClick={onExpand} title="展开侧边栏">
      <span className={styles.railLogo}>◈</span>
      <span className={styles.railArrow}>»</span>
    </div>
  )
}

export function SidebarRoot({ collapsed = false, onToggle, onOpenSettings }: SidebarRootProps) {
  const [q, setQ] = useState('')
  const [groupMode, setGroupMode] = useState<GroupMode>('group')
  const [sortMode, setSortMode] = useState<SortMode>('recent')
  const sessions = useSessionStore((st) => st.sessions)
  const activeId = useSessionStore((st) => st.activeId)
  const createSession = useSessionStore((st) => st.createSession)
  const select = useSessionStore((st) => st.select)
  const active = useSession()

  // Remote fetch (with optional backend filters) + local grouping/sorting.
  const { sessions: listed, byGroup, loading } = useSessions({ q })

  // Scrollbar linger: reveal for 2s after the last scroll, then hide.
  const [linger, setLinger] = useState(false)
  const lingerTimer = useRef<number>(0)
  const handleScroll = useCallback(() => {
    setLinger(true)
    window.clearTimeout(lingerTimer.current)
    lingerTimer.current = window.setTimeout(() => setLinger(false), 2000)
  }, [])

  const onNew = useCallback(async () => {
    const rec = await createSession(active?.agent_id ?? 'a1', `新会话 ${sessions.length + 1}`)
    if (rec) select(rec.id)
  }, [createSession, select, sessions.length, active])

  if (collapsed) {
    return <Rail onExpand={onToggle} />
  }

  return (
    <div className={styles.root}>
      <div className={styles.header}>
        <span className={styles.logo}>◈</span>
        <span className={styles.brand}>
          Infer<b>Glow</b>
        </span>
        <button className={styles.collapseBtn} onClick={onToggle} title="折叠侧边栏">
          ◀
        </button>
      </div>

      <button className={styles.newBtn} onClick={() => void onNew()}>
        ＋ 新建会话
      </button>

      <SearchBox q={q} onChange={setQ} />
      <ViewToggles
        groupMode={groupMode}
        sortMode={sortMode}
        onGroupMode={setGroupMode}
        onSortMode={setSortMode}
      />

      {/* Scrollable middle: workspace area + session tree.
          Scrollbar lingers ~2s after the last scroll (Task 8). */}
      <div className={`${styles.scrollArea}${linger ? ` ${styles.scrollAreaVisible}` : ''}`} onScroll={handleScroll}>
        {/* Workspace area — extensible via the sidebar.workspaces slot */}
        <div className={styles.workspaceRegion}>
          <SlotOutlet name="sidebar.workspaces" props={{ groupMode }} />
        </div>

        <SessionTree
          sessions={listed}
          byGroup={byGroup}
          groupMode={groupMode}
          sortMode={sortMode}
          activeId={activeId}
          loading={loading}
          onSelect={select}
        />
      </div>

      <div className={styles.footer}>
        <SlotOutlet name="sidebar.settings" props={{ onOpenSettings }} />
        <SlotOutlet
          name="sidebar.footer.action"
          props={{ onOpenSettings, onNewSession: onNew, onToggle, collapsed, sessionCount: sessions.length }}
        />
      </div>
    </div>
  )
}