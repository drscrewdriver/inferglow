// TrendChart — per-request stacked columns with Step/Turn granularity,
// Total/Delta mode, event markers (✂ compaction/prune), hover + click select,
// and a paired RequestDetail row under the active step.

import { useMemo } from 'react'
import { CATEGORY_COLORS, CATEGORY_ORDER, EVENT_ICONS, fmtTokens } from './palette'
import { fmtNum } from './token'
import type { ProjectionEvent, ProjectionRequest } from './types'
import styles from './context.module.css'

export interface TrendChartProps {
  requests: ProjectionRequest[]
  events: ProjectionEvent[]
  mode: 'total' | 'delta'
  onSelect: (r: ProjectionRequest | null) => void
}

export function TrendChart({ requests, events, mode, onSelect }: TrendChartProps) {

  // Map request seq → the nearest event marker stamped at/after it.
  const markerFor = useMemo(() => {
    const m = new Map<number, ProjectionEvent>()
    for (const ev of events) {
      if (ev.kind === 'compaction' || ev.kind === 'prune') m.set(ev.seq, ev)
    }
    return m
  }, [events])

  return (
    <div className={styles.chartWrap}>
      <div className={styles.chart}>
        {requests.length === 0 ? (
          <div className={styles.chartEmptyAxis}>暂无步骤数据</div>
        ) : (
          requests.map((r, i) => {
            const marker = markerFor.get(r.seq)
            const max = mode === 'total' ? r.tokens.total : Math.max(1, Math.abs(r.net ?? 0))
            return (
              <div key={`${r.turn}-${r.step}`} style={{ position: 'relative' }}>
                {marker && (
                  <span className={styles.chartMarker} title={marker.name ?? marker.kind}>
                    {EVENT_ICONS[marker.kind] ?? '✂'}
                  </span>
                )}
                <div
                  className={styles.chartCol}
                  onClick={() => onSelect(r)}
                >
                  <div className={styles.chartBar}>
                    <div className={styles.chartParts}>
                      {CATEGORY_ORDER.map((key) => {
                        const v = r.tokens[key]
                        if (!v || v <= 0) return null
                        const pct = (v / max) * 100
                        return (
                          <div
                            key={key}
                            className={styles.chartPartsRow}
                            style={{ height: `${pct}%`, minHeight: v > 0 ? 2 : 0, background: CATEGORY_COLORS[key], width: '100%' }}
                            title={`${key} ${fmtTokens(v)}`}
                          />
                        )
                      })}
                      {mode === 'delta' && (r.net ?? 0) < 0 && (
                        <div style={{ height: `${Math.min(100, ((-(r.net ?? 0)) / max) * 100)}%`, width: '100%' }} />
                      )}
                    </div>
                  </div>
                  <span className={styles.chartTick + (i % 3 === 0 ? ' ' + styles.chartTickB : '')}>
                    {r.turn}.{r.step}
                  </span>
                </div>
              </div>
            )
          })
        )}
      </div>
    </div>
  )
}

export function RequestDetail({
  request,
  prev,
  marker,
}: {
  request: ProjectionRequest | null
  prev?: ProjectionRequest | null
  marker?: ProjectionEvent | null
}) {
  if (!request) return null
  const isDelta = marker !== undefined && prev !== undefined
  void isDelta
  return (
    <div className={styles.reqDetail} data-testid="ctx-reqdetail">
      <div className={styles.reqDetailHead}>
        <span className={styles.reqDetailTag}>
          T{request.turn} · S{request.step}
        </span>
        {request.stepCount && request.stepCount > 1 && <span className={styles.reqDetailTag}>{request.stepCount} 步</span>}
        {marker && <span className={styles.reqDetailMarker}>{EVENT_ICONS[marker.kind] ?? ''} {marker.name ?? marker.kind}</span>}
        <span className={styles.reqDetailTag}>合计 {fmtNum(request.tokens.total)}</span>
      </div>
      <div className={styles.brief}>
        {request.opener && (
          <div>
            <span className={styles.briefLbl}>输入</span>
            {request.opener}
          </div>
        )}
        {request.inputs && (
          <div>
            <span className={styles.briefLbl}>工具</span>
            {request.inputs}
          </div>
        )}
        {request.response && (
          <div>
            <span className={styles.briefLbl}>回复</span>
            {request.response}
          </div>
        )}
      </div>
    </div>
  )
}