/**
 * MessageItem — full DSH-style message rendering:
 * - User messages: right-aligned, with timestamp and copy
 * - Assistant messages: left-aligned, with markdown, code blocks, reasoning
 * - Tool calls: collapsible tool invocation display
 * - Error states
 * - Streaming indicator
 */

import { memo, useCallback, useRef, useState } from 'react'
import type { Message, ToolCall } from '../store.ts'
import { MarkdownRenderer } from './MarkdownRenderer.tsx'

interface MessageItemProps {
  message: Message
}

/** Memoized: streaming flushes touch one message object; siblings must not
 * re-render their markdown on every flush. */
export const MessageItem = memo(function MessageItem({ message }: MessageItemProps) {
  const isUser = message.role === 'user'
  const isAssistant = message.role === 'assistant'
  const [expandedTools, setExpandedTools] = useState<Set<string>>(new Set())
  const [copied, setCopied] = useState(false)
  const copyTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const copyEpoch = useRef(0)

  function toggleTool(toolId: string) {
    setExpandedTools(prev => {
      const next = new Set(prev)
      if (next.has(toolId)) {
        next.delete(toolId)
      } else {
        next.add(toolId)
      }
      return next
    })
  }

  const onCopy = useCallback(() => {
    if (copied) return
    const epoch = copyEpoch.current
    void navigator.clipboard.writeText(message.content).then(() => {
      if (epoch !== copyEpoch.current) return
      setCopied(true)
      copyTimer.current = window.setTimeout(() => {
        copyTimer.current = null
        setCopied(false)
      }, 1000)
    })
  }, [copied, message.content])

  return (
    <div className={`dsh-message ${isUser ? 'dsh-message-user' : 'dsh-message-assistant'}`} data-time-hover-root>
      {/* Header: role + time */}
      <div className="dsh-message-header">
        <span className="dsh-message-role">
          {isUser ? '你' : 'AI'}
          {message.status === 'streaming' && (
            <span className="dsh-message-streaming-dot" />
          )}
        </span>
      </div>

      {/* Message action bar (hover to show) */}
      {message.content && (
        <div className={`dsh-message-action-bar ${isAssistant ? 'dsh-actionbar-reverse' : ''}`}>
          <span className="dsh-message-action-clock" title={new Date(message.timestamp).toLocaleString('zh-CN')}>
            {formatTime(message.timestamp)}
          </span>
          <button
            className="dsh-message-action-btn"
            aria-label={copied ? '已复制' : '复制'}
            onClick={onCopy}
            title={copied ? '已复制' : '复制内容'}
          >
            {copied ? (
              <svg width="14" height="14" viewBox="0 0 14 14" fill="none">
                <path d="M3 7.5l3 3 5-6" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"/>
              </svg>
            ) : (
              <svg width="14" height="14" viewBox="0 0 14 14" fill="none">
                <rect x="5" y="5" width="8" height="8" rx="1.5" stroke="currentColor" strokeWidth="1.2"/>
                <path d="M9 5V3.5A1.5 1.5 0 007.5 2h-4A1.5 1.5 0 002 3.5v4A1.5 1.5 0 003.5 9H5" stroke="currentColor" strokeWidth="1.2"/>
              </svg>
            )}
          </button>
        </div>
      )}

      {/* Content */}
      <div className={`dsh-message-body ${isUser ? 'dsh-message-body-user' : 'dsh-message-body-assistant'}`}>
        {/* Reasoning block (if any) */}
        {isAssistant && message.reasoning && (
          <details className="dsh-message-reasoning">
            <summary className="dsh-message-reasoning-toggle">思考过程</summary>
            <div className="dsh-message-reasoning-content">
              <MarkdownRenderer content={message.reasoning} />
            </div>
          </details>
        )}

        {/* Main content */}
        {message.content && (
          <div className="dsh-message-text">
            {isAssistant ? (
              <MarkdownRenderer content={message.content} />
            ) : (
              <p>{message.content}</p>
            )}
          </div>
        )}

        {/* Tool calls */}
        {message.toolCalls && message.toolCalls.length > 0 && (
          <div className="dsh-message-tools">
            {message.toolCalls.map(tc => (
              <ToolCallCard
                key={tc.id}
                toolCall={tc}
                expanded={expandedTools.has(tc.id)}
                onToggle={() => toggleTool(tc.id)}
              />
            ))}
          </div>
        )}

        {/* Error */}
        {message.status === 'error' && (
          <div className="dsh-message-error">⚠ 生成失败，请重试</div>
        )}
      </div>
    </div>
  )
})

/* ── Tool call card ── */
function ToolCallCard({ toolCall, expanded, onToggle }: {
  toolCall: ToolCall
  expanded: boolean
  onToggle: () => void
}) {
  return (
    <div className="dsh-tool-call">
      <button className="dsh-tool-call-header" onClick={onToggle}>
        <span className="dsh-tool-call-icon">
          <svg width="14" height="14" viewBox="0 0 14 14" fill="none">
            <path d="M7 2v10M2 7h10" stroke="currentColor" strokeWidth="1.2" strokeLinecap="round"/>
          </svg>
        </span>
        <span className="dsh-tool-call-name">{toolCall.name}</span>
        <span className="dsh-tool-call-toggle">{expanded ? '▲' : '▼'}</span>
      </button>
      {expanded && (
        <div className="dsh-tool-call-body">
          <div className="dsh-tool-call-args">
            <strong>参数：</strong>
            <pre>{JSON.stringify(toolCall.args, null, 2)}</pre>
          </div>
          {toolCall.output && (
            <div className="dsh-tool-call-output">
              <strong>输出：</strong>
              <pre>{toolCall.output}</pre>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

function formatTime(ts: number): string {
  return new Date(ts).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
}
