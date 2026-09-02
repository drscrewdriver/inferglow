import { useMemo } from 'react'
import { useChatStore } from '../stores/chatStore'
import { SlotOutlet } from '../framework'
import styles from './details.module.css'

export interface DetailsPanelProps {
  /** Whether the panel is expanded in the frame. */
  open?: boolean
  onToggle?: () => void
}

/**
 * Right-side details pane replacing the former dock: shows the current tool
 * call and the context-window usage. Collapsible; the App auto-closes it when
 * the active session changes (Task 10).
 */
export function DetailsPanel({ open, onToggle }: DetailsPanelProps) {
  const messages = useChatStore((s) => s.messages)
  const streaming = useChatStore((s) => s.streaming)

  const tools = useMemo(() => messages.filter((m) => m.role === 'tool'), [messages])
  const running = useMemo(() => tools.some((t) => (t.toolStatus ?? 'ok') === 'run'), [tools])
  const lastTool = tools[tools.length - 1]

  return (
    <div className={styles.root}>
      <div className={styles.head}>
        <span className={styles.title}>详情</span>
        <button className={styles.toggle} onClick={onToggle} title={open ? '折叠详情面板' : '展开详情面板'}>
          {open ? '›' : '‹'}
        </button>
      </div>

      <div className={styles.body}>
        {/* Context window ring */}
        <section className={styles.card}>
          <div className={styles.cardHead}>
            <span>◎</span>
            <span className={styles.cardTitle}>上下文窗口</span>
            <span className={open ? styles.chev : styles.chev}>▾</span>
          </div>
          <div className={styles.cardBody}>
            <div className={styles.ringWrap}>
              <div className={styles.ring}>
                <svg width="96" height="96" viewBox="0 0 96 96">
                  <circle cx="48" cy="48" r="38" fill="none" stroke="var(--igw-panel-2)" strokeWidth="8" />
                  <circle
                    cx="48"
                    cy="48"
                    r="38"
                    fill="none"
                    stroke="var(--igw-accent)"
                    strokeWidth="8"
                    strokeLinecap="round"
                    strokeDasharray="238.8"
                    strokeDashoffset="238.8"
                  />
                </svg>
                <div className={styles.ringCenter}>
                  <span className={styles.pct}>0%</span>
                  <span className={styles.lbl}>已用</span>
                </div>
              </div>
              <div className={styles.legend}>
                <div className={styles.legendRow}>
                  <span>模型</span>
                  <b>deepseek-chat</b>
                </div>
                <div className={styles.legendRow}>
                  <span>上限</span>
                  <b>128k</b>
                </div>
                <div className={styles.legendRow}>
                  <span>本轮工具</span>
                  <b>{tools.length}</b>
                </div>
              </div>
            </div>
          </div>
        </section>

        {/* Current tool call */}
        <section className={styles.card}>
          <div className={styles.cardHead}>
            <span>⚙</span>
            <span className={styles.cardTitle}>工具调用</span>
            {running && <span className={styles.live}>运行中</span>}
          </div>
          <div className={styles.cardBody}>
            {lastTool ? (
              <div className={styles.toolRow}>
                <span className={`${styles.dot} ${running ? styles.dotRun : styles.dotOk}`} />
                <span className={styles.toolRowName}>{lastTool.toolName ?? 'tool'}</span>
                <span className={styles.toolRowStatus}>
                  {running && streaming ? '进行中' : lastTool.toolStatus === 'error' ? '失败' : '完成'}
                </span>
              </div>
            ) : (
              <div className={styles.empty}>暂无可用的工具调用。</div>
            )}
            {tools.length > 1 && (
              <div className={styles.history}>
                {tools
                  .slice(0, tools.length - 1)
                  .reverse()
                  .map((t) => (
                    <div className={styles.toolRow} key={t.id}>
                      <span className={`${styles.dot} ${t.toolStatus === 'error' ? styles.dotErr : styles.dotOk}`} />
                      <span className={styles.toolRowName}>{t.toolName ?? 'tool'}</span>
                    </div>
                  ))}
              </div>
            )}
          </div>
        </section>

        {/* Session footnote */}
        <section className={styles.card}>
          <div className={styles.cardHead}>
            <span>↻</span>
            <span className={styles.cardTitle}>会话切换</span>
          </div>
          <div className={styles.cardBody}>
            <div className={styles.note}>切换会话时，此面板会自动收起。</div>
          </div>
        </section>

        {/* Plugin slots: background task list (traffic job list) etc. */}
        <SlotOutlet name="details.panel.items" />
      </div>
    </div>
  )
}