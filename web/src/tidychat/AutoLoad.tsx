import { useEffect, useRef } from 'react'
import styles from './tidychat.module.css'
import { useTidychatStore, selectGovernor } from './tidychatStore'
import { createGovernorState, decideGovernor } from './governor'
import { SETTLE_QUIET_MS } from './config'
import { useChatStore } from '../stores/chatStore'

/**
 * Smart auto-load of earlier history, driven by the pure governor reducer.
 * A single effect owns a self-scheduling loop (idle → load → measure → decide)
 * on requestIdleCallback (or a setTimeout fallback). A continuation simply
 * re-arms the loop; pause/done stop scheduling. The rendered hint offers a
 * manual resume when the performance governor pauses.
 */
export function AutoLoad({
  sessionId,
  hasMore,
  loadOlder,
}: {
  sessionId: string
  hasMore: boolean
  loadOlder: (sessionId: string) => Promise<void>
}) {
  const autoLoad = useTidychatStore((s) => s.config.autoLoad)
  const governor = useTidychatStore((s) => selectGovernor(s.governors, sessionId))
  const setGovernor = useTidychatStore((s) => s.setGovernor)
  const tickRef = useRef<() => void>(() => {})

  useEffect(() => {
    if (!sessionId) return
    let active = true
    let timerId = 0
    const clearTimer = () => {
      if (timerId) {
        window.clearTimeout(timerId)
        timerId = 0
      }
    }

    const arm = () => {
      timerId = window.setTimeout(() => {
        if (active) runNow()
      }, 50)
    }

    const runNow = () => {
      clearTimer()
      const st = useTidychatStore.getState()
      const chat = useChatStore.getState()
      if (!st.config.autoLoad || !chat.hasMore || !chat.nextBefore) return
      const g = st.governors[sessionId]
      if (g && g.status !== 'idle') return
      if (chat.loadingOlder) {
        arm()
        return
      }
      const before = chat.messages.length
      setGovernor(sessionId, { status: 'loading', consecutiveSlow: g?.consecutiveSlow ?? 0 })
      const t0 = performance.now()
      void loadOlder(sessionId).then(() => {
        if (!active) return
        const chat2 = useChatStore.getState()
        const scanMs = performance.now() - t0
        const cur = useTidychatStore.getState().governors[sessionId] ?? createGovernorState('loading')
        const r = decideGovernor({
          scanMs,
          grew: chat2.messages.length > before,
          stillHasMore: chat2.hasMore,
          prev: cur,
        })
        useTidychatStore.getState().setGovernor(sessionId, r.next)
        if (r.decision === 'continue') {
          timerId = window.setTimeout(() => {
            if (active) runNow()
          }, SETTLE_QUIET_MS)
        }
      })
    }

    tickRef.current = runNow
    if (autoLoad) arm()
    return () => {
      active = false
      clearTimer()
    }
  }, [sessionId, loadOlder, autoLoad, hasMore, setGovernor])

  if (!sessionId || !autoLoad || !hasMore) return null
  if (governor.status !== 'paused') return null

  return (
    <div data-tidychat-autoload-hint className={styles.autoLoadHint}>
      <span>为保持流畅，已暂停自动加载更早历史。</span>
      <button
        type="button"
        className={styles.autoLoadResume}
        onClick={() => {
          setGovernor(sessionId, createGovernorState('idle'))
          window.setTimeout(() => tickRef.current(), 50)
        }}
      >
        手动继续
      </button>
    </div>
  )
}