/* input-traffic 插件 — 三层规划队列 SteerQueueDock（渲染于 Composer 上方）
 * 显示忠实还原 dsh-input-traffic 的 steer-queue-dock：扁平单列 + 行内徽章 +
 * 折叠表头 / 两级确认清空 / steering 列表 / frozen 面板。 */

import { useEffect, useMemo, useRef, useState } from 'react'
import {
  useTrafficStore,
  useFreezeStore,
  selectQueued,
  selectSteering,
  selectCount,
  selectFrozen,
  selectFreeze,
} from './store'
import { TIER_ICON, TIER_LABEL } from './store'
import type { QueueTier } from './api'
import type { TrafficQueueItem } from './store'
import styles from './input-traffic.module.css'

export interface SteerQueueDockProps {
  /** 将队列项打回输入框的回调。 */
  onPullBack?: (text: string) => void
}

const COLLAPSE_KEY = 'inferglow:input-traffic:collapsed'

/** 行内 tier 徽章：彩色圆点 + 文案（对齐 SteerBadge）。 */
function TierBadge({ tier }: { tier: QueueTier }) {
  const cls = (styles as Record<string, string>)[`badge_${tier}`] ?? ''
  return (
    <span className={`${styles.badge} ${cls}`} data-badge={tier}>
      <span className={styles.dot} aria-hidden />
      <span className={styles.badgeLabel}>{TIER_LABEL[tier]}</span>
    </span>
  )
}

/** 一组三档规划圆点（红 now / 黄 next / 绿 later）；当前档位高亮。 */
function TierDots({
  current,
  nowable,
  disabled,
  onNow,
  onNext,
  onLater,
}: {
  current: QueueTier
  nowable: boolean
  disabled: boolean
  onNow: () => void
  onNext: () => void
  onLater: () => void
}) {
  return (
    <span className={styles.plan} role="group" title="规划插入档位">
      {([
        ['now', onNow, nowable],
        ['next', onNext, true],
        ['later', onLater, true],
      ] as const).map(([tier, fn, en]) => (
        <button
          key={tier}
          type="button"
          className={`${styles.tier} ${styles[`tier_${tier}` as keyof typeof styles]}${current === tier ? ` ${styles.tierActive}` : ''}`}
          title={`${tier === 'now' ? '立即（打断并重发）' : TIER_LABEL[tier]}`}
          aria-pressed={current === tier || undefined}
          disabled={disabled || !en}
          onClick={fn}
        >
          <span className={styles.dot} aria-hidden />
        </button>
      ))}
    </span>
  )
}

/** 主列表 / steering 列表共用一行：徽章 + 内容(preview/editor) + 操作 + 档位点。 */
function QueueRow({
  item,
  bare,
  canPullBack,
  onPullBack,
}: {
  item: TrafficQueueItem
  bare: boolean
  canPullBack: boolean
  onPullBack?: (t: string) => void
}) {
  const edit = useTrafficStore((s) => s.edit)
  const remove = useTrafficStore((s) => s.remove)
  const steer = useTrafficStore((s) => s.steer)
  const move = useTrafficStore((s) => s.move)
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(item.text)
  const editorRef = useRef<HTMLTextAreaElement>(null)

  useEffect(() => setDraft(item.text), [item.text])
  useEffect(() => {
    if (editing && editorRef.current) {
      const el = editorRef.current
      el.style.height = 'auto'
      el.style.height = `${el.scrollHeight}px`
    }
  }, [editing])

  const saveEdit = () => {
    const t = draft.trim()
    if (t && t !== item.text) edit(item.id, t)
    else setDraft(item.text)
    setEditing(false)
  }

  return (
    <li
      className={styles.row}
      data-tier={item.tier}
      data-editing={editing ? '' : undefined}
    >
      {bare && (
        <span className={styles.lead} aria-hidden>
          {TIER_ICON.later.slice(0, 1) || 'Q'}
        </span>
      )}
      <TierBadge tier={item.tier} />
      {editing ? (
        <textarea
          ref={editorRef}
          autoFocus
          className={styles.editor}
          rows={1}
          value={draft}
          onInput={(e) => {
            const el = e.currentTarget
            el.style.height = 'auto'
            el.style.height = `${el.scrollHeight}px`
          }}
          onChange={(e) => setDraft(e.currentTarget.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
              e.preventDefault()
              saveEdit()
            } else if (e.key === 'Escape') {
              setDraft(item.text)
              setEditing(false)
            }
          }}
        />
      ) : (
        <span className={styles.preview}>{item.text}</span>
      )}
      <div className={styles.actions}>
        {!editing ? (
          <>
            <button className={styles.action} title="上移" onClick={() => move(item.id, -1)}>▲</button>
            <button className={styles.action} title="下移" onClick={() => move(item.id, 1)}>▼</button>
            <button
              className={styles.action}
              title="打回输入框"
              disabled={!canPullBack}
              onClick={() => {
                if (!canPullBack) return
                remove(item.id)
                onPullBack?.(item.text)
              }}
            >↩</button>
            <button className={styles.action} title="编辑" onClick={() => setEditing(true)}>✎</button>
            <button className={`${styles.action} ${styles.danger}`} title="移除" onClick={() => remove(item.id)}>✕</button>
            <TierDots
              current={item.tier}
              nowable={!bare}
              disabled={false}
              onNow={() => !bare && steer(item.id, 'now')}
              onNext={() => steer(item.id, 'next', true)}
              onLater={() => steer(item.id, 'later')}
            />
          </>
        ) : (
          <>
            <button className={styles.action} title="保存" onClick={saveEdit}>✓</button>
            <button className={styles.action} title="取消编辑" onClick={() => { setDraft(item.text); setEditing(false) }}>✕</button>
          </>
        )}
      </div>
      <span className={styles.frozenMark} aria-hidden />
    </li>
  )
}

/** 折叠表头 + 两级确认清空 + 显式投放（flush）。 */
export function SteerQueueDock({ onPullBack }: SteerQueueDockProps) {
  const queued = useTrafficStore(selectQueued)
  const steering = useTrafficStore(selectSteering)
  const count = useTrafficStore(selectCount)
  const frozen = useFreezeStore(selectFrozen)
  const freeze = useFreezeStore(selectFreeze)
  const clear = useTrafficStore((s) => s.clear)
  const flush = useTrafficStore((s) => s.flush)
  const disabled = frozen || count === 0

  const [collapsed, setCollapsed] = useState(() => {
    try {
      const v = localStorage.getItem(COLLAPSE_KEY)
      return v === null ? true : v === '1'
    } catch {
      return true
    }
  })
  const [confirmClear, setConfirmClear] = useState(false)
  const confirmTimer = useRef<number>(0)

  useEffect(() => {
    try {
      localStorage.setItem(COLLAPSE_KEY, collapsed ? '1' : '0')
    } catch {
      /* 存储不可用：仅内存 */
    }
  }, [collapsed])

  useEffect(() => {
    if (confirmClear) {
      window.clearTimeout(confirmTimer.current)
      confirmTimer.current = window.setTimeout(() => setConfirmClear(false), 3000)
    }
    return () => window.clearTimeout(confirmTimer.current)
  }, [confirmClear])

  const expanded = !collapsed
  const single = count === 1 && steering.length === 0
  const listVisible = single || expanded
  const frozenItems = useMemo(() => freeze?.items ?? [], [freeze])

  const armClear = () => {
    if (confirmClear) {
      window.clearTimeout(confirmTimer.current)
      setConfirmClear(false)
      clear()
    } else {
      setConfirmClear(true)
    }
  }

  return (
    <div className={styles.dock} data-steer-dock="">
      <div className={styles.panel}>
        <div className={styles.toolbar}>
          <button
            type="button"
            className={styles.header}
            disabled={count <= 1 || confirmClear}
            onClick={() => setCollapsed((v) => !v)}
          >
            <span className={styles.lead} aria-hidden>📨</span>
            {count > 0 && <span className={styles.count}>{count} 条</span>}
            {count > 1 && (
              <span className={styles.chevron} aria-hidden>
                {listVisible ? '▾' : '▴'}
              </span>
            )}
          </button>
          <div className={styles.toolbarActions}>
            <button
              className={styles.dockBtn}
              onClick={() => void flush()}
              disabled={disabled}
              title="显式投放：按档位依次发送整条队列"
            >
              ⏩ 投放
            </button>
            {confirmClear && (
              <button
                type="button"
                className={styles.clearCancel}
                onClick={() => setConfirmClear(false)}
              >
                取消
              </button>
            )}
            <button
              type="button"
              className={`${styles.dockBtn} ${styles.clear}${confirmClear ? ` ${styles.clearConfirm}` : ''}`}
              disabled={disabled}
              onClick={armClear}
            >
              🗑 {confirmClear ? '确认清空' : '清空'}
            </button>
          </div>
        </div>

        {frozen && (
          <div className={styles.frozenBanner} role="status">
            已冻结：队列已暂停，点击输入区右侧 ❄ 恢复
          </div>
        )}

        {frozen && frozenItems.length > 0 && (
          <ul className={styles.frozenList} data-frozen-list="">
            {frozenItems.map((entry, i) => (
              <li key={`${entry.id}-${i}`} className={styles.frozenRow} data-tier={entry.tier}>
                <span className={styles.lead} aria-hidden>⛉</span>
                <TierBadge tier={entry.tier} />
                <span className={styles.preview}>{entry.text}</span>
                <span className={styles.frozenBadge}>已冻结</span>
              </li>
            ))}
          </ul>
        )}

        <ul className={styles.list} hidden={!listVisible}>
          {queued.map((row) => (
            <QueueRow
              key={row.id}
              item={row}
              bare={count === 1}
              canPullBack={!steering.length}
              onPullBack={onPullBack}
            />
          ))}
        </ul>

        {steering.length > 0 && (
          <ul className={styles.steeringList} data-steering-list="">
            {steering.map((row) => (
              <QueueRow
                key={row.id}
                item={row}
                bare={false}
                canPullBack={false}
                onPullBack={onPullBack}
              />
            ))}
          </ul>
        )}

        {count === 0 && !frozen && (
          <div className={styles.staticHint}>暂无排队项 —— 可从输入框投递到「稍后 / 下一步 / 立即」。</div>
        )}
      </div>
    </div>
  )
}