/* InferGlow Web UI — 对话区（消息流 + 输入框）

   集成 DSH 插件槽位：
   - conversation.input.dock  → SteerQueueDock（投放队列，Composer 上方）
   - conversation.input.menu  → @file 候选下拉
   - conversation.input.right → FreezeButton（Composer 右侧）
   输入框由 MentionInput 包装，获得 `@` 文件引用能力。
   */

import { useState, useRef, useEffect } from 'react'
import type { Session, Agent, ChatMessage } from '../api/types'
import { get } from '../api/transport'
import { renderSlot } from '../plugin/registry'
import { MentionInput } from '../plugin/at-file'
import { useTrafficStore, useFreezeStore, selectFrozen } from '../plugin/input-traffic'
import type { QueueTier } from '../plugin/input-traffic/api'

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
  const [tier, setTier] = useState<QueueTier>('next')
  const messagesEndRef = useRef<HTMLDivElement>(null)

  const frozen = useFreezeStore(selectFrozen)

  // 同步流量队列/冻结的会话上下文
  const setTrafficSession = useTrafficStore((s) => s.setSession)
  useEffect(() => {
    setTrafficSession(session?.id ?? null)
  }, [session?.id, setTrafficSession])

  useEffect(() => {
    if (agent?.id) useTrafficStore.getState().setAgent(agent.id)
  }, [agent?.id])

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
    if (!session || !input.trim() || sending || frozen) return
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

  // 把当前输入投递到投放队列（而非立即发送）
  const enqueue = () => {
    useTrafficStore.getState().enqueue(input, tier)
    setInput('')
  }

  const pullBack = (text: string) => setInput(text)

  const inputDisabled = !connected || sending || frozen

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
        {/* 投放队列（Composer 上方） */}
        <div className="queue-dock-slot">
          {renderSlot('conversation.input.dock', { onPullBack: pullBack })}
        </div>

        <div className="input-wrapper">
          <div className="composer">
            <div className="composer-steer">
              <select
                className="steer-select"
                value={tier}
                disabled={frozen}
                title="投递层级"
                onChange={e => setTier(e.target.value as QueueTier)}
              >
                <option value="later">🟢 稍后</option>
                <option value="next">🟡 下一步</option>
                <option value="now">🔴 立即</option>
              </select>
              <button
                className="steer-btn"
                disabled={!connected || frozen || !input.trim()}
                onClick={enqueue}
                title="入队（稍后发送）"
              >
                ＋ 入队
              </button>
            </div>
            <div className="composer-menu">
              {renderSlot('conversation.input.menu')}
            </div>
            <MentionInput
              value={input}
              onChange={setInput}
              placeholder={connected ? '给智能体发消息...（@ 引用文件）' : '未连接后端...'}
              disabled={inputDisabled}
              onSubmit={handleSend}
            />
          </div>
          <div className="composer-right">
            {renderSlot('conversation.input.right', { disabled: !connected })}
          </div>
          <button
            className="send-btn"
            onClick={handleSend}
            disabled={inputDisabled || !input.trim()}
          >
            ↑
          </button>
        </div>
      </div>
    </div>
  )
}