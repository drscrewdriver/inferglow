/**
 * ConvPanels — content for the 轨迹 (trace) and 上下文 (context) conversation
 * tabs. Data-driven since the six-panel integration: the timeline renders real
 * spans from /v1/observability/*, turns render real session messages, and any
 * metric without a backend data source renders "—" instead of fake numbers.
 *
 * Trace data sources, in order: the session's PERSISTED run summaries
 * (GET /v1/sessions/{id}/trace — survives restarts and session restores)
 * merged with the live in-memory ring (covers the still-running run, whose
 * summary is only written at completion). Context composition metrics that
 * still have no endpoint render "—".
 */

import { useEffect, useState } from 'react'
import { api } from '../../bridge/inferglow.ts'
import { getUsageTotals } from '../../bridge/inferglow.ts'
import type { ChatMessage } from '../../api/types.ts'
import type { SpanSummary } from '../../api/client.ts'

const LANE_BY_KIND: Record<string, number> = { agent: 0, llm: 1, tool: 2 }

interface TraceData {
  spans: SpanSummary[] | null // null = collector disabled (503)
  spansErr: string | null
  messages: ChatMessage[]
}

/** Parse one persisted run summary into SpanSummary lines (end = start+ms). */
function tracesToSpans(traces: { content: string; created_at: string }[]): SpanSummary[] {
  const out: SpanSummary[] = []
  for (const t of traces) {
    let parsed: { start?: string; spans?: { kind: string; name: string; duration_ms: number; error?: boolean }[] } = {}
    try { parsed = JSON.parse(t.content) } catch { continue }
    const startMs = parsed.start ? Date.parse(parsed.start) : Date.parse(t.created_at)
    for (const sp of parsed.spans ?? []) {
      const end = (Number.isNaN(startMs) ? Date.parse(t.created_at) : startMs) + (sp.duration_ms ?? 0)
      out.push({
        name: sp.name,
        kind: (sp.kind as SpanSummary['kind']) ?? 'internal',
        duration_ns: (sp.duration_ms ?? 0) * 1e6,
        end_time: new Date(end).toISOString(),
        has_error: !!sp.error,
        attrs: {},
      })
    }
  }
  return out
}

function useTraceData(session: string | null, reloadKey: number): TraceData {
  const [data, setData] = useState<TraceData>({ spans: null, spansErr: null, messages: [] })
  useEffect(() => {
    let alive = true
    setData({ spans: null, spansErr: null, messages: [] })
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
    const tracesP = session
      ? api.sessionTrace(session).catch(() => [])
      : Promise.resolve([])
    void Promise.all([spansP, msgsP, tracesP]).then(([liveSpans, messages, traces]) => {
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
      setData(d => ({ ...d, spans, messages }))
    })
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

export function TracePanel({ session }: { session: string | null }) {
  const [reloadKey, setReloadKey] = useState(0)
  const { spans, spansErr, messages } = useTraceData(session, reloadKey)

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

  const turns = groupTurns(messages)

  return (
    <div className="dsh-trace">
      <div className="dsh-trace-inner">
        <div className="dsh-trace-actions">
          <button type="button" className="dsh-trace-toggle" aria-pressed="false">
            <svg className="dsh-trace-toggle-icon" viewBox="0 0 16 16" fill="none"><circle cx="8" cy="8" r="5.25"/><path d="M8 4.75V8l2.25 1.5"/></svg>
            Duration
          </button>
          <button type="button" className="dsh-trace-action" aria-pressed="false"><span>⊟</span>Turns</button>
          <button type="button" className="dsh-trace-action" aria-pressed="false"><span>⊟</span>Calls</button>
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

      {/* Turn records — real session messages */}
      <div className="dsh-trace-turns">
        {turns.map((turn, i) => (
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
        ))}
        {turns.length === 0 && <div className="dsh-pane-git-empty">暂无消息记录</div>}
      </div>
    </div>
  )
}

/* ── 上下文 (context): real aggregates only; no-source metrics show "—" ── */

function fmtNum(n: number): string {
  return n >= 1000 ? `${(n / 1000).toFixed(1)}k` : String(n)
}

export function ContextPanel({ session }: { session: string | null }) {
  const [reloadKey, setReloadKey] = useState(0)
  const { spans, spansErr, messages } = useTraceData(session, reloadKey)

  const userCount = messages.filter(m => m.role === 'user').length
  const assistantCount = messages.filter(m => m.role === 'assistant').length
  const toolCount = messages.filter(m => m.role === 'tool').length
  const usage = getUsageTotals()

  const stats: [string, string][] = [
    ['轮次', String(userCount)],
    ['助手回复', String(assistantCount)],
    ['工具调用', String(toolCount)],
    ['注入', '—'],
    ['压缩', '—'],
    ['剪枝', '—'],
    ['缓存命中', '—'],
    ['LLM 调用', spans ? String(spans.filter(s => s.kind === 'llm').length) : '—'],
  ]

  const events = [...messages]
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
            <span className="lc-card-sub">来自消息记录与观测 span 的真实聚合</span>
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
              <b className="lc-stat-value">—</b>
            </div>
          </div>
        </div>
        <div className="lc-card">
          <div className="lc-card-title">
            <span className="lc-card-title-text">Token 用量</span>
            <span className="lc-card-sub">供应商上报（llm_end usage）累计 · 本次页面会话</span>
          </div>
          <div className="lc-overview-num">
            <b>{fmtNum(usage.totalTokens)}</b><span> / total tokens</span>
          </div>
          <div className="lc-stats">
            <div className="lc-stat"><span className="lc-stat-label">输入</span><b className="lc-stat-value">{fmtNum(usage.promptTokens)}</b></div>
            <div className="lc-stat"><span className="lc-stat-label">输出</span><b className="lc-stat-value">{fmtNum(usage.completionTokens)}</b></div>
            <div className="lc-stat"><span className="lc-stat-label">LLM 调用</span><b className="lc-stat-value">{usage.llmCalls}</b></div>
          </div>
          {spansErr !== null && (
            <div className="lc-empty">观测未启用（spans: {spansErr.slice(0, 40)}）— 上下文构成/压缩指标暂无数据源</div>
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
            {events.map((ev, i) => (
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
            <span className="lc-card-sub">暂无对应数据端点</span>
          </div>
          <div className="lc-empty">后端尚未记录会话级文件读写/搜索事件（二期）</div>
        </div>
      </div>

      <div className="lc-foot">口径：spans 为 server 内存环（重启即失）；usage 为供应商上报累计，仅统计本页面发起的调用；构成/压缩/费用暂无数据源，显示 —。</div>
    </div>
  )
}
