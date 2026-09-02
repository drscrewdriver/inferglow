import styles from './tidychat.module.css'
import { foldButtonLabel, foldLabel } from './logic'
import type { TzTurn } from './logic'

/** Control bar that sits above every completed (completed turn) fold group. */
export function FoldTurn({
  turn,
  folded,
  enabled,
  onToggle,
}: {
  turn: TzTurn
  folded: boolean
  enabled: boolean
  onToggle: () => void
}) {
  return (
    <div data-tidychat-divider-block data-tidychat-turn={turn.turn} className={styles.foldBar} role="separator">
      <span className={styles.foldLabel}>{foldLabel(turn, folded, enabled)}</span>
      <span className={styles.foldLine} />
      <button type="button" className={styles.foldBtn} onClick={onToggle}>
        {foldButtonLabel(folded)}
      </button>
    </div>
  )
}