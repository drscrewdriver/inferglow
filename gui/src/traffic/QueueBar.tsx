import {
  useTrafficStore,
  selectItems,
  selectFrozen,
} from './trafficStore'
import { TIER_LABEL } from './tiers'
import type { TrafficQueueItem } from './trafficStore'
import styles from './traffic.module.css'

export interface QueueBarProps {
  onPullBack?: (text: string) => void
}

/** Compact floating queue bar above the composer.
 *  Only visible when items are queued or frozen.
 *  Shows one row per item with a colored status dot + truncated text. */
export function QueueBar({ onPullBack }: QueueBarProps) {
  const items = useTrafficStore(selectItems)
  const frozen = useTrafficStore(selectFrozen)
  const remove = useTrafficStore((s) => s.remove)

  // When "next" (🟡) is active, only show "later" (🟢) items as fallback.
  const hasActive = items.some((i) => i.tier === 'next' || i.tier === 'now')
  const visible = frozen || items.length > 0
  if (!visible) return null

  // Filter: if any next/now item exists, hide later items from display.
  const displayItems = hasActive
    ? items.filter((i) => i.tier !== 'later')
    : items

  return (
    <div className={styles.queueBar}>
      {frozen && (
        <div className={styles.queueFrozen}>
          <span className={styles.queueFrozenDot} />
          <span>已冻结：当前轮次完成后暂停</span>
          <span className={styles.queueSpacer} />
          <button
            className={styles.queueCancelBtn}
            onClick={() => useTrafficStore.getState().setFreeze(false)}
          >
            解冻
          </button>
        </div>
      )}
      {displayItems.map((item) => (
        <QueueBarRow
          key={item.id}
          item={item}
          onRemove={() => remove(item.id)}
          onPullBack={onPullBack}
        />
      ))}
    </div>
  )
}

function QueueBarRow({
  item,
  onRemove,
  onPullBack,
}: {
  item: TrafficQueueItem
  onRemove: () => void
  onPullBack?: (text: string) => void
}) {
  const dotColor =
    item.tier === 'now'
      ? 'var(--igw-err)'
      : item.tier === 'next'
        ? 'var(--igw-warn)'
        : 'var(--igw-ok)'

  return (
    <div className={styles.queueRow}>
      <span
        className={styles.queueDot}
        style={{ background: dotColor }}
      />
      <span className={styles.queueLabel}>{TIER_LABEL[item.tier]}</span>
      <span className={styles.queueText}>{item.text}</span>
      <span className={styles.queueSpacer} />
      <button
        className={styles.queueAction}
        title="拉回输入框"
        onClick={() => onPullBack?.(item.text)}
      >
        ↩
      </button>
      <button
        className={styles.queueAction}
        title="删除"
        onClick={onRemove}
      >
        ✕
      </button>
    </div>
  )
}
