import type { Transport } from '../api'
import type { QueueOpBody, QueueItem, RunJob } from './types'

/**
 * Thin, mockable wrapper over the Phase-1 backend traffic endpoints.
 * The store only ever talks to the backend through this interface; when no
 * run id is available the store skips these calls entirely (silent local
 * degradation), so no other module needs to know about the transport.
 */
export interface TrafficApi {
  queue(runId: string, op: QueueOpBody): Promise<{ queue: QueueItem[]; count: number }>
  jobs(runId: string): Promise<RunJob[]>
}

/** REST-backed implementation over the shared transport. */
export const createTrafficApi = (t: Transport): TrafficApi => ({
  async queue(runId, op) {
    return t.request<{ queue: QueueItem[]; count: number }>('PATCH', `/runs/${runId}/queue`, op)
  },
  async jobs(runId) {
    const res = await t.request<{ jobs: RunJob[] }>('GET', `/runs/${runId}/jobs`)
    return res.jobs
  },
})