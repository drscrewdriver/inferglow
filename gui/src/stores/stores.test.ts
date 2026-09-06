import { beforeEach, describe, expect, it } from 'vitest'
import type { MessagePage, ToolStreamEvent, Transport } from '../api'
import { createChatStore } from './chatStore'
import { createSessionStore } from './sessionStore'
import { createUsageStore } from './usageStore'

/** In-memory transport double: records calls and replays canned responses. */
function mockTransport(overrides: Partial<Transport> = {}): Transport & { calls: string[] } {
  const calls: string[] = []
  return {
    calls,
    async request<T>(method: string, path: string): Promise<T> {
      calls.push(`${method} ${path}`)
      if (path === '/sessions' && method === 'GET') {
        return {
          sessions: [
            { id: 's1', agent_id: 'a1', status: 'active', title: '会话一', group: '项目', pinned: true, created_at: '2026-08-13T00:00:00Z', updated_at: '2026-08-13T00:00:00Z' },
            { id: 's2', agent_id: 'a1', status: 'active', title: '会话二', created_at: '2026-08-13T00:00:00Z', updated_at: '2026-08-13T00:00:00Z' },
          ],
          count: 2,
          groups: { 项目: 1 },
          query: {},
        } as T
      }
      if (path.startsWith('/sessions/s1/messages')) {
        return {
          messages: [
            { id: 'm2', session_id: 's1', role: 'assistant', content: '回复', created_at: '2026-08-13T00:01:00Z' },
            { id: 'm1', session_id: 's1', role: 'user', content: '你好', created_at: '2026-08-13T00:00:30Z' },
          ],
          has_more: false,
          next_before: null,
        } as T
      }
      if (path === '/sessions' && method === 'POST') {
        return { id: 's3', agent_id: 'a1', status: 'active', title: '新会话', created_at: '2026-08-13T00:00:00Z', updated_at: '2026-08-13T00:00:00Z' } as T
      }
      if (path === '/usage/report') {
        return { generated_at: '2026-08-13T00:00:00Z', from: '2026-08-01T00:00:00Z', to: '2026-09-01T00:00:00Z', overall: { total_prompt_tokens: 180, total_cached_tokens: 60, cache_hit_rate: 0.33, total_cost: 0.01, actual_cost: 0.008, savings: 0.002, currency: 'USD', session_count: 2 } } as T
      }
      throw new Error(`unexpected request ${method} ${path}`)
    },
    async streamRun(agentId, _req, handlers, _signal): Promise<void> {
      calls.push(`STREAM ${agentId}`)
      // Simulate the real server event sequence, including the contract
      // quirk of a duplicated run_start.
      const ev = (type: string, extra: Partial<ToolStreamEvent> = {}): ToolStreamEvent => ({ type, timestamp: new Date().toISOString(), ...extra })
      handlers.onEvent(ev('run_start'))
      handlers.onEvent(ev('run_start')) // duplicate — must be ignored
      handlers.onEvent(ev('tool_start', { tool_name: 'explore' }))
      handlers.onEvent(ev('tool_end', { tool_name: 'explore' }))
      handlers.onEvent(ev('run_end', { tool_name: '这是完整回复' }))
      handlers.onDone?.('a1')
    },
    ...overrides,
  }
}

describe('sessionStore', () => {
  const t = mockTransport()
  const store = createSessionStore(t)

  beforeEach(() => {
    store.setState({ sessions: [], activeId: null, error: null })
  })

  it('fetches sessions from GET /v1/sessions', async () => {
    await store.getState().fetchSessions()
    expect(store.getState().sessions).toHaveLength(2)
    expect(store.getState().sessions[0].pinned).toBe(true)
  })

  it('creates a session via POST /v1/sessions', async () => {
    const rec = await store.getState().createSession('a1')
    expect(rec?.id).toBe('s3')
    expect(store.getState().sessions).toHaveLength(1)
  })
})

describe('chatStore', () => {
  it('maps the stream-run event sequence into messages, deduping run_start', async () => {
    const store = createChatStore(mockTransport())
    await store.getState().sendMessage('s1', 'a1', '你好')

    const { messages, streaming, running } = store.getState()
    expect(streaming).toBe(false)
    expect(running).toBe(false)

    // user + one assistant placeholder (deduped) + tool + assistant reply
    expect(messages.filter((m) => m.role === 'user')).toHaveLength(1)
    expect(messages.filter((m) => m.role === 'assistant')).toHaveLength(2)
    const reply = messages.find((m) => m.role === 'assistant' && m.content !== '')
    expect(reply?.content).toBe('这是完整回复')
    const tool = messages.find((m) => m.role === 'tool')
    expect(tool?.toolName).toBe('explore')
    expect(tool?.toolStatus).toBe('ok')
  })

  it('records tool errors in tool_status', async () => {
    const t = mockTransport({
      async streamRun(_agentId, _req, handlers) {
        const ev = (type: string, extra: Partial<ToolStreamEvent> = {}): ToolStreamEvent => ({ type, timestamp: '', ...extra })
        handlers.onEvent(ev('tool_start', { tool_name: 'web' }))
        handlers.onEvent(ev('tool_end', { tool_name: 'web', error: 'timeout' }))
        handlers.onDone?.('a1')
      },
    })
    const store = createChatStore(t)
    await store.getState().sendMessage('s1', 'a1', 'x')
    const tool = store.getState().messages.find((m) => m.role === 'tool')
    expect(tool?.toolStatus).toBe('error')
  })

  it('posts approval decisions to the agent input endpoint', async () => {
    const t = mockTransport()
    const store = createChatStore(t)
    await store.getState().approveInput('a1', true)
    await store.getState().approveInput('a1', false)
    expect(t.calls.filter((c) => c.startsWith('POST /agents/a1/input'))).toHaveLength(2)
  })

  it('loads history pages and prepends them', async () => {
    const store = createChatStore(mockTransport())
    const page = await store.getState().loadHistory('s1')
    expect(page).not.toBeNull()
    store.getState().appendHistory(page as MessagePage)
    expect(store.getState().messages).toHaveLength(2)
    expect(store.getState().messages[0].role).toBe('assistant') // newest first
  })
})

describe('usageStore', () => {
  it('fetches the usage report', async () => {
    const store = createUsageStore(mockTransport())
    await store.getState().fetchReport()
    expect(store.getState().report?.overall.total_prompt_tokens).toBe(180)
  })
})
