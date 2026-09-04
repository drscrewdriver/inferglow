/**
 * Chat input — DSH-style layered composer card with:
 * - Multi-line textarea with auto-resize
 * - Send on Enter (Shift+Enter for newline)
 * - Stop generation button during streaming
 * - Toolbar: [+] command, access mode, freeze, status, model, send
 * - Placeholder "描述你想要构建的内容"
 */

import { useState, useRef, type FormEvent } from 'react'
import { IconStop } from '../components/Icons.tsx'
import { store } from '../store.ts'

interface ChatInputProps {
  sessionId: string | null
  placeholder?: string
}

export function ChatInput({ sessionId, placeholder }: ChatInputProps) {
  const [value, setValue] = useState('')
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    doSend()
  }

  function doSend() {
    const text = value.trim()
    if (!text) return

    // If there's no active session (hero state), create one so the reply simulates.
    let sid = sessionId
    if (!sid) {
      store.createSession()
      sid = store.activeSessionId ?? ''
    }

    const userMsg = {
      id: `msg-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
      role: 'user' as const,
      content: text,
      status: 'sent' as const,
      timestamp: Date.now(),
    }

    store.addMessage(sid, userMsg)
    setValue('')

    if (textareaRef.current) {
      textareaRef.current.style.height = 'auto'
    }

    simulateStreaming(sid)
  }

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      doSend()
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
            <button className="dsh-composer-model" type="button" title="Qwen3.6-35B-A3B · On" aria-label="选择模型，当前 Qwen3.6-35B-A3B，推理等级 On">
              <span className="dsh-composer-model-name">Qwen3.6-35B-A3B</span>
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

/* Simulate a streaming response */
function simulateStreaming(sessionId: string | null) {
  const assistantMsg = {
    id: `msg-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
    role: 'assistant' as const,
    content: '',
    status: 'streaming' as const,
    timestamp: Date.now(),
  }

  store.addMessage(sessionId ?? '', assistantMsg)
  store.setStreaming(true, assistantMsg.id)

  const responses = [
    `你好！这是一个**演示响应**。我支持以下功能：\n\n- **Markdown** 渲染\n- 代码块高亮\n- 工具调用展示\n- 思考过程折叠\n\n\`\`\`javascript\nfunction hello() {\n  console.log("Hello, DSH!");\n}\n\`\`\`\n\n有什么我可以帮助你的？`,
    `当然可以！让我来分析一下你的问题。\n\n这是一个**流式响应**的模拟。在完整集成中，这里会对接真实的 LLM 后端，通过 SSE/WS 推送流式内容。\n\n当前状态：\n- ✅ Session 管理\n- ✅ Markdown 渲染  \n- ✅ 主题切换\n- ⏳ API 对接`,
    `好的，我来处理这个请求。\n\n### 分析\n\n你的问题涉及以下几个方面：\n\n1. **架构设计** — 需要考虑可扩展性\n2. **性能优化** — 关注响应时间\n3. **用户体验** — 界面友好性\n\n\`\`\`python\ndef optimize(data):\n    # TODO: 实现优化逻辑\n    return processed_data\n\`\`\`\n\n需要我详细展开某个部分吗？`,
  ]
  
  const response = responses[Math.floor(Math.random() * responses.length)]
  let charIndex = 0

  function streamChunk() {
    if (charIndex >= response.length || !store.isStreaming) return
    
    const chunk = response.slice(charIndex, charIndex + 3)
    charIndex += 3
    
    // 追加内容而非覆盖
    const session = store.sessions.find(s => s.id === sessionId)
    const msg = session?.messages.find(m => m.id === assistantMsg.id)
    store.updateMessage(sessionId ?? '', assistantMsg.id, {
      content: (msg?.content ?? '') + chunk,
      status: 'streaming' as const,
    })
    
    setTimeout(streamChunk, 30 + Math.random() * 50)
  }

  // Start streaming
  streamChunk()

  // Finish after streaming completes
  setTimeout(() => {
    store.updateMessage(sessionId ?? '', assistantMsg.id, {
      status: 'sent' as const,
    })
    store.setStreaming(false)
  }, response.length * 40 + 500)
}