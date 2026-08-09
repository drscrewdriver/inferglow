import { create } from 'zustand'
import { transport, type SessionRecord, type Transport } from '../api'

export interface SessionPatch {
  title?: string
  group?: string
  pinned?: boolean
  status?: string
}

interface SessionState {
  sessions: SessionRecord[]
  activeId: string | null
  loading: boolean
  error: string | null
  fetchSessions: () => Promise<void>
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

    async fetchSessions() {
      set({ loading: true, error: null })
      try {
        const sessions = await t.request<SessionRecord[]>('GET', '/sessions')
        set({ sessions, loading: false })
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
