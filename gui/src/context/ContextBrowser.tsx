// ContextBrowser — six-category accordion: for each category bucket, list its
// contribution across all step records with per-step breakdown (spec Task 14).

import { useState } from 'react'
import { CATEGORY_COLORS, CATEGORY_ORDER, fmtTokens } from './palette'
import type { Composition, ProjectionRequest } from './types'
import styles from './context.module.css'

export interface ContextBrowserProps {
  requests: ProjectionRequest[]
  hoverKey?: string | null
  onHoverKey?: (k: string | null) => void
}

const CATEGORY_LABELS_SHORT: Record<string, string> = {
  system: '系统提示',
  tools: '工具定义',
  user: '用户消息',
  inject: '注入内容',
  assistant: '助手回复',
  tool: '工具结果',
}

function sumOf(requests: ProjectionRequest[], key: keyof Omit<Composition, 'total'>): number {
  return requests.reduce((a, r) => a + (r.tokens[key] ?? 0), 0)
}

export function ContextBrowser({ requests, hoverKey, onHoverKey }: ContextBrowserProps) {
  const [open, setOpen] = useState<Set<string>>(new Set(['system', 'tools']))
  const toggle = (k: string) =>
    setOpen((s) => {
      const next = new Set(s)
      if (next.has(k)) next.delete(k)
      else next.add(k)
      return next
    })

  const totals: Record<string, number> = {}
  for (const key of CATEGORY_ORDER) totals[key] = sumOf(requests, key)
  const grand = requests.reduce((a, r) => a + r.tokens.total, 0) || 1

  return (
    <div className={styles.card} data-testid="ctx-browser">
      <div className={styles.cardTitle}>
        <span className={styles.cardTitleText}>上下文浏览器</span>
        <span className={styles.cardSub}>六类内容 · 按步骤回溯</span>
      </div>
      {requests.length === 0 ? (
        <div className={styles.empty}>暂无步骤数据</div>
      ) : (
        <div className={styles.tree}>
          {CATEGORY_ORDER.map((key) => {
            const isOpen = open.has(key)
            return (
              <div
                key={key}
                className={styles.treeItem + (isOpen ? ' ' + styles.treeOpen : '')}
                onMouseEnter={onHoverKey ? () => onHoverKey(key) : undefined}
                onMouseLeave={onHoverKey ? () => onHoverKey(null) : undefined}
                style={hoverKey && hoverKey !== key ? { opacity: 0.5 } : undefined}
              >
                <button className={styles.treeHead} onClick={() => toggle(key)}>
                  <span className={styles.treeDot} style={{ background: CATEGORY_COLORS[key] }} />
                  <span className={styles.treeName}>{CATEGORY_LABELS_SHORT[key]}</span>
                  <span className={styles.treeCount}>{requests.length} 步</span>
                  <span className={styles.treePct}>
                    {fmtTokens(totals[key])} · {((totals[key] / grand) * 100).toFixed(1)}%
                  </span>
                  <span className={styles.treeChev}>›</span>
                </button>
                {isOpen && (
                  <div className={styles.treeBody}>
                    {requests.map((r) => {
                      const v = r.tokens[key] ?? 0
                      if (v <= 0) return null
                      return (
                        <div key={`${key}-${r.seq}`} className={styles.treeRow}>
                          <span className={styles.treeRowMeta}>
                            T{r.turn}.S{r.step}
                          </span>
                          <span className={styles.treeRowBrief}>{r.response || r.opener || r.inputs || ''}</span>
                          <span className={styles.treeRowTok}>{fmtTokens(v)}</span>
                        </div>
                      )
                    })}
                  </div>
                )}
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}