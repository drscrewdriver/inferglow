import { useEffect, useRef, useState } from 'react'
import {
  useTrafficStore,
  selectItems,
  selectFrozen,
  selectCount,
} from './trafficStore'
import { TIERS, TIER_ICON, TIER_LABEL } from './tiers'
import type { QueueTier } from './tiers'
import type { TrafficQueueItem } from './trafficStore'
import styles from './traffic.module.css'

export interface SteerQueueDockProps {
  /** Callback to pull a queued item back into the composer input. */
  onPullBack?: (text: string) => void
}

const OTHER_TIERS: QueueTier[] = TIERS

// ─── Single queued item: inline edit + steering + ordering + pull-back ────
function QueueRow({ item, onPullBack }: { item: TrafficQueueItem; onPullBack?: (t: string) => void }) {
  const edit = useTrafficStore((s) => s.edit)
  const remove = useTrafficStore((s) => s.remove)
  const steer = useTrafficStore((s) => s.steer)
  const move = useTrafficStore((s) => s.move)
  const [draft, setDraft] = useState(item.text)

  useEffect(() => setDraft(item.text), [item.text])

  const commit = () => {
    const t = draft.trim()
    if (t && t !== item.text) edit(item.id, t)
    else setDraft(item.text)
  }

  return (
    <div className={styles.item}>
      <span className={styles.itemTierBadge}>{TIER_ICON[item.tier]}</span>
      <div className={styles.itemBody}>
        <textarea
          className={styles.itemText}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onBlur={commit}
          rows={Math.min(3, Math.max(1, draft.split('\n').length))}
        />
        <div className={styles.itemMeta}>
          <select
            value={item.tier}
            title="转到层级"
            onChange={(e) => steer(item.id, e.target.value as QueueTier)}
          >
            {OTHER_TIERS.map((t) => (
              <option key={t} value={t}>
                {t === item.tier ? `已在 ${TIER_LABEL[t]}` : `→ ${TIER_LABEL[t]}`}
              </option>
            ))}
          </select>
          <button className={styles.miniBtn} title="打回输入框" onClick={() => onPullBack?.(item.text)}>
            ↩
          </button>
        </div>
      </div>
      <div className={styles.itemActions}>
        <button className={styles.miniBtn} title="上移" onClick={() => move(item.id, -1)}>
          ▲
        </button>
        <button className={styles.miniBtn} title="下移" onClick={() => move(item.id, 1)}>
          ▼
        </button>
        <button className={`${styles.miniBtn} ${styles.danger}`} title="删除" onClick={() => remove(item.id)}>
          ✕
        </button>
      </div>
    </div>
  )
}

// ─── One colored tier group ───────────────────────────────────────────────
function TierGroup({ tier, onPullBack }: { tier: QueueTier; onPullBack?: (t: string) => void }) {
  const items = useTrafficStore(selectItems)
  const clear = useTrafficStore((s) => s.clear)
  const [confirm, setConfirm] = useState(false)
  const group = items.filter((i) => i.tier === tier)
  const frozen = useTrafficStore(selectFrozen)

  return (
    <div className={styles.tierGroup}>
      <div className={styles.tierHead}>
        <span>{TIER_ICON[tier]}</span>
        <span className={styles.tierLabel}>{TIER_LABEL[tier]}</span>
        {group.length === 0 && <span className={styles.tierEmpty}>空</span>}
        {!frozen && group.length > 0 && (
          <button
            className={`${styles.tierClear}${confirm ? ` ${styles.confirm}` : ''}`}
            onClick={() => {
              if (confirm) {
                clear()
                setConfirm(false)
              } else {
                setConfirm(true)
              }
            }}
          >
            {confirm ? '确认清空？' : `清空 ${group.length}`}
          </button>
        )}
      </div>
      {group.map((item) => (
        <QueueRow key={item.id} item={item} onPullBack={onPullBack} />
      ))}
    </div>
  )
}

// ─── Dock: three-level queue + global clear + flush ───────────────────────
export function SteerQueueDock({ onPullBack }: SteerQueueDockProps) {
  const items = useTrafficStore(selectItems)
  const count = useTrafficStore(selectCount)
  const frozen = useTrafficStore(selectFrozen)
  const clear = useTrafficStore((s) => s.clear)
  const drain = useTrafficStore((s) => s.drain)
  const [confirmAll, setConfirmAll] = useState(false)
  const confirmTimer = useRef<number>(0)

  // Auto-drain whenever the queue is non-empty and not frozen.
  useEffect(() => {
    if (!frozen && count > 0) void drain()
  }, [frozen, count, drain])

  return (
    <div className={styles.dock}>
      <div className={styles.dockHead}>
        <span className={styles.dockTitle}>📨 投放队列</span>
        <span>{count} 条</span>
        <span className={styles.dockSpacer} />
        <button className={styles.dockBtn} onClick={() => void drain()} disabled={frozen || count === 0} title="立即发送队列">
          ⏩ 发送
        </button>
        <button
          className={`${styles.dockBtn}${confirmAll ? ` ${styles.confirm}` : ''}`}
          disabled={frozen || count === 0}
          onClick={() => {
            if (confirmAll) {
              clear()
              setConfirmAll(false)
            } else {
              setConfirmAll(true)
              window.clearTimeout(confirmTimer.current)
              confirmTimer.current = window.setTimeout(() => setConfirmAll(false), 2500)
            }
          }}
        >
          {confirmAll ? '确认清空？' : '🗑 清空'}
        </button>
      </div>
      {items.length === 0 && !frozen && (
        <div className={styles.tierEmpty}>暂无排队项 —— 可从输入框投递到「稍后 / 下一步 / 立即」。</div>
      )}
      {TIERS.map((t) => (
        <TierGroup key={t} tier={t} onPullBack={onPullBack} />
      ))}
    </div>
  )
}