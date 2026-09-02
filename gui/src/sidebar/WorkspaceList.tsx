import { useMemo, useState } from 'react'
import { useSessionStore } from '../stores/sessionStore'
import { UNGROUPED_LABEL } from '../framework'
import styles from './sidebar.module.css'
import type { GroupMode } from './SessionTree'

/**
 * Workspace management pane (registered into the `sidebar.workspaces` slot).
 * Lists distinct session groups with create / rename / delete / re-order
 * (▲▼), operating on SessionRecord.group via the session store.
 */
export function WorkspaceList({ groupMode }: { groupMode?: GroupMode }) {
  const sessions = useSessionStore((st) => st.sessions)
  const updateMeta = useSessionStore((st) => st.updateMeta)
  const [order, setOrder] = useState<string[]>([])

  // Known workspaces = user-ordered list ∪ session groups discovered later.
  const present = useMemo(() => {
    const seen: string[] = []
    for (const s of sessions) {
      const g = s.group || UNGROUPED_LABEL
      if (!seen.includes(g)) seen.push(g)
    }
    return seen
  }, [sessions])

  const workspaces = useMemo(() => {
    const list = [...order]
    for (const g of present) if (!list.includes(g)) list.push(g)
    return list
  }, [present, order])

  const count = (g: string) => sessions.filter((s) => (s.group || UNGROUPED_LABEL) === g).length

  const move = (index: number, dir: -1 | 1) => {
    const j = index + dir
    if (j < 0 || j >= workspaces.length) return
    const next = [...workspaces]
    ;[next[index], next[j]] = [next[j], next[index]]
    setOrder(next)
  }

  const create = () => {
    const name = window.prompt('新工作区名称', '')?.trim()
    if (name && !workspaces.includes(name)) setOrder((o) => [...o, name])
  }

  const rename = (oldName: string) => {
    if (oldName === UNGROUPED_LABEL) return
    const fresh = window.prompt('重命名工作区', oldName)?.trim()
    if (!fresh || fresh === oldName) return
    for (const s of sessions) {
      if ((s.group || UNGROUPED_LABEL) === oldName) void updateMeta(s.id, { group: fresh })
    }
    setOrder((o) => o.map((x) => (x === oldName ? fresh : x)))
  }

  const remove = (g: string) => {
    if (g === UNGROUPED_LABEL) return
    // Merge members back into 未归类 (empty group unmaps to falsy).
    for (const s of sessions) {
      if ((s.group || UNGROUPED_LABEL) === g) void updateMeta(s.id, { group: '' })
    }
    setOrder((o) => o.filter((x) => x !== g))
  }

  if (groupMode !== 'group') return null

  return (
    <div className={styles.workspaces}>
      <div className={styles.workspacesHead}>
        <span className={styles.workspacesTitle}>工作区</span>
        <button className={styles.addWorkspace} onClick={create} title="新建工作区">＋</button>
      </div>
      {workspaces.map((g, i) => (
        <div className={styles.workspaceRow} key={g}>
          <span className={styles.workspaceName} title={g}>{g}</span>
          <span className={styles.workspaceCount}>{count(g)}</span>
          <span className={styles.workspaceActions}>
            <button onClick={() => move(i, -1)} title="上移" disabled={i === 0}>▲</button>
            <button onClick={() => move(i, 1)} title="下移" disabled={i === workspaces.length - 1}>▼</button>
            <button onClick={() => rename(g)} title="重命名">✎</button>
            <button onClick={() => remove(g)} title="删除" disabled={g === UNGROUPED_LABEL}>🗑</button>
          </span>
        </div>
      ))}
    </div>
  )
}