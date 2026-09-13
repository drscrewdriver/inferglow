/**
 * ConvPanels — content for the 轨迹 (trace) and 上下文 (context) conversation
 * tabs. Data-driven since the six-panel integration: the timeline renders real
 * spans from /v1/observability/*, turns render server turn summaries, and any
 * metric without a backend data source renders "—" instead of fake numbers.
 *
 * Trace data sources: the session's PERSISTED run summaries
 * (GET /v1/sessions/{id}/trace — structured `summary` included when the
 * content parses), the per-request usage records (GET /sessions/{id}/requests)
 * and the rollout event stream (GET /sessions/{id}/events), merged with the
 * live in-memory ring (covers the still-running run). Every data source
 * degrades independently: a missing source shows "—", never fake numbers.
 */

import { useEffect, useState } from 'react'
import { api, fetchPersistedUsage } from '../../bridge/inferglow.ts'
import { getUsageTotals } from '../../bridge/inferglow.ts'
import type { ChatMessage } from '../../api/types.ts'
import type {
  ContextBreakdown,
  ContextEventPayload,
  RequestRecord,
  SessionEvent,
  SessionTrace,
  SpanSummary,
} from '../../api/client.ts'
import { tracesToSpans } from '../../lib/traceUtils.ts'

const LANE_BY_KIND: Record<string, number> = { agent: 0, llm: 1, tool: 2 }

interface TraceData {
  spans: SpanSummary[] | null // null = collector disabled (503)
  spansErr: string | null
  messages: ChatMessage[]
  traces: SessionTrace[]
  requests: RequestRecord[]
  events: SessionEvent[]
}

/** Decode an llm_request payload's six-category breakdown (tolerant). */
function eventBreakdown(ev: SessionEvent): ContextBreakdown | null {
  const p = ev.payload as { breakdown?: ContextBreakdown } | undefined
  const b = p?.breakdown
  if (!b || typeof b.total !== 'number' || b.total <= 0) return null
  return b
}

function useTraceData(session: string | null, reloadKey: number): TraceData {
  const [data, setData] = useState<TraceData>({
    spans: null, spansErr: null, messages: [], traces: [], requests: [], events: [],
  })
  useEffect(() => {
    let alive = true
    setData({ spans: null, spansErr: null, messages: [], traces: [], requests: [], events: [] })
    const spansP = api.spans(500)
      .then(spans => spans)
      .catch(e => {
        if (alive) setData(d => ({ ...d, spansErr: String((e as Error)?.message ?? e) }))
        return null
      })
    const msgsP = session
      ? api.listMessages(session, 200).catch(() => [] as ChatMessage[])
      : Promise.resolve([] as ChatMessage[])
    // Persisted run summaries: the durable source after restarts/restores.
    const tracesP: Promise<SessionTrace[]> = session
      ? api.sessionTrace(session).catch(() => [])
      : Promise.resolve([])
    const requestsP: Promise<RequestRecord[]> = session
      ? api.sessionRequests(session, { limit: 500 }).catch(() => [])
      : Promise.resolve([])
    const eventsP: Promise<SessionEvent[]> = session
      ? api.sessionEvents(session, {
          kinds: ['llm_request', 'context_event', 'file_op', 'turn_start', 'turn_end'],
          limit: 1000,
        }).catch(() => [])
      : Promise.resolve([])
    void Promise.all([spansP, msgsP, tracesP, requestsP, eventsP]).then(
      ([liveSpans, messages, traces, requests, events]) => {
        if (!alive) return
        let spans: SpanSummary[] | null = liveSpans
        if (session && traces.length > 0) {
          const persisted = tracesToSpans(traces).map(t => ({
            ...t, attrs: { 'inferglow.session_id': session },
          }))
          const seen = new Set((spans ?? []).map(x => `${x.name}@${x.end_time}`))
          const merged = [...persisted.filter(x => !seen.has(`${x.name}@${x.end_time}`)), ...(spans ?? [])]
          spans = merged.sort((a, b) => Date.parse(a.end_time) - Date.parse(b.end_time))
        }
        setData(d => ({ ...d, spans, messages, traces, requests, events }))
      },
    )
    return () => { alive = false }
  }, [session, reloadKey])
  return data
}

/** Group a message list into turns: each user message opens a new turn. */
function groupTurns(messages: ChatMessage[]): ChatMessage[][] {
  const turns: ChatMessage[][] = []
  for (const m of messages) {
    if (m.role === 'user' || turns.length === 0) turns.push([m])
    else turns[turns.length - 1].push(m)
  }
  return turns
}

function fmtClock(iso: string): string {
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '' : d.toLocaleTimeString('zh-CN', { hour12: false })
}

function fmtMs(ms?: number): string {
  if (ms === undefined || ms === null) return '—'
  return ms >= 1000 ? `${(ms / 1000).toFixed(1)}s` : `${ms}ms`
}

/** How many recent entries stay expanded before folding. */
const FOLD_COUNT = 5

export function TracePanel({ session }: { session: string | null }) {
  const [reloadKey, setReloadKey] = useState(0)
  const { spans, spansErr, messages, traces, requests, events } = useTraceData(session, reloadKey)
  const [showTurns, setShowTurns] = useState(true)
  const [showCalls, setShowCalls] = useState(true)
  const [expandAllTurns, setExpandAllTurns] = useState(false)
  const [selected, setSelected] = useState<RequestRecord | null>(null)

  const scoped = spans?.filter(s => !session || s.attrs?.['inferglow.session_id'] === session) ?? null
  const collectorDisabled = spansErr !== null

  // Timeline placement: position spans by end_time inside the observed window.
  let timeline: { left: number; width: number; lane: number; kind: string; name: string; hasError: boolean }[] = []
  if (scoped && scoped.length > 0) {
    const times = scoped.map(s => new Date(s.end_time).getTime()).filter(t => !Number.isNaN(t))
    const min = Math.min(...times)
    const max = Math.max(...times, min + 1)
    const windowMs = Math.max(1, max - min)
    timeline = scoped
      .filter(s => !Number.isNaN(new Date(s.end_time).getTime()))
      .map(s => {
        const end = new Date(s.end_time).getTime()
        const durMs = Math.max(0, s.duration_ns / 1e6)
        return {
          left: ((end - min) / windowMs) * 96,
          width: Math.max(1.2, (durMs / windowMs) * 96),
          lane: LANE_BY_KIND[s.kind] ?? 0,
          kind: s.kind,
          name: s.name,
          hasError: s.has_error,
        }
      })
  }

  // Server turn summaries (P3): one card per persisted run. Fall back to the
  // message-grouping heuristic only when no structured summaries exist.
  const serverTurns = traces.filter(t => (t.summary?.turns?.length ?? 0) > 0)
  const hasServerTurns = serverTurns.length > 0
  const visibleTurns = expandAllTurns ? serverTurns : serverTurns.slice(-FOLD_COUNT)
  const fallbackTurns = groupTurns(messages)

  // Breakdown lookup for the request detail modal (llm_request events by round).
  const breakdownByRound = new Map<number, ContextBreakdown>()
  for (const ev of events) {
    if (ev.type !== 'llm_request') continue
    const b = eventBreakdown(ev)
    if (b && !breakdownByRound.has(ev.round ?? 0)) breakdownByRound.set(ev.round ?? 0, b)
  }
  const selectedBreakdown = selected !== null ? breakdownByRound.get(selected.round) ?? null : null

  return (
    <div className="dsh-trace">
      <div className="dsh-trace-inner">
        <div className="dsh-trace-actions">
          <button type="button" className="dsh-trace-toggle" aria-pressed="false">
            <svg className="dsh-trace-toggle-icon" viewBox="0 0 16 16" fill="none"><circle cx="8" cy="8" r="5.25"/><path d="M8 4.75V8l2.25 1.5"/></svg>
            Duration
          </button>
          <button
            type="button"
            className="dsh-trace-action"
            aria-pressed={showTurns}
            onClick={() => setShowTurns(v => !v)}
          ><span>⊟</span>Turns</button>
          <button
            type="button"
            className="dsh-trace-action"
            aria-pressed={showCalls}
            onClick={() => setShowCalls(v => !v)}
          ><span>⊟</span>Calls</button>
          <button type="button" className="dsh-trace-action" aria-pressed="false" onClick={() => setReloadKey(k => k + 1)}>
            <span>↻</span>刷新
          </button>
        </div>
        <div className="dsh-trace-search">
          <svg width="11" height="11" viewBox="0 0 16 16" fill="none"><path d="M11.89 6.65A5.3 5.3 0 1 1 1.35 6.65a5.3 5.3 0 0 1 10.54 0"/><path d="M16 15.04l-4.47-4.53"/></svg>
          <input type="search" aria-label="搜索轨迹" placeholder="搜索" value="" readOnly />
        </div>
      </div>

      <section className="dsh-trace-timeline" aria-label="Trajectory timeline">
        {collectorDisabled ? (
          <div className="dsh-pane-git-empty">观测未启用 — server 未接 SpanCollector，无轨迹时间线</div>
        ) : timeline.length === 0 ? (
          <div className="dsh-pane-git-empty">本会话暂无 span — 对话产生 agent/llm/tool 事件后出现</div>
        ) : (
          <div className="dsh-trace-plot">
            <div className="dsh-trace-lanes-labels" aria-hidden="true">
              <span>Agent</span><span>Model</span><span>Tools</span>
            </div>
            <div className="dsh-trace-track">
              <div className="dsh-trace-lanes">
                {timeline.map((s, i) => (
                  <span
                    key={i}
                    title={`${s.name} (${s.kind})`}
                    className={`dsh-trace-span span-${s.kind}${s.hasError ? ' span-error' : ''}`}
                    style={{ left: `${s.left}%`, width: `${s.width}%`, top: `${s.lane * 33.33}%` }}
                  />
                ))}
              </div>
            </div>
          </div>
        )}
      </section>

      {/* Calls — per-request usage records from /requests (P3) */}
      {showCalls && (
        <div className="dsh-trace-calls">
          <div className="dsh-trace-calls-head">
            <span className="dsh-trace-calls-title">每次请求（Calls）</span>
            <span className="dsh-trace-calls-sub">{requests.length > 0 ? `${requests.length} 次调用 · 点击查看详情` : ''}</span>
          </div>
          {requests.length === 0 ? (
            <div className="dsh-pane-git-empty">暂无每请求记录 — 对话产生 LLM 调用后出现</div>
          ) : (
            <div className="dsh-trace-calls-list">
              {requests.slice(-50).map((req, i) => (
                <button
                  key={`${req.round}-${i}`}
                  type="button"
                  className="dsh-call-row"
                  onClick={() => setSelected(req)}
                >
                  <span className="dsh-call-round">#{req.round}</span>
                  <span className="dsh-call-model">{req.model || '—'}</span>
                  <span className="dsh-call-tokens">{req.tokens.toLocaleString()} tok</span>
                  {req.cached_tokens ? <span className="dsh-call-cached">缓存 {req.cached_tokens.toLocaleString()}</span> : null}
                  <span className="dsh-call-dur">{fmtMs(req.duration_ms)}</span>
                  <span className="dsh-call-time">{fmtClock(req.timestamp)}</span>
                </button>
              ))}
            </div>
          )}
        </div>
      )}

      {/* Turns — server turn summaries when available (P3), else message grouping */}
      {showTurns && (
        <div className="dsh-trace-turns">
          {hasServerTurns ? (
            <>
              {visibleTurns.map((t, i) => {
                const turns = t.summary?.turns ?? []
                const rounds = turns.reduce((a, x) => a + (x.rounds ?? 0), 0)
                const tokens = turns.reduce((a, x) => a + (x.total_tokens ?? 0), 0)
                const wall = turns.reduce((a, x) => a + (x.wall_ms ?? 0), 0)
                const hasErr = turns.some(x => x.error) || !!t.summary?.error
                return (
                  <div key={t.id} className="dsh-turn">
                    <div className="dsh-turn-head">
                      <span className="dsh-turn-num">运行 {i + 1} · {rounds} 轮</span>
                      <span className="dsh-turn-time">{fmtClock(t.created_at)}</span>
                      <span className="dsh-turn-dur">
                        {tokens > 0 ? `${tokens.toLocaleString()} tok` : ''} {wall > 0 ? `· ${fmtMs(wall)}` : ''}
                      </span>
                      <span className={`dsh-turn-status ${hasErr ? 'error' : 'done'}`}>{hasErr ? '✗' : '✓'}</span>
                    </div>
                  </div>
                )
              })}
              {serverTurns.length > FOLD_COUNT && !expandAllTurns && (
                <button type="button" className="dsh-pane-linkbtn" onClick={() => setExpandAllTurns(true)}>
                  展开更早的 {serverTurns.length - FOLD_COUNT} 次运行
                </button>
              )}
            </>
          ) : (
            fallbackTurns.map((turn, i) => (
              <div key={i} className="dsh-turn">
                <div className="dsh-turn-head">
                  <span className="dsh-turn-num">第 {i + 1} 轮 · {turn.length} 条</span>
                  <span className="dsh-turn-time">{fmtClock(turn[0]?.created_at ?? '')}</span>
                  <span className="dsh-turn-dur">{turn.some(m => m.tool_status === 'error') ? '含错误' : ''}</span>
                  <span className={`dsh-turn-status ${turn.some(m => m.tool_status === 'error') ? 'error' : 'done'}`}>✓</span>
                </div>
                <div className="dsh-turn-msgs">
                  {turn.map((m, j) => (
                    <div key={j} className={`dsh-turn-msg msg-${m.role}`}>
                      <span className="dsh-turn-msg-role">
                        {m.role === 'user' ? '用户' : m.role === 'tool' ? `工具 · ${m.tool_name ?? ''}` : '助手'}
                      </span>
                      <span className="dsh-turn-msg-text">
                        {(m.content || '').slice(0, 200)}
                      </span>
                    </div>
                  ))}
                </div>
              </div>
            ))
          )}
          {fallbackTurns.length === 0 && !hasServerTurns && <div className="dsh-pane-git-empty">暂无消息记录</div>}
        </div>
      )}

      {/* Request detail modal */}
      {selected && (
        <div className="dsh-trace-modal" role="dialog" aria-label="请求详情" onClick={() => setSelected(null)}>
          <div className="dsh-trace-modal-card" onClick={e => e.stopPropagation()}>
            <div className="dsh-trace-modal-head">
              <span className="dsh-trace-modal-title">请求 #{selected.round}</span>
              <button type="button" className="dsh-pane-linkbtn" onClick={() => setSelected(null)}>✕ 关闭</button>
            </div>
            <div className="dsh-trace-modal-grid">
              <div><span className="lc-stat-label">模型</span><b>{selected.model || '—'}</b></div>
              <div><span className="lc-stat-label">总 tokens</span><b>{selected.tokens.toLocaleString()}</b></div>
              <div><span className="lc-stat-label">输入</span><b>{(selected.usage?.prompt_tokens ?? 0).toLocaleString()}</b></div>
              <div><span className="lc-stat-label">输出</span><b>{(selected.usage?.completion_tokens ?? 0).toLocaleString()}</b></div>
              <div><span className="lc-stat-label">缓存命中</span><b>{selected.cached_tokens ? selected.cached_tokens.toLocaleString() : '—'}</b></div>
              <div><span className="lc-stat-label">耗时</span><b>{fmtMs(selected.duration_ms)}</b></div>
              <div><span className="lc-stat-label">时间</span><b>{fmtClock(selected.timestamp)}</b></div>
              <div><span className="lc-stat-label">轮次</span><b>{selected.turn ?? '—'}</b></div>
            </div>
            {selectedBreakdown ? (
              <div className="dsh-trace-breakdown">
                <div className="lc-card-title-text">上下文构成（六分类估算）</div>
                <div className="dsh-breakdown-bar" role="img"
                  aria-label={`上下文构成 总计 ${selectedBreakdown.total} tokens`}>
                  {([
                    ['system', '系统'], ['tool_schemas', '工具'], ['user', '用户'],
                    ['inject', '注入'], ['assistant', '助手'], ['tool_results', '工具结果'],
                  ] as [keyof ContextBreakdown, string][]).map(([key, label]) => {
                    const v = selectedBreakdown[key] ?? 0
                    if (v <= 0) return null
                    return (
                      <span key={key}
                        className={`dsh-breakdown-seg seg-${key.replace('_', '-')}`}
                        style={{ width: `${(v / Math.max(1, selectedBreakdown.total)) * 100}%` }}
                        title={`${label}: ${v.toLocaleString()} tok`} />
                    )
                  })}
                </div>
                <div className="dsh-breakdown-legend">
                  {([
                    ['system', '系统'], ['tool_schemas', '工具'], ['user', '用户'],
                    ['inject', '注入'], ['assistant', '助手'], ['tool_results', '工具结果'],
                  ] as [keyof ContextBreakdown, string][]).map(([key, label]) => (
                    <span key={key} className="dsh-breakdown-legend-item">
                      <i className={`dsh-breakdown-seg seg-${key.replace('_', '-')}`} aria-hidden="true" />
                      {label} {(selectedBreakdown[key] ?? 0).toLocaleString()}
                    </span>
                  ))}
                </div>
              </div>
            ) : (
              <div className="lc-empty">该请求无六分类构成数据（需引擎 rollout 事件流）</div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

/* ── 上下文 (context): real aggregates only; no-source metrics show "—" ── */

function fmtNum(n: number): string {
  return n >= 1000 ? `${(n / 1000).toFixed(1)}k` : String(n)
}

function fmtCost(cost: number, currency: string): string {
  if (cost <= 0) return '—'
  return `$${cost.toFixed(4)}${currency && currency !== 'USD' ? ` ${currency}` : ''}`
}

interface ContextData {
  usageReport: { total_cost: number; currency: string; record_count: number } | null
  persisted: { promptTokens: number; completionTokens: number; totalTokens: number; cachedTokens: number; llmCalls: number } | null
}

export function ContextPanel({ session }: { session: string | null }) {
  const [reloadKey, setReloadKey] = useState(0)
  const { spans, spansErr, messages, requests, events } = useTraceData(session, reloadKey)
  const [ctx, setCtx] = useState<ContextData>({ usageReport: null, persisted: null })
  const [trendMode, setTrendMode] = useState<'total' | 'delta'>('total')

  useEffect(() => {
    let alive = true
    if (!session) {
      setCtx({ usageReport: null, persisted: null })
      return
    }
    void Promise.all([
      api.sessionUsageReport(session).catch(() => null),
      fetchPersistedUsage(session),
    ]).then(([usageReport, persisted]) => {
      if (alive) setCtx({ usageReport, persisted })
    })
    return () => { alive = false }
  }, [session, reloadKey])

  const userCount = messages.filter(m => m.role === 'user').length
  const usage = getUsageTotals()

  // Context events by kind (P2 rollout stream via /events).
  const contextEvents = events.filter(e => e.type === 'context_event')
  const compactionCount = contextEvents.filter(e => (e.payload as ContextEventPayload | undefined)?.kind === 'compaction').length
  const injectCount = contextEvents.filter(e => (e.payload as ContextEventPayload | undefined)?.kind === 'inject').length
  const pruneCount = contextEvents.filter(e => (e.payload as ContextEventPayload | undefined)?.kind === 'prune').length

  // Cache hit from per-request records.
  const cached = ctx.persisted?.cachedTokens ?? requests.reduce((a, r) => a + (r.cached_tokens ?? 0), 0)
  const promptTotal = ctx.persisted?.promptTokens ?? requests.reduce((a, r) => a + (r.usage?.prompt_tokens ?? 0), 0)
  const cacheHit = promptTotal > 0 && cached > 0 ? `${Math.round((cached / promptTotal) * 100)}%` : '—'

  // File activity from file_op events.
  const fileOps = events.filter(e => e.type === 'file_op')

  // Latest six-category breakdown (llm_request events).
  let latestBreakdown: ContextBreakdown | null = null
  for (const ev of events) {
    if (ev.type !== 'llm_request') continue
    const b = eventBreakdown(ev)
    if (b) latestBreakdown = b
  }

  // Context Trend bars (per-request records): total tokens, or delta of
  // prompt tokens between consecutive requests (context growth).
  const trendData = requests.slice(-40)
  const trendBars = trendData.map((r, i) => {
    if (trendMode === 'total') {
      return { value: r.tokens, peak: 0, label: `#${r.round} · ${r.tokens} tok` }
    }
    const prev = i > 0 ? trendData[i - 1].usage?.prompt_tokens ?? 0 : 0
    const delta = (r.usage?.prompt_tokens ?? 0) - prev
    return { value: delta, peak: Math.min(0, delta), label: `#${r.round} · Δ${delta >= 0 ? '+' : ''}${delta} prompt` }
  })
  const trendMax = Math.max(1, ...trendBars.map(b => Math.abs(b.value)))

  const stats: [string, string][] = [
    ['轮次', String(userCount)],
    ['注入', injectCount > 0 ? String(injectCount) : '—'],
    ['压缩', compactionCount > 0 ? String(compactionCount) : '—'],
    ['剪枝', pruneCount > 0 ? String(pruneCount) : '—'],
    ['缓存命中', cacheHit],
    ['LLM 调用', requests.length > 0
      ? String(requests.length)
      : spans ? String(spans.filter(s => s.kind === 'llm').length) : '—'],
  ]

  const msgEvents = [...messages]
    .reverse()
    .slice(0, 12)
    .map(m => ({
      kind: m.role === 'tool' ? '工具' : m.role === 'assistant' ? '回复' : '输入',
      label: (m.content || m.tool_name || '').slice(0, 60),
      time: fmtClock(m.created_at),
    }))

  return (
    <div className="lc-root">
      {/* Head row: real stats + provider usage */}
      <div className="lc-cols lc-head">
        <div className="lc-card lc-col lc-col-stats">
          <div className="lc-card-title">
            <span className="lc-card-title-text">会话统计</span>
            <span className="lc-card-sub">消息记录 / 每请求记录 / rollout 事件的真实聚合</span>
          </div>
          <div className="lc-stats">
            {stats.map(([label, value]) => (
              <div key={label} className="lc-stat">
                <span className="lc-stat-label">{label}</span>
                <b className="lc-stat-value">{value}</b>
              </div>
            ))}
            <div className="lc-stat lc-stat-tipped">
              <span className="lc-stat-label">预估费用<i className="lc-stat-q" aria-hidden="true">?</i></span>
              <b className="lc-stat-value">
                {ctx.usageReport ? fmtCost(ctx.usageReport.total_cost, ctx.usageReport.currency) : '—'}
              </b>
            </div>
          </div>
        </div>
        <div className="lc-card">
          <div className="lc-card-title">
            <span className="lc-card-title-text">Token 用量</span>
            <span className="lc-card-sub">持久化每请求记录（跨重启）· 本次页面增量为实时补充</span>
          </div>
          <div className="lc-overview-num">
            <b>{fmtNum(ctx.persisted?.totalTokens ?? 0)}</b><span> / total tokens（持久化）</span>
          </div>
          <div className="lc-stats">
            <div className="lc-stat"><span className="lc-stat-label">输入</span><b className="lc-stat-value">{fmtNum(ctx.persisted?.promptTokens ?? 0)}</b></div>
            <div className="lc-stat"><span className="lc-stat-label">输出</span><b className="lc-stat-value">{fmtNum(ctx.persisted?.completionTokens ?? 0)}</b></div>
            <div className="lc-stat"><span className="lc-stat-label">调用</span><b className="lc-stat-value">{ctx.persisted?.llmCalls ?? 0}</b></div>
          </div>
          {usage.totalTokens > 0 && (
            <div className="lc-empty">本次页面增量：{fmtNum(usage.totalTokens)} tok（{usage.llmCalls} 次调用）</div>
          )}
          {spansErr !== null && (
            <div className="lc-empty">观测未启用（spans: {spansErr.slice(0, 40)}）— 实时时间线暂无数据源</div>
          )}
        </div>
      </div>

      {/* Context Trend + composition */}
      <div className="lc-cols">
        <div className="lc-card lc-col">
          <div className="lc-card-title">
            <span className="lc-card-title-text">Context Trend</span>
            <span className="dsh-trace-calls-sub">
              <button type="button" className={`dsh-trace-action${trendMode === 'total' ? ' active' : ''}`} aria-pressed={trendMode === 'total'} onClick={() => setTrendMode('total')}>总量</button>
              <button type="button" className={`dsh-trace-action${trendMode === 'delta' ? ' active' : ''}`} aria-pressed={trendMode === 'delta'} onClick={() => setTrendMode('delta')}>增量</button>
            </span>
          </div>
          {trendBars.length === 0 ? (
            <div className="lc-empty">暂无每请求数据 — 对话产生 LLM 调用后出现</div>
          ) : (
            <div className="lc-trend" role="img" aria-label="每请求 token 趋势">
              {trendBars.map((bar, i) => (
                <span
                  key={i}
                  className={`lc-trend-bar${bar.value < 0 ? ' lc-trend-bar-neg' : ''}`}
                  style={{ height: `${Math.max(4, (Math.abs(bar.value) / trendMax) * 100)}%` }}
                  title={bar.label}
                />
              ))}
            </div>
          )}
        </div>
        <div className="lc-card lc-col">
          <div className="lc-card-title">
            <span className="lc-card-title-text">上下文构成</span>
            <span className="lc-card-sub">最近一次请求的六分类估算</span>
          </div>
          {latestBreakdown ? (
            <div className="dsh-trace-breakdown">
              <div className="dsh-breakdown-bar" role="img"
                aria-label={`上下文构成 总计 ${latestBreakdown.total} tokens`}>
                {([
                  ['system', '系统'], ['tool_schemas', '工具'], ['user', '用户'],
                  ['inject', '注入'], ['assistant', '助手'], ['tool_results', '工具结果'],
                ] as [keyof ContextBreakdown, string][]).map(([key]) => {
                  const v = latestBreakdown![key] ?? 0
                  if (v <= 0) return null
                  return (
                    <span key={key}
                      className={`dsh-breakdown-seg seg-${key.replace('_', '-')}`}
                      style={{ width: `${(v / Math.max(1, latestBreakdown!.total)) * 100}%` }} />
                  )
                })}
              </div>
              <div className="dsh-breakdown-legend">
                {([
                  ['system', '系统'], ['tool_schemas', '工具'], ['user', '用户'],
                  ['inject', '注入'], ['assistant', '助手'], ['tool_results', '工具结果'],
                ] as [keyof ContextBreakdown, string][]).map(([key, label]) => (
                  <span key={key} className="dsh-breakdown-legend-item">
                    <i className={`dsh-breakdown-seg seg-${key.replace('_', '-')}`} aria-hidden="true" />
                    {label} {(latestBreakdown![key] ?? 0).toLocaleString()}
                  </span>
                ))}
              </div>
            </div>
          ) : (
            <div className="lc-empty">暂无构成数据 — 需引擎 rollout 事件流（llm_request 事件）</div>
          )}
        </div>
      </div>

      {/* Bottom row: message-flow events + file activity */}
      <div className="lc-cols">
        <div className="lc-card lc-col">
          <div className="lc-card-title">
            <span className="lc-card-title-text">消息事件</span>
            <button type="button" className="dsh-pane-linkbtn" onClick={() => setReloadKey(k => k + 1)}>↻ 刷新</button>
          </div>
          <div className="lc-events">
            {msgEvents.map((ev, i) => (
              <div key={i} className="lc-event">
                <span className={`lc-kind ${ev.kind === '工具' ? 'lc-kind-inject' : 'lc-kind-reply'}`}>{ev.kind}</span>
                <span className="lc-event-label">{ev.label || '(空)'}</span>
                <span className="lc-event-time">{ev.time}</span>
              </div>
            ))}
            {events.length === 0 && <div className="lc-empty">该会话暂无消息</div>}
          </div>
        </div>
        <div className="lc-card lc-col">
          <div className="lc-card-title">
            <span className="lc-card-title-text">文件活动</span>
            <span className="lc-card-sub">来自 rollout file_op 事件</span>
          </div>
          {fileOps.length === 0 ? (
            <div className="lc-empty">该会话暂无文件读写/搜索事件</div>
          ) : (
            <div className="lc-events">
              {fileOps.slice(-12).map((ev, i) => {
                const p = ev.payload as { op?: string; path?: string } | undefined
                return (
                  <div key={i} className="lc-event">
                    <span className={`lc-kind ${p?.op === 'write' ? 'lc-kind-inject' : 'lc-kind-reply'}`}>
                      {p?.op === 'write' ? '写入' : p?.op === 'search' ? '搜索' : '读取'}
                    </span>
                    <span className="lc-event-label">{p?.path || ev.tool_name || '(未记录路径)'}</span>
                    <span className="lc-event-time">{fmtClock(ev.timestamp)}</span>
                  </div>
                )
              })}
            </div>
          )}
        </div>
      </div>

      <div className="lc-foot">口径：spans 为 server 内存环（重启即失）；tokens/费用为持久化每请求记录（跨重启可读）；构成为引擎估算值；压缩/注入计数来自 rollout 事件流。</div>
    </div>
  )
}
