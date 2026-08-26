import { useEffect } from 'react'
import { useMemoryStore } from './MemoryStore'
import type { MemoryRecord } from '../api'
import styles from './memory.module.css'

/** Truncate long memory content to a single-line title. */
function titleOf(m: MemoryRecord): string {
  const t = m.content.trim() || m.facts?.[0]?.trim() || '（无内容）'
  return t.length > 42 ? `${t.slice(0, 42)}…` : t
}

/**
 * 记忆面板 — lists persisted memory facts from the backend. Registered into
 * `details.panel.items` (right-side panel strip), aligned to the HTML
 * prototype's 记忆 dock panel. Degrades silently to an empty state when the
 * backend is unavailable.
 */
export function MemoryPanel() {
  const memories = useMemoryStore((s) => s.memories)
  const loading = useMemoryStore((s) => s.loading)
  const error = useMemoryStore((s) => s.error)
  const refresh = useMemoryStore((s) => s.refresh)
  const remove = useMemoryStore((s) => s.remove)

  useEffect(() => {
    void refresh()
  }, [refresh])

  return (
    <section className={styles.card}>
      <div className={styles.cardHead}>
        <span className={styles.emoji}>🧠</span>
        <span className={styles.cardTitle}>记忆</span>
        {memories.length > 0 && <span className={styles.badge}>{memories.length}</span>}
        <button className={styles.refresh} onClick={() => void refresh()} title="刷新记忆">
          ⟳
        </button>
      </div>
      <div className={styles.cardBody}>
        {loading && <div className={styles.empty}>加载中…</div>}
        {!loading && error && <div className={styles.empty}>暂无可用的记忆。</div>}
        {!loading && !error && memories.length === 0 && <div className={styles.empty}>暂无记忆。</div>}
        {!loading &&
          memories.map((m) => (
            <div className={styles.mem} key={m.id}>
              <div className={styles.memHead}>
                <span className={styles.memTitle}>{titleOf(m)}</span>
                {m.category && <span className={styles.fresh}>{m.category}</span>}
                <button className={styles.del} onClick={() => void remove(m.id)} title="删除记忆">
                  ✕
                </button>
              </div>
              <div className={styles.memBody}>{m.facts?.join(' · ') || m.content}</div>
            </div>
          ))}
      </div>
    </section>
  )
}
