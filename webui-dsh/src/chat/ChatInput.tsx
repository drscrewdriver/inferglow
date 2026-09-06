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
import { AgentPicker } from '../panels/AgentPicker.tsx'
import { ConfigPopover } from '../panels/ConfigPopover.tsx'

interface ChatInputProps {
  sessionId: string | null
  placeholder?: string
}

/** DSH access levels — the composer permission chip's config list. */
const PERMISSION_ITEMS = [
  { id: 'readonly', label: 'Readonly', detail: '只读 — 仅允许读取工作区，不执行写操作' },
  { id: 'workspace-write', label: 'Workspace Write', detail: '工作区内可写(推荐)' },
  { id: 'permissive', label: 'Permissive', detail: '宽松 — 工作区外读、有限写' },
  { id: 'full-access', label: 'Full Access', detail: '完全访问 — 无工作区边界(慎用)' },
] as const

const PERM_LABELS: Record<string, string> = Object.fromEntries(
  PERMISSION_ITEMS.map(i => [i.id, i.label]),
)

function genMsgId(): string {
  return `msg-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
}

export function ChatInput({ sessionId, placeholder }: ChatInputProps) {
  const [value, setValue] = useState('')
  const [modelLabel, setModelLabel] = useState('')
  const [pickerOpen, setPickerOpen] = useState(false)
  const pickerAnchorRef = useRef<HTMLSpanElement | null>(null)
  const [chipOpen, setChipOpen] = useState<null | 'permission' | 'context'>(null)
  const permChipRef = useRef<HTMLButtonElement | null>(null)
  const ctxChipRef = useRef<HTMLButtonElement | null>(null)
  const [perm, setPerm] = useState(store.settings.permission)
  const [ctxMode, setCtxMode] = useState(store.settings.contextMode)
  const [ctxModes, setCtxModes] = useState<{ id: string; description: string }[] | null>(null)
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
              <button ref={permChipRef} className="dsh-composer-mode" type="button"
                title={`访问级别，当前：${PERM_LABELS[perm]}`} aria-haspopup="listbox" aria-expanded={chipOpen === 'permission'}
                onClick={() => setChipOpen(o => (o === 'permission' ? null : 'permission'))}>
                <span className="dsh-composer-mode-icon">
                  <svg width="14" height="14" viewBox="0 0 16 16" fill="none">
                    <path d="M4 9l3 3 5-6" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round"/>
                  </svg>
                </span>
                <span className="dsh-composer-mode-label">{PERM_LABELS[perm]}</span>
                <svg width="12" height="12" viewBox="0 0 12 12" fill="none">
                  <path d="M3 4.5l3 3 3-3" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round"/>
                </svg>
              </button>
              {chipOpen === 'permission' && (
                <ConfigPopover anchor={permChipRef.current} items={PERMISSION_ITEMS.map(i => ({
                  id: i.id, label: i.label, detail: i.detail, selected: i.id === perm,
                }))} footer="DSH 访问级别 · 后端工具门槛映射二期"
                  onPick={id => { const v = id as typeof perm; setPerm(v); store.updateSetting('permission', v); setChipOpen(null) }}
                  onClose={() => setChipOpen(null)} />
              )}
              <button ref={ctxChipRef} className="dsh-composer-mode" type="button"
                title={`上下文管理模式，当前：${ctxMode}`} aria-haspopup="listbox" aria-expanded={chipOpen === 'context'}
                onClick={() => {
                  setChipOpen(o => (o === 'context' ? null : 'context'))
                  if (ctxModes === null) {
                    void api.contextModes().then(ms => setCtxModes(ms)).catch(() => setCtxModes([]))
                  }
                }}>
                <span className="dsh-composer-mode-icon">
                  <svg width="14" height="14" viewBox="0 0 16 16" fill="none">
                    <path d="M3 3h10M3 6.5h10M3 10h6" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round"/>
                  </svg>
                </span>
                <span className="dsh-composer-mode-label">上下文:{ctxMode}</span>
                <svg width="12" height="12" viewBox="0 0 12 12" fill="none">
                  <path d="M3 4.5l3 3 3-3" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round"/>
                </svg>
              </button>
              {chipOpen === 'context' && (
                <ConfigPopover anchor={ctxChipRef.current}
                  items={(ctxModes ?? []).map(m => ({ id: m.id, label: m.id, detail: m.description, selected: m.id === ctxMode }))}
                  emptyText="后端未提供上下文模式列表" footer="GET /v1/context/modes · 引擎 per-run 切换二期"
                  onPick={id => { setCtxMode(id); store.updateSetting('contextMode', id); setChipOpen(null) }}
                  onClose={() => setChipOpen(null)} />
              )}
            </div>
          </div>
          <div className="dsh-composer-trailing">
            <button className="dsh-composer-freeze" type="button" title="冻结会话" aria-label="冻结会话">
              <span className="dsh-composer-freeze-label">冻结会话</span>
            </button>
            <span className="dsh-composer-status" title="周末 · Asia/Shanghai · 周末模式">周末</span>
            <span ref={pickerAnchorRef} style={{ position: 'relative', display: 'inline-flex' }}>
            <button
              className="dsh-composer-model"
              type="button"
              title={modelLabel ? `Agent: ${modelLabel}` : '选择 Agent'}
              aria-label={modelLabel ? `选择 Agent，当前 ${modelLabel}` : '选择 Agent'}
              aria-haspopup="listbox"
              aria-expanded={pickerOpen}
              onClick={() => setPickerOpen(o => !o)}
            >
              <span className="dsh-composer-model-name">{modelLabel || '…'}</span>
              <span className="dsh-composer-model-effort">On</span>
              <svg width="12" height="12" viewBox="0 0 12 12" fill="none">
                <path d="M3 4.5l3 3 3-3" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round"/>
              </svg>
            </button>
            {pickerOpen && <AgentPicker anchor={pickerAnchorRef.current} onClose={() => setPickerOpen(false)} />}
            </span>
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
