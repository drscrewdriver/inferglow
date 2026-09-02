import type { Transport } from '../api'
import type { RunRecord, RunStep } from '../api'

/**
 * Thin, mockable wrapper over the backend run/steps endpoints used by the
 * 待办清单 panel. Silent-degrades to an empty panel when unavailable
 * (mirrors trafficApi).
 */
export interface TodoApi {
  listRuns(): Promise<RunRecord[]>
  steps(runId: string): Promise<RunStep[]>
}

/** REST-backed implementation over the shared transport. */
export const createTodoApi = (t: Transport): TodoApi => ({
  async listRuns() {
    const res = await t.request<RunRecord[]>('GET', '/runs')
    return Array.isArray(res) ? res : []
  },
  async steps(runId) {
    const res = await t.request<{ run_id: string; steps: RunStep[] }>(
      'GET',
      `/runs/${encodeURIComponent(runId)}/steps`,
    )
    return res.steps ?? []
  },
})
