import { create } from 'zustand'
import { transport, type MessagePage, type ToolStreamEvent, type Transport } from '../api'

export interface ChatMessage {
  id: string
  role: 'user' | 'assistant' | 'tool'
  content: string
  toolName?: string
  toolStatus?: string
  createdAt: number
}

/** Guards the stream-run contract quirks: run_end carries the assistant reply
 * in tool_name, and run_start may fire twice (server emits it manually plus
 * via the OnRunStart callback) — consumers must be idempotent. */
export function normalizeStreamEvent(ev: ToolStreamEvent): {
  kind: 'run_start' | 'llm_start' | 'llm_end' | 'tool_start' | 'tool_end' | 'run_end' | 'error'
} & Partial<ToolStreamEvent> {
  return { ...ev } as never
}

const MAX_MESSAGES = 500

interface ChatState {
  messages: ChatMessage[]
  streaming: boolean
  running: boolean
  error: string | null
  controller: AbortController | null
  runSeen: boolean
  hasMore: boolean
  nextBefore: string | null
  loadingOlder: boolean
  loadHistory: (sessionId: string, before?: string) => Promise<MessagePage | null>
  appendHistory: (page: MessagePage) => void
  loadOlder: (sessionId: string) => Promise<void>
  sendMessage: (sessionId: string, agentId: string, message: string) => Promise<void>
  /** Thread-level approval decision via the agent input endpoint. */
  approveInput: (agentId: string, approve: boolean) => Promise<void>
  stop: () => void
  clear: () => void
}

export const createChatStore = (t: Transport) =>
  create<ChatState>()((set, get) => ({
    messages: [],
    streaming: false,
    running: false,
    error: null,
    controller: null,
    runSeen: false,
    hasMore: false,
    nextBefore: null,
    loadingOlder: false,

    async loadHistory(sessionId, before) {
      const q = new URLSearchParams({ limit: '50' })
      if (before) q.set('before', before)
      const page = await t.request<MessagePage>('GET', `/sessions/${sessionId}/messages?${q}`)
      set({ hasMore: page.has_more, nextBefore: page.next_before })
      return page
    },

    appendHistory(page) {
      // Newest-first from the API; prepend older pages, append newer ones.
      const incoming: ChatMessage[] = page.messages.map((m) => ({
        id: m.id,
        role: m.role,
        content: m.content ?? '',
        toolName: m.tool_name,
        toolStatus: m.tool_status,
        createdAt: Date.parse(m.created_at),
      }))
      set({ messages: [...incoming, ...get().messages].slice(0, MAX_MESSAGES) })
    },

    async loadOlder(sessionId) {
      const { hasMore, nextBefore, loadingOlder } = get()
      if (!hasMore || !nextBefore || loadingOlder) return
      set({ loadingOlder: true })
      try {
        const page = await t.request<MessagePage>(
          'GET',
          `/sessions/${sessionId}/messages?limit=50&before=${encodeURIComponent(nextBefore)}`,
        )
        get().appendHistory(page)
        set({ hasMore: page.has_more, nextBefore: page.next_before })
      } catch (err) {
        set({ error: err instanceof Error ? err.message : String(err) })
      } finally {
        set({ loadingOlder: false })
      }
    },

    async sendMessage(sessionId, agentId, message) {
      const controller = new AbortController()
      set({ streaming: true, running: true, error: null, controller, runSeen: false })
      // Optimistically show the user message.
      set({
        messages: [
          ...get().messages,
          { id: `local-u-${Date.now()}`, role: 'user' as const, content: message, createdAt: Date.now() },
        ].slice(-MAX_MESSAGES),
      })

      // Fallback when stream-run is unavailable (agent not wired, 404/5xx):
      // retry through the synchronous chat endpoint and surface the reply.
      const fallbackChat = async (msg: string) => {
        try {
          const resp = await t.request<{ response: string }>('POST', `/agents/${agentId}/chat`, {
            message: msg,
            session_id: sessionId,
          })
          const st = get()
          const msgs = [...st.messages]
          const last = msgs[msgs.length - 1]
          if (last && last.role === 'assistant' && last.content === '') {
            msgs[msgs.length - 1] = { ...last, content: resp.response }
          } else {
            msgs.push({ id: `run-${Date.now()}`, role: 'assistant' as const, content: resp.response, createdAt: Date.now() })
          }
          set({ messages: msgs.slice(-MAX_MESSAGES) })
        } catch (err) {
          set({ error: err instanceof Error ? err.message : String(err) })
        }
      }

      await t.streamRun(
        agentId,
        { message, session_id: sessionId },
        {
          onEvent: (ev) => {
            const st = get()
            switch (ev.type) {
              case 'run_start':
                // Server may emit run_start twice (manual + callback); ignore
                // the duplicate so we don't create a second placeholder.
                if (st.runSeen) return
                set({ runSeen: true })
                set({
                  messages: [
                    ...st.messages,
                    { id: `run-${Date.now()}`, role: 'assistant' as const, content: '', createdAt: Date.now() },
                  ].slice(-MAX_MESSAGES),
                })
                break
              case 'llm_start':
                set({ running: true })
                break
              case 'llm_end':
                set({ running: false })
                break
              case 'tool_start':
                set({
                  messages: [
                    ...st.messages,
                    { id: `tool-${Date.now()}`, role: 'tool' as const, content: '', toolName: ev.tool_name, toolStatus: 'run', createdAt: Date.now() },
                  ].slice(-MAX_MESSAGES),
                })
                break
              case 'tool_end': {
                const msgs = [...st.messages]
                for (let i = msgs.length - 1; i >= 0; i--) {
                  if (msgs[i].role === 'tool' && msgs[i].toolName === ev.tool_name) {
                    msgs[i] = { ...msgs[i], toolStatus: ev.error ? 'error' : 'ok' }
                    break
                  }
                }
                set({ messages: msgs })
                break
              }
              case 'run_end': {
                // CONTRACT: run_end.tool_name carries the full assistant reply.
                const msgs = [...st.messages]
                const last = msgs[msgs.length - 1]
                if (last && last.role === 'assistant' && last.content === '') {
                  msgs[msgs.length - 1] = { ...last, content: ev.tool_name ?? (ev.error ? `错误：${ev.error}` : '') }
                } else if (ev.tool_name) {
                  msgs.push({ id: `run-${Date.now()}`, role: 'assistant' as const, content: ev.tool_name, createdAt: Date.now() })
                }
                set({ messages: msgs.slice(-MAX_MESSAGES), running: false })
                break
              }
              case 'error':
                set({ error: ev.error ?? 'agent error', running: false })
                break
            }
          },
          onDone: () => {
            set({ streaming: false, running: false, runSeen: false })
          },
          onError: (msg) => {
            // Degrade to the synchronous endpoint when stream-run fails.
            set({ streaming: false, running: false })
            void fallbackChat(message)
            set({ error: `流式不可用，已回退同步请求：${msg}` })
          },
        },
        controller.signal,
      )
    },

    stop() {
      get().controller?.abort()
      set({ streaming: false, running: false })
    },

    async approveInput(agentId, approve) {
      try {
        // POST /v1/agents/{id}/input carries the preempt/approval decision;
        // the agent maps "approve"/"reject" to its own continuation policy.
        await t.request<{ response: string }>('POST', `/agents/${agentId}/input`, {
          message: approve ? 'approve' : 'reject',
          preempt_mode: 'force',
        })
      } catch (err) {
        set({ error: err instanceof Error ? err.message : String(err) })
      }
    },

    clear() {
      set({ messages: [], error: null, runSeen: false, hasMore: false, nextBefore: null, loadingOlder: false })
    },
  }))

export const useChatStore = createChatStore(transport)
