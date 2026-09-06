/**
 * SubagentPanel — the 任务管理 (subagent management) tool, seventh entry of
 * the panel "+" menu (R9). Two sections, both real-data:
 *
 * - 子代理: GET /v1/subagents?session= — the server SubagentRegistry rows the
 *   spawn_agent action records (R9 Phase 0). Live rows poll every 4s so a
 *   running (synchronous) sub-agent is visible while its parent run blocks.
 * - 运行记录: the session's persisted run summaries (GET /v1/sessions/{id}/trace)
 *   — every agent run with duration / usage / error; click expands spans.
 *
 * Row presentation follows butter-side-bar's jobs section: status dot,
 * live rows first, finished rows newest-first.
 */

import { useEffect, useMemo, useState } from 'react'
import { api } from '../bridge/inferglow.ts'
import { store, subscribe } from '../store.ts'
import type { SessionTrace, SpawnRecord } from '../api/client.ts'

const POLL_MS = 4000

interface RunRow {
  id: string
  agentId: string
  startMs: number
  endMs: number
  error: string
  usage?: { prompt_tokens?: number; completion_tokens?: number; total_tokens?: number }
  spans: { kind: string; name: string; duration_ms: number; error?: boolean }[]
}

/** Two-adjacent-unit duration (「2分3秒」). */
function fmtDuration(ms: number): string {
  const total = Math.max(0, Math.floor(ms / 1000))
  if (total < 60) return `${total}秒`
  const m = Math.floor(total / 60)
  const s = total % 60
  if (m < 60) return `${m}分${s}秒`
  return `${Math.floor(m / 60)}时${m % 60}分`
}

function fmtClock(ms: number): string {
  const d = new Date(ms)
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}:${String(d.getSeconds()).padStart(2, '0')}`
}

function Dot({ state }: { state: 'ongoing' | 'done' | 'error' }) {
  const color = state === 'ongoing' ? '#4f7cff' : state === 'done' ? '#3fb96f' : '#d4544a'
  return (
    <span
      style={{
        width: 8, height: 8, borderRadius: '50%', flexShrink: 0, background: color,
        boxShadow: state === 'ongoing' ? '0 0 6px currentColor' : 'none',
        opacity: state === 'ongoing' ? 1 : 0.85,
      }}
    />
  )
}

function parseRuns(traces: SessionTrace[]): RunRow[] {
  const rows: RunRow[] = []
  for (const t of traces) {
    let parsed: {
      agent_id?: string
      start?: string
      duration?: string
      spans?: { kind: string; name: string; duration_ms: number; error?: boolean }[]
      usage?: RunRow['usage']
      error?: string
    } = {}
    try { parsed = JSON.parse(t.content) } catch { continue }
    const startMs = parsed.start ? Date.parse(parsed.start) : Date.parse(t.created_at)
    let durMs = 0
    if (parsed.duration) {
      // Go duration string ("1.234s" / "1m2.3s" / "12ms").
      const m = parsed.duration.match(/^([\d.]+)(ms|s|m|h)/)
      if (m) {
        const v = parseFloat(m[1])
        durMs = m[2] === 'ms' ? v : m[2] === 's' ? v * 1000 : m[2] === 'm' ? v * 60_000 : v * 3_600_000
      }
    }
    rows.push({
      id: t.id,
      agentId: parsed.agent_id ?? '?',
      startMs: Number.isNaN(startMs) ? Date.parse(t.created_at) : startMs,
      endMs: (Number.isNaN(startMs) ? Date.parse(t.created_at) : startMs) + durMs,
      error: parsed.error ?? '',
      usage: parsed.usage,
      spans: parsed.spans ?? [],
    })
  }
  // Finished runs newest-first (live runs never appear here — the summary is
  // written at completion).
  return rows.sort((a, b) => b.endMs - a.endMs)
}

export function SubagentPanel() {
  const [sessionId, setSessionId] = useState<string | null>(null)
  const [spawns, setSpawns] = useState<SpawnRecord[] | null>(null)
  const [spawnErr, setSpawnErr] = useState<string | null>(null)
  const [runs, setRuns] = useState<RunRow[] | null>(null)
  const [expanded, setExpanded] = useState<string | null>(null)
  const [now, setNow] = useState(() => Date.now())

  // Track the active session via a tiny store subscription.
  useEffect(() => {
    let last = store.activeSessionId
    setSessionId(last)
    const unsub = subscribe(() => {
      const cur = store.activeSessionId
      if (cur !== last) { last = cur; setSessionId(cur); setExpanded(null) }
    })
    return unsub
  }, [])

  // Poll spawns + run summaries; tick `now` so live rows' durations move.
  useEffect(() => {
    let alive = true
    const load = () => {
      if (!sessionId) { setSpawns([]); setRuns([]); return }
      api.listSubagents(sessionId)
        .then(list => { if (alive) { setSpawns(list); setSpawnErr(null) } })
        .catch(e => { if (alive) setSpawnErr(String((e as Error)?.message ?? e)) })
      api.sessionTrace(sessionId)
        .then(traces => { if (alive) setRuns(parseRuns(traces)) })
        .catch(() => { if (alive) setRuns([]) })
      setNow(Date.now())
    }
    load()
    const timer = window.setInterval(load, POLL_MS)
    return () => { alive = false; window.clearInterval(timer) }
  }, [sessionId])

  const spawnRows = useMemo(() => spawns ?? [], [spawns])
  const runRows = useMemo(() => runs ?? [], [runs])
  const empty = spawnRows.length === 0 && runRows.length === 0

  return (
    <div style={{ padding: '10px 14px', overflowY: 'auto', height: '100%', boxSizing: 'border-box', fontSize: 12.5, color: 'var(--dsw-alias-label-primary)' }}>
      {spawnErr && <div style={{ color: '#d4544a', marginBottom: 8 }}>⚠ {spawnErr}</div>}

      {/* ── 子代理 ── */}
      <div style={{ color: 'var(--dsw-alias-label-secondary)', fontSize: 11.5, margin: '2px 0 6px' }}>子代理（spawn_agent）</div>
      {spawnRows.length === 0 && (
        <div style={{ color: 'var(--dsw-alias-label-secondary)', fontSize: 12, lineHeight: 1.7, marginBottom: 14 }}>
          暂无子代理。会话中让模型调用 spawn_agent 派生子任务，这里实时显示其状态与耗时。
        </div>
      )}
      {spawnRows.map(sp => {
        const running = sp.status === 'running'
        const dur = running ? now - sp.started_at : (sp.ended_at ?? sp.started_at) - sp.started_at
        return (
          <div
            key={sp.id}
            style={{
              display: 'flex', alignItems: 'flex-start', gap: 8, padding: '7px 10px', marginBottom: 6,
              borderRadius: 8, background: 'color-mix(in srgb, var(--dsw-alias-label-secondary) 8%, transparent)',
            }}
          >
            <span style={{ paddingTop: 4 }}><Dot state={running ? 'ongoing' : sp.status === 'error' ? 'error' : 'done'} /></span>
            <div style={{ flex: 1, minWidth: 0 }}>
              <div style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                {sp.task}
              </div>
              <div style={{ fontSize: 11, color: 'var(--dsw-alias-label-secondary)', marginTop: 2, display: 'flex', gap: 8 }}>
                <span>{running ? '运行中' : sp.status === 'error' ? '失败' : '已完成'}</span>
                <span>· {fmtDuration(dur)}{running ? '…' : ''}</span>
                <span>· {fmtClock(sp.started_at)}</span>
              </div>
              {sp.error && <div style={{ fontSize: 11, color: '#d4544a', marginTop: 2, wordBreak: 'break-all' }}>{sp.error}</div>}
              {!sp.error && sp.result && (
                <div style={{ fontSize: 11, color: 'var(--dsw-alias-label-secondary)', marginTop: 2, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                  {sp.result}
                </div>
              )}
            </div>
          </div>
        )
      })}

      {/* ── 运行记录 ── */}
      <div style={{ color: 'var(--dsw-alias-label-secondary)', fontSize: 11.5, margin: '10px 0 6px' }}>运行记录（每次 agent run）</div>
      {runRows.length === 0 && (
        <div style={{ color: 'var(--dsw-alias-label-secondary)', fontSize: 12, lineHeight: 1.7 }}>
          {empty ? '当前会话暂无运行记录，发送消息后这里显示每次 agent run。' : '暂无已完成的运行。'}
        </div>
      )}
      {runRows.map(run => {
        const isOpen = expanded === run.id
        return (
          <div key={run.id} style={{ marginBottom: 6 }}>
            <button
              type="button"
              onClick={() => setExpanded(isOpen ? null : run.id)}
              style={{
                display: 'flex', alignItems: 'center', gap: 8, width: '100%', textAlign: 'left',
                padding: '7px 10px', borderRadius: 8, border: 'none', cursor: 'pointer',
                background: 'color-mix(in srgb, var(--dsw-alias-label-secondary) 8%, transparent)',
                color: 'inherit', font: 'inherit', fontSize: 12.5,
              }}
            >
              <Dot state={run.error ? 'error' : 'done'} />
              <span style={{ flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                {run.agentId} · {fmtClock(run.startMs)}
              </span>
              <span style={{ fontSize: 11, color: 'var(--dsw-alias-label-secondary)', flexShrink: 0 }}>
                {fmtDuration(run.endMs - run.startMs)}
                {run.usage?.total_tokens ? ` · ${run.usage.total_tokens} tok` : ''}
                {run.error ? ' · 出错' : ''}
              </span>
              <svg width="11" height="11" viewBox="0 0 12 12" fill="none" style={{ transform: isOpen ? 'rotate(90deg)' : 'none', transition: 'transform .15s', flexShrink: 0 }}>
                <path d="M4 2.5L8.5 6 4 9.5" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round"/>
              </svg>
            </button>
            {isOpen && (
              <div style={{ margin: '4px 0 0 20px', borderLeft: '2px solid color-mix(in srgb, currentColor 15%, transparent)', paddingLeft: 10 }}>
                {run.spans.length === 0 && <div style={{ color: 'var(--dsw-alias-label-secondary)', fontSize: 11.5 }}>该 run 无 span 明细</div>}
                {run.spans.map((sp, i) => (
                  <div key={i} style={{ display: 'flex', gap: 8, padding: '3px 0', fontSize: 11.5 }}>
                    <span style={{ color: 'var(--dsw-alias-label-secondary)', width: 30, flexShrink: 0 }}>{sp.kind}</span>
                    <span style={{ flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', color: sp.error ? '#d4544a' : undefined }}>
                      {sp.name}
                    </span>
                    <span style={{ color: 'var(--dsw-alias-label-secondary)', flexShrink: 0 }}>{fmtDuration(sp.duration_ms ?? 0)}</span>
                  </div>
                ))}
                {run.error && <div style={{ fontSize: 11.5, color: '#d4544a', padding: '3px 0', wordBreak: 'break-all' }}>{run.error}</div>}
              </div>
            )}
          </div>
        )
      })}
    </div>
  )
}
