import { useEffect, useState } from 'react'
import { useTrafficStore, selectJobs } from './trafficStore'
import type { RunJob } from './types'
import styles from './traffic.module.css'

const STATUS_LABEL: Record<RunJob['status'], string> = {
  ongoing: '进行中',
  stopping: '停止中',
  completed: '已完成',
  killed: '已终止',
  failed: '失败',
}

const STATUS_DOT: Record<RunJob['status'], string> = {
  ongoing: styles.dotOngoing,
  stopping: styles.dotStopping,
  completed: styles.dotCompleted,
  killed: styles.dotKilled,
  failed: styles.dotFailed,
}

/** Format a millisecond duration into a compact human readout. */
function formatDur(ms: number | null | undefined): string {
  if (ms === null || ms === undefined || Number.isNaN(ms) || ms < 0) return '--'
  const s = Math.floor(ms / 1000)
  if (s < 60) return `${s}s`
  const m = Math.floor(s / 60)
  return `${m}m${s % 60}s`
}

const MS = (iso: string | null | undefined): number => (iso ? Date.parse(iso) : Number.NaN)

function jobDuration(j: RunJob, now: number): number {
  const start = MS(j.started_at)
  if (Number.isNaN(start)) return j.duration ?? -1
  const end = MS(j.finished_at)
  const to = Number.isNaN(end) ? now : end
  return to - start
}

/**
 * Render background jobs with a 1s freshness ticker (live duration for
 * ongoing jobs, status refresh). Rendered into `details.panel.items`.
 */
export function JobList() {
  const jobs = useTrafficStore(selectJobs)
  const refreshJobs = useTrafficStore((s) => s.refreshJobs)
  const cancelJob = useTrafficStore((s) => s.cancelJob)
  const retryJob = useTrafficStore((s) => s.retryJob)
  const runId = useTrafficStore((s) => s.runId)
  const [, setTick] = useState(0)

  useEffect(() => {
    const timer = window.setInterval(() => {
      if (runId) void refreshJobs()
      setTick((t) => t + 1)
    }, 1000)
    return () => window.clearInterval(timer)
  }, [runId, refreshJobs])

  const now = Date.now()

  return (
    <div className={styles.jobs}>
      {jobs.length === 0 && <div className={styles.jobEmpty}>暂无后台任务。</div>}
      {jobs.map((j) => (
        <div className={styles.jobRow} key={j.id}>
          <span className={`${styles.jobDot} ${STATUS_DOT[j.status]}`} title={STATUS_LABEL[j.status]} />
          <div className={styles.jobMain}>
            <div className={styles.jobKind}>
              {j.kind} · {STATUS_LABEL[j.status]}
            </div>
            {j.description && <div className={styles.jobDesc}>{j.description}</div>}
          </div>
          <span className={styles.jobDur}>{formatDur(jobDuration(j, now))}</span>
          <div className={styles.jobActions}>
            {j.status === 'ongoing' || j.status === 'stopping' ? (
              <button className={styles.miniBtn} title="取消" onClick={() => cancelJob(j.id)}>
                ■
              </button>
            ) : (
              <button className={styles.miniBtn} title="重试" onClick={() => void retryJob(j.id)}>
                ↻
              </button>
            )}
          </div>
        </div>
      ))}
    </div>
  )
}