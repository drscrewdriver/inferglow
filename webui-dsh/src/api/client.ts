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

/** One todo item (shared with the model's task_tracker tools). */
export interface TaskItem {
  id: string
  title: string
  status: string // pending | in_progress | done | cancelled
  description?: string
  priority: number
  created_at: number
  updated_at: number
}

export interface TraceSpanLine {
  kind: string // agent | llm | tool
  name: string
  duration_ms: number
  error?: boolean
}

/** One persisted run summary (trace-role record). */
export interface SessionTrace {
  id: string
  session_id: string
  content: string // JSON: {agent_id,start,duration,spans,usage,error}
  created_at: string
}

/** Tagged 401 — the caller's key is missing or invalid. */
export class AuthError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'AuthError'
  }
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
  createSession(agentId: string, title?: string, workspace?: string): Promise<Session>
  deleteSession(sessionId: string): Promise<void>
  renameSession(sessionId: string, title: string): Promise<void>
  listMessages(sessionId: string, limit?: number): Promise<ChatMessage[]>
  /** One-level directory listing (lazy-loading friendly). */
  fsTree(path?: string, workspace?: string): Promise<FsTreeResult>
  /** Whole-file read (server caps at 10MB). */
  fsRead(path: string, workspace?: string): Promise<FsReadResult>
  sessionTrace(sessionId: string, limit?: number): Promise<SessionTrace[]>
  contextModes(): Promise<{ id: string; description: string }[]>
  listTasks(status?: string): Promise<TaskItem[]>
  createTask(title: string): Promise<TaskItem>
  updateTask(id: string, changes: { status?: string; title?: string }): Promise<TaskItem>
  deleteTask(id: string): Promise<void>
  /** Recursive filename substring search. */
  fsSearch(q: string, limit?: number, workspace?: string): Promise<{ query: string; matches: string[]; truncated: boolean }>
  /** Most recently modified files under the workspace. */
  producedFiles(limit?: number, workspace?: string): Promise<{ files: ProducedFile[]; count: number }>
  /** Recent finished spans (bare JSON array; 503 when collector disabled). */
  spans(limit?: number): Promise<SpanSummary[]>
  /** Registered workspaces (name→root) backing the workspace selector. */
  listWorkspaces(): Promise<{ name: string; root: string }[]>
  /** Register a new workspace (POST /v1/workspaces). */
  createWorkspace(name: string, root: string): Promise<{ name: string; root: string }>
  /** Gated one-command execution (POST /v1/exec; server -api-key + -exec). */
  execRun(opts: { argv: string[]; workspace?: string; workdir?: string; timeoutMs?: number }): Promise<{
    exit_code: number
    stdout: string
    stderr: string
    duration_ms: number
    truncated: boolean
  }>
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
  // authHeaders must run LAST so its Authorization survives the literal
  // headers spread (a trailing literal would clobber it — the 401 bug).
  const res = await fetch(`${base}${path}`, authHeaders(getApiKey, {
    ...init,
    headers: { 'Content-Type': 'application/json', ...init?.headers },
  }))
  if (!res.ok) {
    const text = await res.text().catch(() => res.statusText)
    // Tag 401s so the bridge can surface "API key missing/invalid" instead
    // of a silent empty list (the fresh-browser bug).
    if (res.status === 401) {
      throw new AuthError(`401 ${text}`)
    }
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
    async createSession(agentId, title, workspace) {
      return request<Session>(base(), '/v1/sessions', getApiKey, {
        method: 'POST',
        body: JSON.stringify({ agent_id: agentId, title, workspace: workspace || undefined }),
      })
    },
    async renameSession(sessionId, title) {
      await request<void>(base(), `/v1/sessions/${encodeURIComponent(sessionId)}`, getApiKey, {
        method: 'PATCH',
        body: JSON.stringify({ title }),
      })
    },
    async deleteSession(sessionId) {
      await fetch(`${base()}/v1/sessions/${encodeURIComponent(sessionId)}`, authHeaders(getApiKey, {
        method: 'DELETE',
      }))
    },
    listMessages,
    async fsTree(path = '', workspace?: string) {
      const qs = new URLSearchParams()
      if (path) qs.set('path', path)
      if (workspace) qs.set('workspace', workspace)
      const q = qs.toString()
      return request<FsTreeResult>(base(), `/v1/fs/tree${q ? `?${q}` : ''}`, getApiKey)
    },
    async contextModes() {
      return request<{ id: string; description: string }[]>(
        base(),
        '/v1/context/modes',
        getApiKey,
      )
    },
    async listTasks(status?: string) {
      return request<{ tasks: TaskItem[] }>(base(), '/v1/tasks' + (status ? `?status=${status}` : ''), getApiKey)
        .then(d => d.tasks ?? [])
    },
    async createTask(title: string) {
      return request<TaskItem>(base(), '/v1/tasks', getApiKey, {
        method: 'POST',
        body: JSON.stringify({ title }),
      })
    },
    async updateTask(id: string, changes: { status?: string; title?: string }) {
      return request<TaskItem>(base(), `/v1/tasks/${encodeURIComponent(id)}`, getApiKey, {
        method: 'PATCH',
        body: JSON.stringify(changes),
      })
    },
    async deleteTask(id: string) {
      await fetch(`${base()}/v1/tasks/${encodeURIComponent(id)}`, authHeaders(getApiKey, { method: 'DELETE' }))
    },
    async sessionTrace(sessionId: string, limit = 100) {
      const data = await request<{ traces: SessionTrace[] }>(
        base(),
        `/v1/sessions/${encodeURIComponent(sessionId)}/trace?limit=${limit}`,
        getApiKey,
      )
      return data.traces ?? []
    },
    async fsRead(path: string, workspace?: string) {
      return request<FsReadResult>(
        base(),
        `/v1/fs/read?path=${encodeURIComponent(path)}&workspace=${encodeURIComponent(workspace ?? '')}`,
        getApiKey,
      )
    },
    async fsSearch(q: string, limit = 200, workspace?: string) {
      return request<{ query: string; matches: string[]; truncated: boolean }>(
        base(),
        `/v1/fs/search?q=${encodeURIComponent(q)}&limit=${limit}&workspace=${encodeURIComponent(workspace ?? '')}`,
        getApiKey,
      )
    },
    async producedFiles(limit = 10, workspace?: string) {
      return request<{ files: ProducedFile[]; count: number }>(
        base(),
        `/v1/produced-files?limit=${limit}&workspace=${encodeURIComponent(workspace ?? '')}`,
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
    async listWorkspaces() {
      return request<{ name: string; root: string }[]>(base(), '/v1/workspaces', getApiKey)
    },
    async createWorkspace(name: string, root: string) {
      return request<{ name: string; root: string }>(base(), '/v1/workspaces', getApiKey, {
        method: 'POST',
        body: JSON.stringify({ name, root_dir: root }),
      })
    },
    async execRun(opts) {
      return request<{ exit_code: number; stdout: string; stderr: string; duration_ms: number; truncated: boolean }>(
        base(),
        '/v1/exec',
        getApiKey,
        {
          method: 'POST',
          body: JSON.stringify({
            argv: opts.argv,
            workspace: opts.workspace || undefined,
            workdir: opts.workdir || undefined,
            timeout_ms: opts.timeoutMs || undefined,
          }),
        },
      )
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
