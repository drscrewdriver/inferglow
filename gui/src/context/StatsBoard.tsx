// StatsBoard — 9-cell overview (spec Task 14): turns / steps / injects /
// compactions / prunes / tool calls / images / cache-hit / cost.

import type { Composition, ProjectionEvent, ProjectionRequest } from './types'
import styles from './context.module.css'
import { fmtTokens } from './palette'

export interface StatsBoardProps {
  requests: ProjectionRequest[]
  events: ProjectionEvent[]
  current: Readonly<Composition>
  toolCalls: number
  images: number
}

/** Form a styled '?'-marked cell; tip is shown on hover. */
function cell(label: string, value: string, tip?: string) {
  return (
    <div className={styles.stat + (tip ? ' ' + styles.statTipped : '')}>
      <span className={styles.statLabel}>
        {label}
        {tip && <i className={styles.statQ}>?</i>}
      </span>
      <b className={styles.statValue}>{value}</b>
      {tip && (
        <span className={styles.statTip} role="tooltip">
          {tip}
        </span>
      )}
    </div>
  )
}

export function StatsBoard({ requests, events, current, toolCalls, images }: StatsBoardProps) {
  const turns = new Set<number>()
  let compactions = 0
  let prunes = 0
  let injects = 0
  for (const ev of events) {
    if (ev.kind === 'compaction') compactions++
    else if (ev.kind === 'prune') prunes++
    else if (ev.kind === 'inject') injects++
  }
  for (const r of requests) turns.add(r.turn)
  const steps = requests.length
  const cacheHit = current.assistant > 0
    ? Math.round((current.tools / (current.tools + current.assistant + 1)) * 1000) / 10
    : null

  return (
    <div className={styles.card}>
      <div className={styles.cardTitle}>
        <span className={styles.cardTitleText}>上下文概览</span>
        <span className={styles.cardSub}>轮次 · 步数 · 事件 · 用量</span>
      </div>
      <div className={styles.stats}>
        {cell('轮次', String(turns.size))}
        {cell('步数', String(steps))}
        {cell('注入', String(injects))}
        {cell('压缩', String(compactions))}
        {cell('剪枝', String(prunes))}
        {cell('工具调用', String(toolCalls), '当前上下文内仍有效的工具结果数量')}
        {cell('图片', String(images), '上下文中存活的图片块数量')}
        {cell('缓存命中', cacheHit === null ? '—' : cacheHit + '%', '使用工具结果/助手 token 比例的启发式估算')}
        {cell('总 token', fmtTokens(current.total), '当前上下文的六类 token 合计（启发式估算）')}
      </div>
    </div>
  )
}