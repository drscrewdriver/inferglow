import type { Transport } from '../api'
import type { MemoryListResult, MemoryRecord } from '../api'

/**
 * Thin, mockable wrapper over the backend memory endpoints.
 * The store only ever talks to the backend through this interface; on any
 * failure it silently degrades to an empty panel, so no other module needs to
 * know about the transport (mirrors trafficApi).
 */
export interface MemoryApi {
  list(limit?: number, q?: string): Promise<MemoryRecord[]>
  remove(id: string): Promise<void>
}

/** REST-backed implementation over the shared transport. */
export const createMemoryApi = (t: Transport): MemoryApi => ({
  async list(limit = 20, q) {
    const qs = new URLSearchParams({ limit: String(limit) })
    if (q) qs.set('q', q)
    const res = await t.request<MemoryListResult>('GET', `/memories?${qs.toString()}`)
    return res.memories ?? []
  },
  async remove(id) {
    await t.request<{ status: string }>('DELETE', `/memories/${encodeURIComponent(id)}`)
  },
})
