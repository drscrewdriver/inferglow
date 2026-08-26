/* InferGlow Web UI — 对话区（消息流 + 输入框） */

import { useState, useRef, useEffect } from 'react'
import type { Session, Agent, ChatMessage } from '../api/types'
import { get } from '../api/transport'

interface ChatAreaProps {
  session: Session | null
  agent: Agent | null
  connected: boolean
  activeTab: 'chat' | 'trace' | 'context'
}

export function ChatArea({ session, agent, connected, activeTab }: ChatAreaProps) {
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [input, setInput] = useState('')
  const [sending, setSending] = useState(false)
  const messagesEndRef = useRef<HTMLDivElement>(null)

  // 加载会话消息
  useEffect(() => {
    if (!session) { setMessages([]); return }
    let cancelled = false
    async function load() {
      try {
        const res = await get<{ messages: ChatMessage[] }>(
          `/v1/sessions/${session!.id}/messages?limit=50`
        )
        if (!cancelled) setMessages(res.messages ?? [])
      } catch {
        if (!cancelled) setMessages([])
      }
    }
    load()
    return () => { cancelled = true }
  }, [session?.id])

  // 自动滚到底部
  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages.length])

  const handleSend = async () => {
    if (!session || !input.trim() || sending) return
    const text = input.trim()
    setInput('')
    setSending(true)

    // 乐观插入用户消息
    const userMsg: ChatMessage = {
      id: `tmp-${Date.now()}`,
      role: 'user',
      content: text,
      created_at: new Date().toISOString(),
    }
    setMessages(prev => [...prev, userMsg])

    try {
      // TODO: 对接 SSE stream-run
      const res = await fetch(`/v1/agents/${session.agent_id}/chat`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ message: text, session_id: session.id }),
      })
      const data = await res.json() as { response?: string }
      if (data.response) {
        setMessages(prev => [...prev, {
          id: `resp-${Date.now()}`,
          role: 'assistant',
          content: data.response!,
          created_at: new Date().toISOString(),
        }])
      }
    } catch {
      // 静默
    } finally {
      setSending(false)
    }
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSend()
    }
  }

  // 轨迹 / 上下文标签页占位
  if (activeTab !== 'chat') {
    return (
      <div className="chat-area">
        <div className="chat-empty">
          <div className="chat-empty-icon">{activeTab === 'trace' ? '📊' : '🔍'}</div>
          <div>{activeTab === 'trace' ? '轨迹视图（待实现）' : '上下文视图（待实现）'}</div>
        </div>
      </div>
    )
  }

  return (
    <div className="chat-area">
      <div className="chat-messages">
        {!session ? (
          <div className="chat-empty">
            <div className="chat-empty-icon">💬</div>
            <div>选择一个会话开始对话</div>
          </div>
        ) : messages.length === 0 ? (
          <div className="chat-empty">
            <div className="chat-empty-icon">✨</div>
            <div>发送第一条消息</div>
            {agent && (
              <div style={{ fontSize: 12, color: 'var(--text-faint)' }}>
                Agent: {agent.name} · Model: {agent.model ?? 'default'}
              </div>
            )}
          </div>
        ) : (
          messages.map(msg => (
            <div key={msg.id} className={`message ${msg.role}`}>
              <div className="message-role">
                {msg.role === 'user' ? '你' : msg.role === 'assistant' ? 'Agent' : msg.role}
                {msg.tool_name && <span style={{ fontWeight: 400, color: 'var(--text-faint)' }}> · {msg.tool_name}</span>}
              </div>
              <div className="message-content">{msg.content}</div>
            </div>
          ))
        )}
        <div ref={messagesEndRef} />
      </div>

      <div className="chat-input-area">
        <div className="input-wrapper">
          <textarea
            className="input-box"
            placeholder={connected ? '给智能体发消息...' : '未连接后端...'}
            value={input}
            onChange={e => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            disabled={!connected || sending}
            rows={1}
          />
          <button
            className="send-btn"
            onClick={handleSend}
            disabled={!connected || sending || !input.trim()}
          >
            ↑
          </button>
        </div>
      </div>
    </div>
  )
}
