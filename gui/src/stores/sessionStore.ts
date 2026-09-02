import { create } from 'zustand'
import { transport, type SessionRecord, type Transport } from '../api'

export interface SessionPatch {
  title?: string
  group?: string
  pinned?: boolean
  status?: string
}

/** Backend filter params for GET /sessions. Optional—when omitted the call
 * stays the plain `GET /sessions` that longer-running tests rely on. */
export interface SessionQuery {
  q?: string
  group?: string
  pinned?: boolean
}

/** Serialize a query into a `?k=v&...` suffix, or '' when nothing is set. */
export function buildSessionQuery(query?: SessionQuery): string {
  if (!query) return ''
  const params = new URLSearchParams()
  if (query.q) params.set('q', query.q)
  if (query.group) params.set('group', query.group)
  if (query.pinned !== undefined) params.set('pinned', String(query.pinned))
  const s = params.toString()
  return s ? `?${s}` : ''
}

/** Wire contract for GET /sessions: the list rides under `sessions` alongside
 * the backend's count/groups/query envelope (keys absent when unset). */
export interface SessionListResponse {
  sessions: SessionRecord[]
  count?: number
  groups?: Record<string, number>
  query?: Record<string, unknown>
}

interface SessionState {
  sessions: SessionRecord[]
  activeId: string | null
  loading: boolean
  error: string | null
  fetchSessions: (query?: SessionQuery) => Promise<void>
  createSession: (agentId: string, title?: string) => Promise<SessionRecord | null>
  updateMeta: (id: string, patch: SessionPatch) => Promise<void>
  removeSession: (id: string) => Promise<void>
  select: (id: string | null) => void
}

/** Factory so tests can inject a mock Transport; the app uses the default. */
export const createSessionStore = (t: Transport) =>
  create<SessionState>()((set, get) => ({
    sessions: [],
    activeId: null,
    loading: false,
    error: null,

    async fetchSessions(query) {
      set({ loading: true, error: null })
      try {
        const resp = await t.request<SessionListResponse>('GET', `/sessions${buildSessionQuery(query)}`)
        set({ sessions: resp.sessions, loading: false })
      } catch (err) {
        set({ loading: false, error: err instanceof Error ? err.message : String(err) })
      }
    },

    async createSession(agentId, title) {
      try {
        const rec = await t.request<SessionRecord>('POST', '/sessions', { agent_id: agentId, title })
        set({ sessions: [...get().sessions, rec] })
        return rec
      } catch (err) {
        set({ error: err instanceof Error ? err.message : String(err) })
        return null
      }
    },

    async updateMeta(id, patch) {
      const updated = await t.request<SessionRecord>('PATCH', `/sessions/${id}`, patch)
      set({ sessions: get().sessions.map((s) => (s.id === id ? updated : s)) })
    },

    async removeSession(id) {
      await t.request<{ status: string }>('DELETE', `/sessions/${id}`)
      const next = get().sessions.filter((s) => s.id !== id)
      set({ sessions: next, activeId: get().activeId === id ? null : get().activeId })
    },

    select(id) {
      set({ activeId: id })
    },
  }))

export const useSessionStore = createSessionStore(transport)
