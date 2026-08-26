import { useEffect, useState } from 'react'
import styles from './approval.module.css'
import { createApprovalStore } from './approvalStore'
import type { ApprovalRecord, RiskLevel } from '../api'

const useApprovalStore = createApprovalStore()

const RISK_LABEL: Record<RiskLevel, string> = {
  none: '无风险',
  low: '低风险',
  medium: '中风险',
  high: '高风险',
}

const RISK_CLASS: Record<RiskLevel, string> = {
  none: styles.riskNone,
  low: styles.riskLow,
  medium: styles.riskMedium,
  high: styles.riskHigh,
}

function ApprovalCard({ rec, onDecide }: { rec: ApprovalRecord; onDecide: (approve: boolean, justification: string) => void }) {
  const [justification, setJustification] = useState('')
  const risk = rec.request?.risk ?? 'low'

  return (
    <div className={styles.card} data-approval-card>
      <div className={styles.head}>
        <span className={`${styles.risk} ${RISK_CLASS[risk]}`}>{RISK_LABEL[risk]}</span>
        <span className={styles.subject}>{rec.request?.subject ?? rec.id}</span>
      </div>
      <div className={styles.capability}>{rec.request?.capability ?? 'approval'}</div>
      {rec.request?.source && <div className={styles.source}>来源：{rec.request.source}</div>}

      <input
        className={styles.justify}
        placeholder="备注（可选）…"
        value={justification}
        onChange={(e) => setJustification(e.target.value)}
      />

      <div className={styles.actions}>
        <button className={styles.deny} onClick={() => onDecide(false, justification)}>
          ✕ 拒绝
        </button>
        <button className={styles.allow} onClick={() => onDecide(true, justification)}>
          ✓ 允许
        </button>
      </div>
    </div>
  )
}

export function ApprovalPanel() {
  const store = useApprovalStore
  const pending = store((s) => s.pending)
  const loading = store((s) => s.loading)
  const error = store((s) => s.error)
  const fetch = store((s) => s.fetch)
  const decide = store((s) => s.decide)

  useEffect(() => {
    void fetch()
  }, [fetch])

  return (
    <section className={styles.root} data-approval-panel>
      <div className={styles.panelHead}>
        <span>🛡 审批</span>
        <button className={styles.refresh} onClick={() => void fetch()} title="刷新">
          ↻
        </button>
      </div>

      {loading && <div className={styles.empty}>加载中…</div>}
      {!loading && error && <div className={styles.empty} style={{ color: 'var(--igw-err)' }}>{error}</div>}
      {!loading && !error && pending.length === 0 && <div className={styles.empty}>无待审批项</div>}

      {pending.map((rec) => (
        <ApprovalCard
          key={rec.id || rec.request?.request_id}
          rec={rec}
          onDecide={(approve, justification) => {
            void decide({ recordId: rec.id, approve, justification })
          }}
        />
      ))}
    </section>
  )
}