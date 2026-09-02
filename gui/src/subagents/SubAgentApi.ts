import type { Transport } from '../api'
import type { AgentListResult, AgentRecord } from '../api'

/**
 * Thin, mockable wrapper over the backend agent endpoints used by the
 * 子智能体 panel. Silent-degrades to an empty panel when unavailable
 * (mirrors trafficApi).
 */
export interface SubAgentApi {
  list(): Promise<AgentRecord[]>
}

/** REST-backed implementation over the shared transport. */
export const createSubAgentApi = (t: Transport): SubAgentApi => ({
  async list() {
    const res = await t.request<AgentListResult>('GET', '/agents')
    return res.agents ?? []
  },
})
