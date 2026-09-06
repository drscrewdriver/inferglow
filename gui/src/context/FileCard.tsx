// FileCard — Agent file-operation tracker (read/write/search/image/dir);
// each row locates the matching step via onLocate.

import { FILE_BADGES, fmtClock } from './palette'
import type { FileOp } from './types'
import styles from './context.module.css'

export interface FileCardProps {
  files: FileOp[]
  scope?: string
  onLocate?: (op: FileOp) => void
}

export function FileCard({ files, scope, onLocate }: FileCardProps) {
  return (
    <div className={styles.card} data-testid="ctx-filecard">
      <div className={styles.cardTitle}>
        <span className={styles.cardTitleText}>文件活动</span>
        {scope && <span className={styles.cardSub}>{scope}</span>}
      </div>
      <div className={styles.files}>
        {files.length === 0 && <div className={styles.empty}>当前步骤无文件操作</div>}
        {files.map((f, i) => {
          const badge = FILE_BADGES[f.kind]
          return (
            <div
              key={`${f.seq}-${i}`}
              className={styles.fileRow}
              data-testid="ctx-file"
              onClick={onLocate ? () => onLocate(f) : undefined}
              title={f.summary}
            >
              <span className={styles.fileBadge} style={{ background: badge.color + '22', color: badge.color }}>
                {badge.label}
              </span>
              <span className={styles.filePath}>{f.path}</span>
              <span className={styles.eventRight}>{fmtClock(f.time)}</span>
            </div>
          )
        })}
      </div>
    </div>
  )
}