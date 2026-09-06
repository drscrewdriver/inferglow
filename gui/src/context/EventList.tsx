// EventList — context events (compaction/prune/inject/model/mode) with
// kind-toggle chips and icons.

import { useState } from 'react'
import { EVENT_ICONS, fmtClock, fmtTokens } from './palette'
import type { ProjectionEvent } from './types'
import styles from './context.module.css'

const EVENT_KINDS = ['inject', 'compaction', 'prune', 'model', 'mode'] as const

const KIND_LABEL: Record<string, string> = {
  inject: '注入',
  compaction: '压缩',
  prune: '剪枝',
  model: '模型',
  mode: '模式',
}

export function EventList({ events }: { events: ProjectionEvent[] }) {
  const [picked, setPicked] = useState<string[]>([...EVENT_KINDS])
  const toggle = (k: string) => {
    setPicked((p) => {
      if (p.length === EVENT_KINDS.length) return [k]
      if (!p.includes(k)) return [...p, k]
      return p.length === 1 ? [...EVENT_KINDS] : p.filter((x) => x !== k)
    })
  }
  const shown = picked.length === EVENT_KINDS.length ? events : events.filter((e) => picked.includes(e.kind))
  return (
    <div>
      <div className={styles.kinds}>
        {EVENT_KINDS.map((k) => (
          <button
            key={k}
            className={styles.granBtn + (picked.includes(k) ? ' ' + styles.granOn : '')}
            onClick={() => toggle(k)}
          >
            {KIND_LABEL[k]}
          </button>
        ))}
      </div>
      <div className={styles.events} style={{ marginTop: 10 }}>
        {shown.length === 0 && <div className={styles.empty}>暂无事件</div>}
        {shown.map((ev, i) => (
          <div key={`${ev.seq}-${i}`} className={styles.eventRow} data-testid="ctx-event">
            <span className={styles.eventIcon}>{EVENT_ICONS[ev.kind] ?? '•'}</span>
            <div className={styles.eventBody}>
              <span className={styles.eventName}>{KIND_LABEL[ev.kind]} {ev.name ?? ''}</span>
              {ev.detail && <span className={styles.eventDetail}>{ev.detail}</span>}
            </div>
            <span className={styles.eventRight}>
              {ev.tokens !== undefined ? fmtTokens(ev.tokens) : ''}
              {' '}
              {fmtClock(ev.time)}
            </span>
          </div>
        ))}
      </div>
    </div>
  )
}