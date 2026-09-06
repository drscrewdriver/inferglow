/**
 * InferGlow API client for the DSH shell.
 *
 * Wires the vendored DSH UI (mock-driven) to the real backend. The api/*
 * files ported from the host webui are kept verbatim (same-origin BASE '');
 * this client adds a configurable base (store.settings.apiEndpoint) and the
 * streaming chat contract.
 *
 * Chat path: POST /v1/agents/{id}/stream-run (SSE). Backend quirk: the
 * run_end event carries the full assistant reply in its `tool_name` field
 * (server/handlers_stream.go) — onDone receives it as the final reply.
 * Falls back to non-streaming POST /v1/agents/{id}/chat when SSE is
 * unavailable. There is no token-delta event in the current contract, so
 * the reply arrives whole at run_end.
 */
import { consumeSSE } from './sse.ts'
import type { Agent, ChatMessage, Session } from './types.ts'

export interface SendChatHandlers {
  onRunStart?(): void
  onLLMStart?(round: number): void
  /** Incremental assistant text chunk (real streaming). */
  onDelta?(text: string): void
  /** Incremental reasoning chunk (DeepSeek/MiMo style providers). */
  onReasoning?(text: string): void
  /** Provider-reported token usage per LLM call. */
  onUsage?(usage: { prompt_tokens: number; completion_tokens: number; total_tokens: number }): void
  onToolStart?(toolName: string): void
  onToolEnd?(toolName: string, err?: string): void
  /** Full assistant reply (from run_end / chat fallback) — authoritative. */
  onDone(reply: string): void
  onError(message: string): void
}

export interface SendChatOptions {
  agentId: string
  sessionId?: string
  message: string
  handlers: SendChatHandlers
}

export interface FsEntry {
  name: string
  path: string
  is_dir: boolean
  hidden?: boolean
}

export interface FsTreeResult {
  root: string
  path: string
  entries: FsEntry[]
}

export interface FsReadResult {
  root: string
  path: string
  content: string
  bytes: number
}

export interface ProducedFile {
  path: string
  bytes: number
  modified: string
}

/** Mirrors observability.SpanSummary (duration in nanoseconds). */
export interface SpanSummary {
  name: string
  kind: 'llm' | 'tool' | 'agent' | 'compress' | 'retrieval' | 'internal'
  duration_ns: number
  end_time: string
  has_error: boolean
  attrs?: Record<string, string>
}

export interface InferGlowApi {
  health(): Promise<boolean>
  listAgents(): Promise<Agent[]>
  listSessions(): Promise<Session[]>
  createSession(agentId: string, title?: string): Promise<Session>
  deleteSession(sessionId: string): Promise<void>
  listMessages(sessionId: string, limit?: number): Promise<ChatMessage[]>
  /** One-level directory listing (lazy-loading friendly). */
  fsTree(path?: string): Promise<FsTreeResult>
  /** Whole-file read (server caps at 10MB). */
  fsRead(path: string): Promise<FsReadResult>
  /** Recursive filename substring search. */
  fsSearch(q: string, limit?: number): Promise<{ query: string; matches: string[]; truncated: boolean }>
  /** Most recently modified files under the workspace. */
  producedFiles(limit?: number): Promise<{ files: ProducedFile[]; count: number }>
  /** Recent finished spans (bare JSON array; 503 when collector disabled). */
  spans(limit?: number): Promise<SpanSummary[]>
  sendChat(opts: SendChatOptions): Promise<void>
  /** Abort the in-flight stream-run request, if any. */
  cancel(): void
}

function trimBase(raw: string | undefined): string {
  const base = (raw ?? '').trim()
  return base.endsWith('/') ? base.slice(0, -1) : base
}

function authHeaders(getApiKey: () => string, init?: RequestInit): RequestInit {
  // Attach Bearer when the user stored an API key (server -api-key mode).
  const key = (getApiKey() ?? '').trim()
  if (!key) return init ?? {}
  const headers = new Headers(init?.headers)
  headers.set('Authorization', `Bearer ${key}`)
  return { ...init, headers }
}

async function request<T>(base: string, path: string, getApiKey: () => string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${base}${path}`, {
    ...authHeaders(getApiKey, init),
    headers: { 'Content-Type': 'application/json', ...init?.headers },
  })
  if (!res.ok) {
    const text = await res.text().catch(() => res.statusText)
    throw new Error(`${res.status} ${text}`)
  }
  return res.json() as Promise<T>
}

export function createInferGlowApi(
  getBase: () => string = () => '',
  getApiKey: () => string = () => '',
): InferGlowApi {
  let controller: AbortController | null = null
  const base = () => trimBase(getBase())

  async function listAgents(): Promise<Agent[]> {
    const data = await request<{ agents: Agent[] }>(base(), '/v1/agents', getApiKey)
    return data.agents ?? []
  }

  async function listSessions(): Promise<Session[]> {
    const data = await request<{ sessions: Session[] }>(base(), '/v1/sessions', getApiKey)
    return data.sessions ?? []
  }

  async function listMessages(sessionId: string, limit = 50): Promise<ChatMessage[]> {
    const data = await request<{ messages: ChatMessage[] }>(
      base(),
      `/v1/sessions/${encodeURIComponent(sessionId)}/messages?limit=${limit}`,
      getApiKey,
    )
    // Backend returns newest-first; the chat view is chronological.
    return (data.messages ?? []).slice().reverse()
  }

  async function sendChatSSE(opts: SendChatOptions): Promise<boolean> {
    controller = new AbortController()
    let res: Response
    try {
      res = await fetch(
        `${base()}/v1/agents/${encodeURIComponent(opts.agentId)}/stream-run`,
        authHeaders(getApiKey, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            message: opts.message,
            session_id: opts.sessionId || undefined,
          }),
          signal: controller.signal,
        }),
      )
    } catch (err) {
      controller = null
      throw err
    }
    if (!res.ok || !res.body || !(res.headers.get('Content-Type') ?? '').includes('text/event-stream')) {
      controller = null
      return false
    }

    let reply = ''
    let failure = ''
    try {
      for await (const ev of consumeSSE(res)) {
        let payload: Record<string, unknown> = {}
        try {
          payload = JSON.parse(ev.data) as Record<string, unknown>
        } catch {
          /* keep-empty payload for events without data */
        }
        const toolName = typeof payload.tool_name === 'string' ? payload.tool_name : ''
        const round = typeof payload.round === 'number' ? payload.round : 0
        const errText = typeof payload.error === 'string' ? payload.error : ''
        const delta = typeof payload.delta === 'string' ? payload.delta : ''
        const usage = payload.usage as
          | { prompt_tokens: number; completion_tokens: number; total_tokens: number }
          | undefined
        switch (ev.event) {
          case 'run_start':
            opts.handlers.onRunStart?.()
            break
          case 'llm_start':
            opts.handlers.onLLMStart?.(round)
            break
          case 'delta':
            if (delta) opts.handlers.onDelta?.(delta)
            break
          case 'reasoning':
            if (delta) opts.handlers.onReasoning?.(delta)
            break
          case 'llm_end':
            if (usage) opts.handlers.onUsage?.(usage)
            break
          case 'tool_start':
            opts.handlers.onToolStart?.(toolName)
            break
          case 'tool_end':
            opts.handlers.onToolEnd?.(toolName, errText || undefined)
            break
          case 'run_end':
            // Contract quirk: full assistant reply rides in tool_name.
            reply = toolName
            if (errText) failure = errText
            break
          case 'error':
            failure = errText || 'agent run failed'
            break
          case 'done':
            break
          default:
            break
        }
        if (failure) break
      }
    } catch (err) {
      if ((err as Error)?.name !== 'AbortError') {
        controller = null
        throw err
      }
      controller = null
      return true // aborted: caller marks the message stopped
    }
    controller = null
    if (failure && !reply) {
      opts.handlers.onError(failure)
      return true
    }
    opts.handlers.onDone(reply)
    return true
  }

  return {
    async health() {
      try {
        const res = await fetch(`${base()}/health`)
        return res.ok
      } catch {
        return false
      }
    },
    listAgents,
    listSessions,
    async createSession(agentId, title) {
      return request<Session>(base(), '/v1/sessions', getApiKey, {
        method: 'POST',
        body: JSON.stringify({ agent_id: agentId, title }),
      })
    },
    async deleteSession(sessionId) {
      await fetch(`${base()}/v1/sessions/${encodeURIComponent(sessionId)}`, authHeaders(getApiKey, {
        method: 'DELETE',
      }))
    },
    listMessages,
    async fsTree(path = '') {
      return request<FsTreeResult>(
        base(),
        `/v1/fs/tree${path ? `?path=${encodeURIComponent(path)}` : ''}`,
        getApiKey,
      )
    },
    async fsRead(path: string) {
      return request<FsReadResult>(
        base(),
        `/v1/fs/read?path=${encodeURIComponent(path)}`,
        getApiKey,
      )
    },
    async fsSearch(q: string, limit = 200) {
      return request<{ query: string; matches: string[]; truncated: boolean }>(
        base(),
        `/v1/fs/search?q=${encodeURIComponent(q)}&limit=${limit}`,
        getApiKey,
      )
    },
    async producedFiles(limit = 10) {
      return request<{ files: ProducedFile[]; count: number }>(
        base(),
        `/v1/produced-files?limit=${limit}`,
        getApiKey,
      )
    },
    async spans(limit = 500) {
      // The endpoint responds with a bare JSON array; a 503 body means the
      // collector is not wired on the server (面板显示"观测未启用"空态).
      const res = await fetch(`${base()}/v1/observability/spans?limit=${limit}`, authHeaders(getApiKey))
      if (!res.ok) throw new Error(`${res.status} ${(await res.text().catch(() => ''))}`)
      return (await res.json()) as SpanSummary[]
    },
    async sendChat(opts) {
      const streamed = await sendChatSSE(opts)
      if (streamed) return
      // Fallback: non-streaming chat.
      const data = await request<{ response: string }>(
        base(),
        `/v1/agents/${encodeURIComponent(opts.agentId)}/chat`,
        getApiKey,
        {
          method: 'POST',
          body: JSON.stringify({
            message: opts.message,
            session_id: opts.sessionId || undefined,
          }),
        },
      )
      opts.handlers.onDone(data.response ?? '')
    },
    cancel() {
      controller?.abort()
      controller = null
    },
  }
}
