import { useTrafficStore, selectFrozen } from './trafficStore'
import styles from './traffic.module.css'

export interface FreezeButtonProps {
  /** Backend offline / demo — the composer is disabled regardless. */
  disabled?: boolean
}

/**
 * Toggles freeze on the active conversation: while frozen the composer is
 * disabled and the queue stops draining. Click again to resume.
 */
export function FreezeButton({ disabled }: FreezeButtonProps) {
  const frozen = useTrafficStore(selectFrozen)
  const setFreeze = useTrafficStore((s) => s.setFreeze)

  if (disabled) {
    return <span className={styles.freezeBtn} title="后端未连接">🧊</span>
  }

  return (
    <button
      className={`${styles.freezeBtn}${frozen ? ` ${styles.frozen}` : ''}`}
      onClick={() => setFreeze(!frozen)}
      title={frozen ? '恢复：重新投放被冻结的队列' : '冻结：暂停队列投放并锁定输入'}
    >
      {frozen ? '❄ 已冻结 · 点击恢复' : '🧊 冻结'}
    </button>
  )
}