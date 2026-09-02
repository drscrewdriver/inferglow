import { useEffect } from 'react'
import { useTodoStore } from './TodoStore'
import styles from './todo.module.css'

/**
 * 待办清单面板 — shows the most recent run's steps as a progress-tracked
 * todo list. Registered into `details.panel.items`, aligned to the HTML
 * prototype's 待办清单 dock panel. Degrades silently to an empty state.
 */
export function TodoPanel() {
  const items = useTodoStore((s) => s.items)
  const runLabel = useTodoStore((s) => s.runLabel)
  const loading = useTodoStore((s) => s.loading)
  const error = useTodoStore((s) => s.error)
  const refresh = useTodoStore((s) => s.refresh)

  useEffect(() => {
    void refresh()
  }, [refresh])

  const done = items.filter((i) => i.done).length
  const pct = items.length ? Math.round((done / items.length) * 100) : 0

  return (
    <section className={styles.card}>
      <div className={styles.cardHead}>
        <span className={styles.emoji}>☑</span>
        <span className={styles.cardTitle}>待办清单</span>
        {items.length > 0 && (
          <span className={styles.badge}>
            {done}/{items.length}
          </span>
        )}
        <button className={styles.refresh} onClick={() => void refresh()} title="刷新待办">
          ⟳
        </button>
      </div>
      <div className={styles.cardBody}>
        {loading && <div className={styles.empty}>加载中…</div>}
        {!loading && error && <div className={styles.empty}>暂无可用的待办。</div>}
        {!loading && !error && items.length === 0 && <div className={styles.empty}>暂无待办。</div>}
        {!loading && items.length > 0 && (
          <>
            {runLabel && <div className={styles.runLabel}>{runLabel}</div>}
            <div className={styles.progress}>
              <div className={styles.bar}>
                <i style={{ width: `${pct}%` }} />
              </div>
              <span className={styles.pct}>{pct}%</span>
            </div>
            {items.map((t, i) => (
              <div className={styles.item} key={`${t.step}-${i}`}>
                <span className={`${styles.st} ${t.done ? styles.stDone : styles.stPending}`}>
                  {t.done ? '✓' : ''}
                </span>
                <span className={`${styles.txt} ${t.done ? styles.txtDone : ''}`}>{t.step}</span>
                {t.duration && <span className={styles.dur}>{t.duration}</span>}
              </div>
            ))}
          </>
        )}
      </div>
    </section>
  )
}
