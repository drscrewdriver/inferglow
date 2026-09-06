// ContextView — Phase 4 context tab root. Composes StatsBoard +
// CurrentComposition + TrendChart + ContextBrowser + EventList + FileCard
// from a Projection derived client-side off the session's message stream.

import { useMemo, useState } from 'react'
import { useChatStore } from '../stores/chatStore'
import type { ProjectionRequest } from './types'
import { aggregateByTurn, deltaOf, fold } from './fold'
import { ContextBrowser } from './ContextBrowser'
import { CurrentComposition } from './CurrentComposition'
import { EventList } from './EventList'
import { FileCard } from './FileCard'
import { StatsBoard } from './StatsBoard'
import { RequestDetail, TrendChart } from './TrendChart'
import { EVENT_ICONS } from './palette'
import styles from './context.module.css'

export function ContextView({ sessionId }: { sessionId: string }) {
  const messages = useChatStore((s) => s.messages)

  // Re-fold the message stream into a projection (pure derivation, memoized).
  const projection = useMemo(
    () => fold(messages, { model: 'deepseek-chat', provider: 'harness', contextWindow: 131072 }),
    [messages],
  )

  const [granularity, setGranularity] = useState<'step' | 'turn'>('step')
  const [trendMode, setTrendMode] = useState<'total' | 'delta'>('total')
  const [hoverKey, setHoverKey] = useState<string | null>(null)
  const [selected, setSelected] = useState<ProjectionRequest | null>(null)

  // Display list depends on granularity/mode; markers map seq → compaction/prune.
  const displayRequests = useMemo(() => {
    let list = projection.requests
    if (granularity === 'turn') list = aggregateByTurn(list)
    if (trendMode === 'delta') list = deltaOf(list)
    return list
  }, [projection.requests, granularity, trendMode])

  const activeReq = selected ?? displayRequests[displayRequests.length - 1] ?? null
  const markerSeq = new Set(projection.events.filter((e) => e.kind === 'compaction' || e.kind === 'prune').map((e) => e.seq))
  void sessionId

  const linkTools = `read/write/search/image/dir · 每次工具调用并入工具结果桶`

  return (
    <div className={styles.root} data-conversation-scroll data-testid="ctx-view">
      <div className={styles.cols + ' ' + styles.colsHead}>
        <StatsBoard
          requests={projection.requests}
          events={projection.events}
          current={projection.current}
          toolCalls={projection.toolCalls.size}
          images={projection.images}
        />
      </div>

      <div className={styles.cols}>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
          <CurrentComposition
            comp={projection.current}
            subtitle={`${projection.model ?? ''}${projection.provider ? ' · ' + projection.provider : ''}`}
            contextWindow={projection.contextWindow}
            hoverKey={hoverKey}
            onHoverKey={setHoverKey}
          />

          <div className={styles.card}>
            <div className={styles.cardTitle}>
              <span className={styles.cardTitleText}>上下文趋势</span>
              <span className={styles.cardSub}>堆叠 ≈ 六类 token 占比</span>
              <div className={styles.trendCtl}>
                <div className={styles.gran}>
                  <button className={styles.granBtn + (granularity === 'step' ? ' ' + styles.granOn : '')} onClick={() => setGranularity('step')}>步</button>
                  <button className={styles.granBtn + (granularity === 'turn' ? ' ' + styles.granOn : '')} onClick={() => setGranularity('turn')}>轮</button>
                </div>
                <div className={styles.gran}>
                  <button className={styles.granBtn + (trendMode === 'total' ? ' ' + styles.granOn : '')} onClick={() => setTrendMode('total')}>总量</button>
                  <button className={styles.granBtn + (trendMode === 'delta' ? ' ' + styles.granOn : '')} onClick={() => setTrendMode('delta')}>增量</button>
                </div>
              </div>
            </div>
            <TrendChart requests={displayRequests} events={projection.events} mode={trendMode} onSelect={setSelected} />
            <RequestDetail request={activeReq} prev={trendMode === 'delta' ? (displayRequests[displayRequests.indexOf(activeReq) - 1] ?? null) : undefined} marker={activeReq && markerSeq.has(activeReq.seq) ? projection.events.find((e) => e.seq === activeReq.seq) : null} />
          </div>
        </div>

        <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
          <ContextBrowser requests={projection.requests} hoverKey={hoverKey} onHoverKey={setHoverKey} />
        </div>
      </div>

      <div className={styles.cols}>
        <div className={styles.card}>
          <div className={styles.cardTitle}>
            <span className={styles.cardTitleText}>上下文事件</span>
            <span className={styles.cardSub}>{linkTools}</span>
          </div>
          <EventList events={projection.events} />
        </div>
        <FileCard files={projection.files} scope={`T${activeReq?.turn ?? 0}.S${activeReq?.step ?? 0}`} onLocate={(op) => {
          const target = projection.requests.find((r) => r.seq >= op.seq)
          if (target) setSelected(target)
        }} />
      </div>

      {projection.events.length > 0 && <div className={styles.foot}>{EVENT_ICONS.compaction} 压缩 / 剪枝事件以事件栏标记</div>}
    </div>
  )
}