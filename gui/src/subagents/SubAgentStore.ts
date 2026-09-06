import { create } from 'zustand'
import { transport } from '../api'
import type { AgentRecord } from '../api'
import { createSubAgentApi, type SubAgentApi } from './SubAgentApi'

export interface SubAgentState {
  agents: AgentRecord[]
  loading: boolean
  error: string | null
  /** Re-fetch the agent list from the backend. */
  refresh: () => Promise<void>
}

export function createSubAgentStore(api: SubAgentApi) {
  return create<SubAgentState>()((set) => ({
    agents: [],
    loading: false,
    error: null,

    async refresh() {
      set({ loading: true, error: null })
      try {
        const agents = await api.list()
        set({ agents, loading: false })
      } catch (e) {
        // silent degradation: panel falls back to the empty state
        set({ agents: [], loading: false, error: e instanceof Error ? e.message : '加载失败' })
      }
    },
  }))
}

/** Default app instance wired to the shared transport. */
export const useSubAgentStore = createSubAgentStore(createSubAgentApi(transport))
