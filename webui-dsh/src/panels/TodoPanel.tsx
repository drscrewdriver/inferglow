/**
 * TodoPanel — 待办管理. Reads and writes the SAME server task store the
 * model's task_tracker tools use (/v1/tasks backed by actions.TaskStore),
 * so a `task_add` inside a conversation shows up here after a refresh (or
 * the panel's manual reload), and checking items off here is visible to the
 * model's next task_list. Polls on mount + reload button; the session event
 * bus is a planned follow-up.
 */

import { useEffect, useState } from 'react'
import { api } from '../bridge/inferglow.ts'
import type { TaskItem } from '../api/client.ts'

const STATUS_ORDER: { id: string; label: string; color: string }[] = [
  { id: 'pending', label: '待办', color: '#8a8f98' },
  { id: 'in_progress', label: '进行中', color: '#4f7cff' },
  { id: 'done', label: '已完成', color: '#98c379' },
]

export function TodoPanel() {
  const [tasks, setTasks] = useState<TaskItem[] | null>(null)
  const [draft, setDraft] = useState('')
  const [err, setErr] = useState<string | null>(null)
  /** Inline title editor: the id of the task being renamed. */
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editDraft, setEditDraft] = useState('')

  const reload = () => {
    api.listTasks()
      .then(t => { setTasks(t); setErr(null) })
      .catch(e => setErr(String((e as Error)?.message ?? e)))
  }
  useEffect(reload, [])

  const add = async () => {
    const title = draft.trim()
    if (!title) return
    try {
      await api.createTask(title)
      setDraft('')
      reload()
    } catch (e) {
      setErr(String((e as Error)?.message ?? e))
    }
  }
  const cycle = async (t: TaskItem) => {
    const next = t.status === 'pending' ? 'in_progress' : t.status === 'in_progress' ? 'done' : 'pending'
    try {
      await api.updateTask(t.id, { status: next })
      reload()
    } catch (e) {
      setErr(String((e as Error)?.message ?? e))
    }
  }
  const remove = async (id: string) => {
    try {
      await api.deleteTask(id)
      reload()
    } catch (e) {
      setErr(String((e as Error)?.message ?? e))
    }
  }
  /** Commit the inline title edit (PATCH title — server-side supported). */
  const commitEdit = async (id: string) => {
    const title = editDraft.trim()
    setEditingId(null)
    if (!title) return
    try {
      await api.updateTask(id, { title })
      reload()
    } catch (e) {
      setErr(String((e as Error)?.message ?? e))
    }
  }

  return (
    <div style={{ padding: '10px 14px', overflowY: 'auto', height: '100%', boxSizing: 'border-box', color: 'var(--dsw-alias-label-primary)' }}>
      <div style={{ display: 'flex', gap: 6, marginBottom: 10 }}>
        <input
          value={draft}
          onChange={e => setDraft(e.target.value)}
          onKeyDown={e => { if (e.key === 'Enter') void add() }}
          placeholder="新增待办，回车确认"
          style={{
            flex: 1, padding: '6px 10px', borderRadius: 8, fontSize: 12.5,
            border: '1px solid color-mix(in srgb, currentColor 18%, transparent)',
            background: 'transparent', color: 'var(--dsw-alias-label-primary)', outline: 'none',
          }}
        />
        <button type="button" className="dsh-pane-iconbtn" title="刷新（含模型侧新增）" onClick={reload}>↻</button>
      </div>
      {err && <div style={{ color: '#d4544a', fontSize: 12, marginBottom: 8 }}>⚠ {err}</div>}
      {tasks === null && <div style={{ color: 'var(--dsw-alias-label-secondary)', fontSize: 12 }}>加载中…</div>}
      {tasks !== null && tasks.length === 0 && (
        <div style={{ color: 'var(--dsw-alias-label-secondary)', fontSize: 12, lineHeight: 1.7 }}>
          暂无待办。对话框里让模型 task_add，或直接在上方输入。
        </div>
      )}
      {tasks?.map(t => {
        const meta = STATUS_ORDER.find(s => s.id === t.status) ?? STATUS_ORDER[0]
        return (
          <div
            key={t.id}
            style={{
              display: 'flex', alignItems: 'center', gap: 8, padding: '7px 10px', marginBottom: 6,
              borderRadius: 8, background: 'color-mix(in srgb, var(--dsw-alias-label-secondary) 8%, transparent)',
            }}
          >
            <button
              type="button"
              onClick={() => void cycle(t)}
              title="点击切换状态：待办 → 进行中 → 完成"
              style={{
                width: 16, height: 16, borderRadius: '50%', cursor: 'pointer', flexShrink: 0,
                border: `1.6px solid ${meta.color}`,
                background: t.status === 'done' ? meta.color : 'transparent',
              }}
            />
            {editingId === t.id ? (
              <input
                autoFocus
                value={editDraft}
                onChange={e => setEditDraft(e.target.value)}
                onBlur={() => void commitEdit(t.id)}
                onKeyDown={e => {
                  if (e.key === 'Enter') void commitEdit(t.id)
                  else if (e.key === 'Escape') setEditingId(null)
                }}
                style={{
                  flex: 1, minWidth: 0, fontSize: 12.5, padding: '2px 6px',
                  background: 'transparent', color: 'inherit', outline: 'none',
                  border: '1px solid color-mix(in srgb, currentColor 30%, transparent)', borderRadius: 4,
                }}
              />
            ) : (
              <span
                style={{
                  flex: 1, fontSize: 12.5,
                  color: 'var(--dsw-alias-label-primary)',
                  textDecoration: t.status === 'done' ? 'line-through' : 'none',
                  opacity: t.status === 'done' ? 0.7 : 1,
                  overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
                }}>
                {t.title}
                {t.priority >= 1 && <span style={{ color: '#e5c07b', marginLeft: 6, fontSize: 11 }}>高优</span>}
              </span>
            )}
            <span style={{ fontSize: 11, color: meta.color, flexShrink: 0, opacity: 0.95 }}>{meta.label}</span>
            <button
              type="button"
              className="dsh-ws-row-delete"
              title="编辑"
              aria-label={`编辑「${t.title}」`}
              onClick={() => { setEditingId(t.id); setEditDraft(t.title) }}
            >
              <svg width="11" height="11" viewBox="0 0 12 12" fill="none">
                <path d="M8.5 1.5l2 2L4 10l-2.5.5L2 8l6.5-6.5z" stroke="currentColor" strokeWidth="1.2" strokeLinejoin="round"/>
              </svg>
            </button>
            <button type="button" className="dsh-ws-row-delete" title="删除" onClick={() => void remove(t.id)}>
              <svg width="11" height="11" viewBox="0 0 12 12" fill="none">
                <path d="M2.5 2.5l7 7m0-7l-7 7" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round"/>
              </svg>
            </button>
          </div>
        )
      })}
    </div>
  )
}
