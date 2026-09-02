// CurrentComposition — six-color stacked bar + legend + 80% auto-compact
// reserve (dsh AUTO_COMPACT_RATIO), plus a shared StackedBar/Legend pair the
// TrendChart reuses.

import { AUTO_COMPACT_RATIO } from './token'
import { CATEGORY_COLORS, CATEGORY_LABELS, CATEGORY_ORDER, fmtTokens } from './palette'
import type { Composition } from './types'
import styles from './context.module.css'

export function StackedBar({
  comp,
  height = 16,
  hoverKey,
  onHoverKey,
  reserve,
}: {
  comp: Readonly<Composition>
  height?: number
  hoverKey?: string | null
  onHoverKey?: (k: string | null) => void
  reserve?: { ratio: number; label: string }
}) {
  const total = comp.total || 1
  return (
    <div>
      <div
        className={styles.bar + (onHoverKey ? ' ' + styles.barDim : '')}
        style={{ height }}
        data-testid="ctx-stackedbar"
      >
        {CATEGORY_ORDER.map((key) => {
          const v = comp[key] ?? 0
          if (v <= 0) return null
          return (
            <div
              key={key}
              className={styles.barSeg}
              style={{
                width: `${(v / total) * 100}%`,
                background: CATEGORY_COLORS[key],
                filter: hoverKey && hoverKey !== key ? 'opacity(0.35)' : undefined,
              }}
              onMouseEnter={onHoverKey ? () => onHoverKey(key) : undefined}
              onMouseLeave={onHoverKey ? () => onHoverKey(null) : undefined}
              title={`${CATEGORY_LABELS[key]} ${fmtTokens(v)} (${((v / total) * 100).toFixed(1)}%)`}
            />
          )
        })}
      </div>
      {reserve && <div className={styles.reserve}>{reserve.label}</div>}
    </div>
  )
}

export function Legend({
  comp,
  hoverKey,
  onHoverKey,
}: {
  comp: Readonly<Composition>
  hoverKey?: string | null
  onHoverKey?: (k: string | null) => void
}) {
  const total = comp.total || 1
  return (
    <div className={styles.legend} data-testid="ctx-legend">
      {CATEGORY_ORDER.map((key) => (
        <div
          key={key}
          className={styles.legendItem}
          style={onHoverKey && hoverKey && hoverKey !== key ? { opacity: 0.45 } : undefined}
          onMouseEnter={onHoverKey ? () => onHoverKey(key) : undefined}
          onMouseLeave={onHoverKey ? () => onHoverKey(null) : undefined}
        >
          <span className={styles.legendDot} style={{ background: CATEGORY_COLORS[key] }} />
          <span className={styles.legendKey}>{CATEGORY_LABELS[key]}</span>
          <span className={styles.legendVal}>
            {fmtTokens(comp[key] ?? 0)}
            <span style={{ opacity: 0.6 }}> · {((comp[key] ?? 0) / total * 100).toFixed(1)}%</span>
          </span>
        </div>
      ))}
    </div>
  )
}

export function CurrentComposition({
  comp,
  subtitle,
  contextWindow,
  hoverKey,
  onHoverKey,
}: {
  comp: Readonly<Composition>
  subtitle?: string
  contextWindow?: number
  hoverKey?: string | null
  onHoverKey?: (k: string | null) => void
}) {
  const pct = contextWindow && contextWindow > 0 ? Math.round((comp.total / contextWindow) * 100) : null
  const reserve = contextWindow && contextWindow > 0
    ? { ratio: AUTO_COMPACT_RATIO, label: `预留 ${Math.round(AUTO_COMPACT_RATIO * 100)}% 自动压缩空间` }
    : undefined
  return (
    <div className={styles.card} data-testid="ctx-current">
      <div className={styles.cardTitle}>
        <span className={styles.cardTitleText}>当前上下文</span>
        {subtitle && <span className={styles.cardSub}>{subtitle}</span>}
      </div>
      <div className={styles.overviewNum}>
        <b>{fmtTokens(comp.total)}</b>
        <span>{contextWindow ? `/ ${fmtTokens(contextWindow)} tokens` : 'tokens（估算）'}</span>
        {pct !== null && (
          <span className={styles.overviewPct}>
            <b>{pct}%</b>已用
          </span>
        )}
      </div>
      <StackedBar comp={comp} hoverKey={hoverKey} onHoverKey={onHoverKey} reserve={reserve} />
      <Legend comp={comp} hoverKey={hoverKey} onHoverKey={onHoverKey} />
    </div>
  )
}