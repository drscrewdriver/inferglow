// ProducedFiles — turn-ended modified-file chips (Phase 5, Task 19). Derives
// the latest invested file ops (write/edit/create…) from the message stream via
// the context fold and renders them as clickable chips.

import { useMemo } from 'react'
import { useChatStore } from '../stores/chatStore'
import { fold } from '../context/fold'
import { FILE_BADGES } from '../context/palette'
import styles from './mention.module.css'

export interface ProducedFilesProps {
  onOpen?: (path: string) => void
}

export function ProducedFiles({ onOpen }: ProducedFilesProps) {
  const messages = useChatStore((s) => s.messages)
  const files = useMemo(() => fold(messages).files, [messages])
  // Only invested files (writes/edits) — reads/searches aren't "produced".
  const produced = files.filter((f) => f.kind === 'write')

  if (produced.length === 0) return null

  return (
    <div className={styles.produced} data-testid="produced-files">
      {produced.map((f, i) => {
        const badge = FILE_BADGES[f.kind]
        return (
          <button
            key={`${f.seq}-${i}`}
            className={styles.producedChip}
            onClick={onOpen ? () => onOpen(f.path) : undefined}
            title={f.summary ?? f.path}
          >
            <span className={styles.producedKind} style={{ color: badge.color }}>
              {badge.label}
            </span>
            {f.path}
          </button>
        )
      })}
    </div>
  )
}