/**
 * Chat input — DSH-style layered composer card with:
 * - Multi-line textarea with auto-resize
 * - Send on Enter (Shift+Enter for newline)
 * - Stop generation button during streaming
 * - Toolbar: [+] command, access mode, freeze, status, model, send
 * - Placeholder "描述你想要构建的内容"
 */

import { useState, useRef, useEffect, type FormEvent } from 'react'
import { IconStop } from '../components/Icons.tsx'
import { store, subscribe, type Message } from '../store.ts'
import { api, ensureSession, getActiveAgentId, agentName, recordUsage } from '../bridge/inferglow.ts'

interface ChatInputProps {
  sessionId: string | null
  placeholder?: string
}

function genMsgId(): string {
  return `msg-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
}

export function ChatInput({ sessionId, placeholder }: ChatInputProps) {
  const [value, setValue] = useState('')
  const [modelLabel, setModelLabel] = useState('')
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  /* Model chip reflects the resolved agent (settings.model / first agent). */
  useEffect(() => {
    const sync = () => {
      const id = store.settings.model || getActiveAgentId()
      setModelLabel(id ? agentName(id) : '未配置 Agent')
    }
    sync()
    return subscribe(sync)
  }, [])

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    doSend()
  }

  async function doSend() {
    const text = value.trim()
    if (!text || store.isStreaming) return

    // Persist a session for the conversation first (backend; local fallback).
    let sid = sessionId
    if (!sid) {
      sid = await ensureSession(text)
      if (!sid) return
    }

    const userMsg = {
      id: genMsgId(),
      role: 'user' as const,
      content: text,
      status: 'sent' as const,
      timestamp: Date.now(),
    }
    const assistantMsg = {
      id: genMsgId(),
      role: 'assistant' as const,
      content: '',
      status: 'streaming' as const,
      timestamp: Date.now(),
    }

    store.addMessage(sid, userMsg)
    store.addMessage(sid, assistantMsg)
    store.setStreaming(true, assistantMsg.id)
    setValue('')

    if (textareaRef.current) {
      textareaRef.current.style.height = 'auto'
    }

    // Prefer the agent the session is bound to (mirrors the host webui),
    // falling back to the settings/first-agent resolution.
    const agentId = store.sessions.find(s => s.id === sid)?.agentId || getActiveAgentId()
    if (!agentId) {
      store.updateMessage(sid, assistantMsg.id, {
        status: 'error' as const,
        content: '后端未返回可用 Agent — 请检查 InferGlow Server 是否已启动。',
      })
      store.setStreaming(false)
      return
    }

    const patchToolEnd = (toolName: string, err?: string) => {
      const session = store.sessions.find(s => s.id === sid)
      const msg = session?.messages.find(m => m.id === assistantMsg.id)
      const calls = [...(msg?.toolCalls ?? [])]
      const last = calls.length > 0 ? calls[calls.length - 1] : undefined
      if (last && last.name === toolName && last.output === undefined) {
        calls[calls.length - 1] = { ...last, output: err ? `error: ${err}` : 'ok' }
        store.updateMessage(sid, assistantMsg.id, { toolCalls: calls })
      }
    }

    // Real-stream buffering: server deltas arrive per token; flush into the
    // store at 80ms intervals so React re-renders stay cheap. run_end carries
    // the full reply and is authoritative (reconciles any dropped chunk).
    let streamText = ''
    let reasoningText = ''
    let flushTimer: number | null = null
    const flush = () => {
      flushTimer = null
      const updates: Partial<Message> = { status: 'streaming' }
      if (streamText) updates.content = streamText
      if (reasoningText) updates.reasoning = reasoningText
      store.updateMessage(sid, assistantMsg.id, updates)
    }
    const cancelFlush = () => {
      if (flushTimer !== null) {
        window.clearTimeout(flushTimer)
        flushTimer = null
      }
    }

    try {
      await api.sendChat({
        agentId,
        sessionId: sid,
        message: text,
        handlers: {
          onToolStart: toolName => {
            store.addToolCall(sid, assistantMsg.id, {
              id: genMsgId(),
              name: toolName,
              args: {},
            })
          },
          onToolEnd: patchToolEnd,
          onDelta: t => {
            streamText += t
            if (flushTimer === null) flushTimer = window.setTimeout(flush, 80)
          },
          onReasoning: t => {
            reasoningText += t
            if (flushTimer === null) flushTimer = window.setTimeout(flush, 80)
          },
          onUsage: recordUsage,
          onDone: reply => {
            cancelFlush()
            store.updateMessage(sid, assistantMsg.id, {
              content: reply || streamText || '(空回复)',
              status: 'sent' as const,
              reasoning: reasoningText || undefined,
            })
            store.setStreaming(false)
          },
          onError: message => {
            cancelFlush()
            store.updateMessage(sid, assistantMsg.id, {
              status: 'error' as const,
              content: message,
            })
            store.setStreaming(false)
          },
        },
      })
    } catch (err) {
      cancelFlush()
      const aborted = (err as Error)?.name === 'AbortError'
      store.updateMessage(sid, assistantMsg.id, {
        status: aborted ? 'sent' : 'error',
        content: aborted ? (streamText || '(已停止)') : String((err as Error)?.message ?? err),
      })
      store.setStreaming(false)
    }
  }

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      void doSend()
    }
  }

  function handleInput(e: React.ChangeEvent<HTMLTextAreaElement>) {
    setValue(e.target.value)
    // The user has begun typing — sink the composer to the bottom.
    store.setComposerTouched(true)
    // Auto-resize
    const el = e.target
    el.style.height = 'auto'
    el.style.height = Math.min(el.scrollHeight, 200) + 'px'
  }

  function handleStop() {
    api.cancel()
    store.setStreaming(false)
  }

  const hasContent = value.trim().length > 0
  const isStreaming = store.isStreaming

  return (
    <div className="dsh-composer-card">
      {/* Layered card: backdrop / mirror / grow / core form */}
      <div className="dsh-composer-card-backdrop" aria-hidden="true" />
      <div className="dsh-composer-card-mirror" aria-hidden="true" />
      <div className="dsh-composer-card-grow" aria-hidden="true" />

      <form className="dsh-composer-form" onSubmit={handleSubmit}>
        <textarea
          ref={textareaRef}
          className="dsh-composer-textarea"
          value={value}
          onChange={handleInput}
          onFocus={() => store.setComposerTouched(true)}
          onKeyDown={handleKeyDown}
          placeholder={placeholder ?? '描述你想要构建的内容'}
          disabled={isStreaming}
          rows={1}
        />
        <div className="dsh-composer-row">
          <div className="dsh-composer-tools">
            <button className="dsh-composer-add" type="button" title="命令" aria-label="命令">
              <svg width="14" height="14" viewBox="0 0 16 16" fill="none">
                <path d="M8.64453 1.5V7.34961H14.5V8.65039H8.64453V14.5H7.34473V8.65039H1.5V7.34961H7.34473V1.5H8.64453Z" fill="currentColor"/>
              </svg>
            </button>
            <div className="dsh-composer-modes">
              <button className="dsh-composer-mode" type="button" title="访问模式，当前：Workspace Write">
                <span className="dsh-composer-mode-icon">
                  <svg width="14" height="14" viewBox="0 0 16 16" fill="none">
                    <path d="M4 9l3 3 5-6" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round"/>
                  </svg>
                </span>
                <span className="dsh-composer-mode-label">Workspace Write</span>
                <svg width="12" height="12" viewBox="0 0 12 12" fill="none">
                  <path d="M3 4.5l3 3 3-3" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round"/>
                </svg>
              </button>
            </div>
          </div>
          <div className="dsh-composer-trailing">
            <button className="dsh-composer-freeze" type="button" title="冻结会话" aria-label="冻结会话">
              <span className="dsh-composer-freeze-label">冻结会话</span>
            </button>
            <span className="dsh-composer-status" title="周末 · Asia/Shanghai · 周末模式">周末</span>
            <button
              className="dsh-composer-model"
              type="button"
              title={modelLabel ? `Agent: ${modelLabel}` : '选择 Agent'}
              aria-label={modelLabel ? `选择 Agent，当前 ${modelLabel}` : '选择 Agent'}
            >
              <span className="dsh-composer-model-name">{modelLabel || '…'}</span>
              <span className="dsh-composer-model-effort">On</span>
              <svg width="12" height="12" viewBox="0 0 12 12" fill="none">
                <path d="M3 4.5l3 3 3-3" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round"/>
              </svg>
            </button>
            {isStreaming ? (
              <button
                className="dsh-composer-primary"
                onClick={handleStop}
                title="停止生成"
                type="button"
              >
                <IconStop size={16} />
              </button>
            ) : (
              <button
                className={`dsh-composer-primary ${!hasContent ? 'dsh-composer-primary-disabled' : ''}`}
                onClick={doSend}
                type="button"
                disabled={!hasContent}
                title="发送消息"
                aria-label="发送消息"
              >
                <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
                  <path d="M8 13V3M4 7l4-4 4 4" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round"/>
                </svg>
              </button>
            )}
          </div>
        </div>
      </form>
    </div>
  )
}
