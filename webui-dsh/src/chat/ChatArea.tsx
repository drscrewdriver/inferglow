/**
 * Chat area — main message display with:
 * - Empty state / welcome screen
 * - Message list with auto-scroll
 * - Input area at bottom
 * - Stats line (token usage simulation)
 */

import { useRef, useEffect, useState } from 'react'
import type { Message } from '../store.ts'
import { MessageItem } from './MessageItem.tsx'
import { ChatInput } from './ChatInput.tsx'
import { store, subscribe } from '../store.ts'
import { TracePanel, ContextPanel } from '../app/layout/ConvPanels.tsx'
import { ConversationWidthHandles } from './ConversationWidthHandles.tsx'

/** Format elapsed milliseconds into a human-readable duration label. */
function formatRunDuration(ms: number): string {
  const total = Math.max(0, Math.floor(ms / 1000))
  const minutes = Math.floor(total / 60)
  const seconds = total % 60
  return minutes > 0
    ? `${minutes}:${String(seconds).padStart(2, '0')}`
    : `${seconds}秒`
}

/**
 * TurnStatus — displays "Deep diving..." during a running turn,
 * with a shimmer gradient animation and an optional running-clock
 * that appears after 15 seconds.
 */
function TurnStatus({ streamingStartTime }: {
  streamingStartTime: number | null
}) {
  const mountedAt = useRef(Date.now())
  const anchor = streamingStartTime ?? mountedAt.current
  const [elapsedMs, setElapsedMs] = useState(() => Math.max(0, Date.now() - anchor))

  useEffect(() => {
    const tick = () => setElapsedMs(Math.max(0, Date.now() - anchor))
    tick()
    const id = setInterval(tick, 1000)
    return () => clearInterval(id)
  }, [anchor])

  const showClock = elapsedMs >= 15_000

  return (
    <div className="dsh-turn-status" role="status" aria-live="polite">
      Deep diving...
      {showClock && (
        <span className="dsh-turn-status-clock" aria-hidden>
          {formatRunDuration(elapsedMs)}
        </span>
      )}
    </div>
  )
}

interface ChatAreaProps {
  activeSessionId: string | null
  convTab?: string
}

export function ChatArea({ activeSessionId, convTab = '对话' }: ChatAreaProps) {
  const scrollRef = useRef<HTMLDivElement>(null)
  const [messages, setMessages] = useState<Message[]>([])
  const [isStreaming, setIsStreaming] = useState(false)
  const [streamingStartTime, setStreamingStartTime] = useState<number | null>(null)
  const [autoScroll, setAutoScroll] = useState(store.settings.autoScroll)
  const [composerTouched, setComposerTouched] = useState(store.composerTouched)

  /* Subscribe to store */
  useEffect(() => {
    const unsub = subscribe(() => {
      const session = store.sessions.find(s => s.id === activeSessionId)
      setMessages(session?.messages ?? [])
      setIsStreaming(store.isStreaming)
      setStreamingStartTime(store.streamingStartTime)
      setAutoScroll(store.settings.autoScroll)
      setComposerTouched(store.composerTouched)
    })
    return unsub
  }, [activeSessionId])

  /* Auto-scroll */
  useEffect(() => {
    if (autoScroll && scrollRef.current && !isStreaming) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight
    }
  }, [messages.length, autoScroll, isStreaming])

  /* Show the centered hero composer when:
   * - no session is selected, OR
   * - the session is still empty and the user hasn't begun typing yet.
   * Once the user starts typing (or messages exist), the input sinks to the bottom. */
  if (!activeSessionId || (messages.length === 0 && !composerTouched)) {
    return (
      <div className="dsh-composer-seat">
        <div className="dsh-composer-stack">
          <div className="dsh-hero-glow" aria-hidden="true"><svg viewBox="0 0 1051 468" fill="none"><defs><filter id="hero-glow-filter" x="0" y="0" width="1051" height="468" filterUnits="userSpaceOnUse" colorInterpolationFilters="sRGB"><feFlood floodOpacity="0" result="BackgroundImageFix"/><feBlend mode="normal" in="SourceGraphic" in2="BackgroundImageFix" result="shape"/><feGaussianBlur stdDeviation="50" result="effect1_foregroundBlur"/></filter></defs><g filter="url(#hero-glow-filter)"><ellipse cx="525.5" cy="234" rx="425.5" ry="134" fill="#6187D8" fillOpacity="0.08"/></g></svg></div>
          <div className="dsh-hero-headline">
            <span className="dsh-hero-fish"><svg width="34" height="25" viewBox="0 0 16 16" fill="none" aria-hidden="true"><path d="M8.00003 0.3237C3.76075 0.3237 0.32373 3.76072 0.32373 8C0.32373 9.17603 0.589121 10.2922 1.0632 11.2901L1.35291 11.8989L2.5705 11.3205L2.28079 10.7117C1.89079 9.89074 1.67301 8.97167 1.67301 8C1.67301 4.50546 4.50549 1.67298 8.00003 1.67298C11.4946 1.67298 14.3271 4.50546 14.3271 8C14.3271 11.4945 11.4946 14.327 8.00003 14.327C7.28473 14.327 6.76077 14.277 6.29621 14.1487C5.83857 14.0224 5.40441 13.8109 4.88514 13.4488C4.12569 12.919 3.03778 12.7316 2.141 13.2978L2.12682 13.307L2.11264 13.3171L1.34886 13.854L1.79659 15.188L2.86122 14.4384C3.19068 14.2305 3.68325 14.2542 4.11326 14.5539C4.72789 14.9826 5.30042 15.2724 5.93762 15.4484C6.56803 15.6224 7.22776 15.6763 8.00003 15.6763C12.2393 15.6763 15.6763 12.2393 15.6763 8C15.6763 3.76072 12.2393 0.3237 8.00003 0.3237ZM7.32033 4.82535V7.32536H4.82538V8.67464H7.32033V11.1747H8.6696V8.67464H11.1747V7.32536H8.6696V4.82535H7.32033Z" fill="currentColor"/></svg></span>
            <h1 className="dsh-hero-title">探索未至之境</h1>
            <span className="dsh-hero-badge">预览版</span>
          </div>
          <div className="dsh-hero-workspace-row">
            <div className="dsh-hero-toolbar">
              <button
                type="button"
                className="dsh-hero-workspace-btn"
                aria-label="选择工作区"
                aria-haspopup="menu"
                aria-expanded="false"
              >
                <svg width="14" height="14" viewBox="0 0 16 16" fill="none">
                  <path d="M2.3 3.5a1.2 1.2 0 011.2-1.2h3l1.1 1.4h5.2a1.2 1.2 0 011.2 1.2v1h.2a1.2 1.2 0 011 1.7l-1.1 3.2a1.2 1.2 0 01-1.1.8H3.2a1.2 1.2 0 01-1.2-1.5l.3-4.6z" stroke="currentColor" strokeWidth="1.2" strokeLinejoin="round"/>
                  <path opacity="0.4" d="M2.3 5.6h11.4l-1 3a.6.6 0 01-.6.4H3.9l-.7-3.4z" fill="currentColor"/>
                </svg>
                <span className="dsh-hero-workspace-label">rewrite-agently</span>
                <svg width="12" height="12" viewBox="0 0 12 12" fill="none">
                  <path d="M3 4.5l3 3 3-3" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round"/>
                </svg>
              </button>
              <button
                type="button"
                className="dsh-hero-agent-btn"
                aria-haspopup="menu"
                aria-expanded="false"
                title="即将开始的这个会话所用的 Agent 预设"
              >
                <svg width="14" height="14" viewBox="0 0 16 16" fill="none">
                  <circle cx="8" cy="3.2" r="1.5" stroke="currentColor" strokeWidth="1.2"/>
                  <circle cx="3.4" cy="11.4" r="1.5" stroke="currentColor" strokeWidth="1.2"/>
                  <circle cx="12.6" cy="11.4" r="1.5" stroke="currentColor" strokeWidth="1.2"/>
                  <path d="M5.1 9.2c.8-.6 1.8-1 2.9-1s2.1.4 2.9 1" stroke="currentColor" strokeWidth="1.2" strokeLinecap="round"/>
                </svg>
                <span className="dsh-hero-agent-label">标准模式 (Windows)</span>
                <svg width="12" height="12" viewBox="0 0 12 12" fill="none">
                  <path d="M3 4.5l3 3 3-3" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round"/>
                </svg>
              </button>
            </div>
            <ChatInput sessionId={activeSessionId} />
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="dsh-chat-area">
      {convTab === '轨迹' ? (
        <TracePanel />
      ) : convTab === '上下文' ? (
        <ContextPanel />
      ) : (
        <>
          {/* Message list — content width wrapped in a draggable body band */}
          <ConversationWidthHandles>
            <div className="dsh-chat-messages" ref={scrollRef}>
              {messages.map((msg: Message) => (
                <MessageItem key={msg.id} message={msg} />
              ))}
              {isStreaming && (
                <div className="dsh-message" style={{ paddingTop: 4 }}>
                  <TurnStatus streamingStartTime={streamingStartTime} />
                </div>
              )}
            </div>
          </ConversationWidthHandles>

          {/* Input */}
          <div className="dsh-chat-input-area">
            <ChatInput sessionId={activeSessionId} placeholder="给智能体发消息" />
          </div>

          {/* Composer dock stats */}
          <div className="dsh-composer-dock">
            <span>1 轮 · 3 步</span><span className="dsh-dock-sep">|</span>
            <span>LLM 4.2s · 工具调用 7m29s</span><span className="dsh-dock-sep">|</span>
            <span>首 token 平均 0.7s · 169 tok/s</span><span className="dsh-dock-sep">|</span>
            <span>缓存命中 67%</span><span className="dsh-dock-sep">|</span>
            <span>输入 49K tok · 输出 337 tok</span>
          </div>
        </>
      )}
    </div>
  )
}
