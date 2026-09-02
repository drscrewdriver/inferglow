import { useMemo, useState } from 'react'
import type { SessionRecord } from '../api'
import { useSessionStore } from '../stores/sessionStore'
import { sortSessions, UNGROUPED_LABEL, type SessionGroup } from '../framework'
import styles from './sidebar.module.css'

export type SortMode = 'pinned' | 'recent' | 'title'
export type GroupMode = 'group' | 'flat'

const COLLAPSE_THRESHOLD = 5

export interface SessionTreeProps {
  sessions: SessionRecord[]
  byGroup: SessionGroup[]
  groupMode: GroupMode
  sortMode: SortMode
  activeId: string | null
  loading: boolean
  onSelect: (id: string) => void
}

function SessionMenuItem({ s, active, onSelect }: { s: SessionRecord; active: boolean; onSelect: (id: string) => void }) {
  const updateMeta = useSessionStore((st) => st.updateMeta)
  const removeSession = useSessionStore((st) => st.removeSession)
  const [open, setOpen] = useState(false)

  const doMenu = (action: string) => {
    setOpen(false)
    if (action === 'pin') void updateMeta(s.id, { pinned: true })
    else if (action === 'unpin') void updateMeta(s.id, { pinned: false })
    else if (action === 'archive') void updateMeta(s.id, { status: 'archived' })
    else if (action === 'restore') void updateMeta(s.id, { status: 'active' })
    else if (action === 'delete') void removeSession(s.id)
    else if (action === 'rename') {
      const title = window.prompt('重命名会话', s.title ?? '')
      if (title) void updateMeta(s.id, { title })
    } else if (action === 'group') {
      const group = window.prompt('设置会话工作区（留空为未归类）', s.group ?? '')
      if (group !== null) void updateMeta(s.id, { group })
    }
  }

  const statusIcon = s.status === 'archived' ? '🗄' : s.pinned ? '📌' : ''

  return (
    <div
      className={`${styles.sessionItem}${active ? ` ${styles.sessionItemActive}` : ''}`}
      onClick={() => onSelect(s.id)}
      title={`${s.title ?? '未命名'} · ${s.group || UNGROUPED_LABEL}`}
    >
      <span className={`${styles.avatar}${s.group ? ` ${styles.avatarProj}` : ''}`}>
        {(s.title ?? '话').trim().charAt(0)}
      </span>
      <span className={styles.sessionBody}>
        <span className={styles.sessionTitle}>{s.title ?? '未命名会话'}</span>
        <span className={styles.sessionMeta}>{s.agent_id ?? ''}</span>
      </span>
      {statusIcon && <span className={styles.pin}>{statusIcon}</span>}
      <span className={styles.moreHost}>
        <button
          className={styles.moreBtn}
          onClick={(e) => {
            e.stopPropagation()
            setOpen((v) => !v)
          }}
          title="更多操作"
        >
          ⋯
        </button>
        {open && (
          <div className={styles.menu} onClick={(e) => e.stopPropagation()}>
            {s.pinned
              ? <button onClick={() => doMenu('unpin')}>取消置顶</button>
              : <button onClick={() => doMenu('pin')}>置顶</button>}
            <button onClick={() => doMenu('rename')}>重命名</button>
            <button onClick={() => doMenu('group')}>移动工作区</button>
            {s.status === 'archived'
              ? <button onClick={() => doMenu('restore')}>恢复</button>
              : <button onClick={() => doMenu('archive')}>归档</button>}
            <button onClick={() => doMenu('delete')} className={styles.menuDanger}>删除</button>
          </div>
        )}
      </span>
    </div>
  )
}

interface GroupSectionProps {
  label: string
  sessions: SessionRecord[]
  activeId: string | null
  onSelect: (id: string) => void
}

function GroupSection({ label, sessions, activeId, onSelect }: GroupSectionProps) {
  const [expanded, setExpanded] = useState(false)
  const collapsed = sessions.length > COLLAPSE_THRESHOLD && !expanded
  const shown = collapsed ? sessions.slice(0, COLLAPSE_THRESHOLD) : sessions
  return (
    <div className={styles.group}>
      <div className={styles.groupLabel}>
        <span className={styles.groupName}>{label}</span>
        <span className={styles.groupCount}>{sessions.length}</span>
      </div>
      {shown.map((s) => (
        <SessionMenuItem key={s.id} s={s} active={s.id === activeId} onSelect={onSelect} />
      ))}
      {collapsed && (
        <button className={styles.groupToggle} onClick={() => setExpanded(true)}>
          显示全部 {sessions.length}
        </button>
      )}
      {sessions.length > COLLAPSE_THRESHOLD && expanded && (
        <button className={styles.groupToggle} onClick={() => setExpanded(false)}>收起</button>
      )}
    </div>
  )
}

export function SessionTree({ sessions, byGroup, groupMode, sortMode, activeId, loading, onSelect }: SessionTreeProps) {
  const tree = useMemo(() => {
    const applySort = (list: SessionRecord[]) =>
      sortSessions(list, sortMode === 'title' ? 'title' : sortMode === 'recent' ? 'updated' : 'updated')
    if (groupMode === 'group') {
      return byGroup.map((g) => ({ label: g.group, sessions: applySort(g.sessions) }))
    }
    return [{ label: '全部会话', sessions: applySort(sessions) }]
  }, [byGroup, sessions, groupMode, sortMode])

  if (loading && sessions.length === 0) {
    return <div className={styles.empty}>加载会话…</div>
  }

  return (
    <div className={styles.tree}>
      {tree.map((g) => (
        <GroupSection key={g.label} label={g.label} sessions={g.sessions} activeId={activeId} onSelect={onSelect} />
      ))}
      {sessions.length === 0 && !loading && <div className={styles.empty}>暂无会话，点击上方新建。</div>}
    </div>
  )
}