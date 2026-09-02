import { useEffect } from 'react'
import { useSubAgentStore } from './SubAgentStore'
import type { AgentRecord } from '../api'
import styles from './subagents.module.css'

/** Best-effort field reads; concrete agent JSON varies by backend impl. */
function nameOf(a: AgentRecord): string {
  const n = a.name ?? a.id
  return typeof n === 'string' && n ? n : '（未命名）'
}
function modelOf(a: AgentRecord): string | null {
  const m = a.model ?? a.agent_model ?? a.model_name
  return typeof m === 'string' && m ? m : null
}
function descOf(a: AgentRecord): string {
  const d = a.system_prompt ?? a.description ?? a.prompt
  return typeof d === 'string' && d.trim() ? d.trim() : '子智能体 · 由后端 agents 提供'
}

/**
 * 子智能体面板 — lists backend agents as sub-agent profile cards.
 * Registered into `details.panel.items`, aligned to the HTML prototype's
 * 子智能体 dock panel. Degrades silently to an empty state.
 */
export function SubAgentPanel() {
  const agents = useSubAgentStore((s) => s.agents)
  const loading = useSubAgentStore((s) => s.loading)
  const error = useSubAgentStore((s) => s.error)
  const refresh = useSubAgentStore((s) => s.refresh)

  useEffect(() => {
    void refresh()
  }, [refresh])

  return (
    <section className={styles.card}>
      <div className={styles.cardHead}>
        <span className={styles.emoji}>⇄</span>
        <span className={styles.cardTitle}>子智能体</span>
        {agents.length > 0 && <span className={styles.badge}>{agents.length}</span>}
        <button className={styles.refresh} onClick={() => void refresh()} title="刷新子智能体">
          ⟳
        </button>
      </div>
      <div className={styles.cardBody}>
        {loading && <div className={styles.empty}>加载中…</div>}
        {!loading && error && <div className={styles.empty}>暂无可用的子智能体。</div>}
        {!loading && !error && agents.length === 0 && <div className={styles.empty}>暂无子智能体。</div>}
        {!loading &&
          agents.map((a, i) => (
            <div className={styles.sub} key={a.id ?? String(i)}>
              <div className={styles.subHead}>
                <span className={styles.avatar}>{nameOf(a).slice(0, 1).toUpperCase()}</span>
                <div className={styles.subMeta}>
                  <div className={styles.subName}>{nameOf(a)}</div>
                  <div className={styles.subScope}>子智能体</div>
                </div>
              </div>
              {modelOf(a) && (
                <div className={styles.subTags}>
                  <span className={styles.tag}>{modelOf(a)}</span>
                </div>
              )}
              <div className={styles.subDesc}>{descOf(a)}</div>
            </div>
          ))}
      </div>
    </section>
  )
}
