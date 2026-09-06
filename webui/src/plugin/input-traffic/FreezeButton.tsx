/* input-traffic 插件 — 冻结按钮 FreezeButton（渲染于 Composer 右侧） */

import { useFreezeStore, selectFrozen } from './store'
import styles from './input-traffic.module.css'

export interface FreezeButtonProps {
  /** 后端离线 / 演示态 —— 无论如何都禁用输入。 */
  disabled?: boolean
}

/**
 * 切换当前会话的冻结状态：冻结时暂停队列投放并锁定输入；点击恢复。
 */
export function FreezeButton({ disabled }: FreezeButtonProps) {
  const frozen = useFreezeStore(selectFrozen)
  const setFreeze = useFreezeStore((s) => s.setFreeze)

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