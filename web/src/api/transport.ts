import type { ToolStreamEvent } from './types'

/**
 * Transport abstraction: the GUI talks to the backend exclusively through this
 * interface. The default implementation is REST over HTTP (fetch + SSE).
 *
 * A future Wails desktop shell (OT-10) can plug in a second implementation
 * that calls window.go.desktop.* bindings and subscribes via
 * runtime.EventsOn — components and stores only ever see `Transport`.
 */
export interface StreamRunHandlers {
  onEvent: (ev: ToolStreamEvent) => void
  onDone?: (agentId: string) => void
  onError?: (message: string) => void
}

export interface Transport {
  /** JSON request against the API root (path is relative to /v1). */
  request<T>(method: string, path: string, body?: unknown): Promise<T>
  /** Stream an agent run via SSE, dispatching parsed events to handlers. */
  streamRun(
    agentId: string,
    req: { message: string; session_id?: string },
    handlers: StreamRunHandlers,
    signal: AbortSignal,
  ): Promise<void>
}

const API_BASE = '/v1'

async function parseJson<T>(resp: Response): Promise<T> {
  if (!resp.ok) {
    let message = `HTTP ${resp.status}`
    try {
      const body = (await resp.json()) as { error?: string }
      if (body.error) message = body.error
    } catch {
      // non-JSON error body
    }
    throw new Error(message)
  }
  return (await resp.json()) as T
}

/** REST transport: fetch-based JSON requests + ReadableStream SSE parsing. */
export const restTransport: Transport = {
  async request<T>(method: string, path: string, body?: unknown): Promise<T> {
    const resp = await fetch(API_BASE + path, {
      method,
      headers: body !== undefined ? { 'Content-Type': 'application/json' } : undefined,
      body: body !== undefined ? JSON.stringify(body) : undefined,
    })
    return parseJson<T>(resp)
  },

  async streamRun(agentId, req, handlers, signal): Promise<void> {
    const resp = await fetch(`${API_BASE}/agents/${agentId}/stream-run`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
      signal,
    })
    if (!resp.ok) {
      let message = `HTTP ${resp.status}`
      try {
        const body = (await resp.json()) as { error?: string }
        if (body.error) message = body.error
      } catch {
        // ignore
      }
      handlers.onError?.(message)
      return
    }
    const { parseSSE } = await import('./sse')
    await parseSSE(resp, (e) => {
      if (e.event === 'done') {
        handlers.onDone?.(e.data ? (JSON.parse(e.data) as { agent_id?: string }).agent_id ?? agentId : agentId)
        return
      }
      try {
        handlers.onEvent(JSON.parse(e.data) as ToolStreamEvent)
      } catch {
        // malformed payload — skip
      }
    }, signal)
  },
}

// Future Wails transport (declared, not implemented in this version): would
// wrap window.go.desktop.StartSession / SendChat / GetStatus bindings and
// subscribe via runtime.EventsOn("session:event", cb). detectTransport picks
// it at runtime when window.go?.desktop exists.

/** Picks the transport at runtime: REST in browsers; Wails bindings when the
 * desktop shell exposes window.go (future). */
export function detectTransport(): Transport {
  // In a Wails WebView, window.go.desktop exists and would select a
  // wailsTransport implementation here. Browsers always use REST.
  return restTransport
}
