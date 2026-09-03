/* input-traffic 插件 — 后端流量队列 / 任务的 API 契约（基于 webui transport） */

import { get, patch } from '../../api/transport'

/** 三层投放优先级队列层级。较高级优先排空：now > next > later。 */
export type QueueTier = 'later' | 'next' | 'now'

/** 后端队列条目（PATCH /v1/runs/{id}/queue）。 */
export interface QueueItem {
  id: string
  run_id: string
  tier: QueueTier
  text: string
  session_id?: string
  created_at: string
  updated_at: string
}

/** PATCH /v1/runs/{id}/queue 请求体 */
export interface QueueOpBody {
  kind: 'push' | 'edit' | 'remove' | 'steer' | 'clear'
  item_id?: string
  tier?: QueueTier
  text?: string
  to_front?: boolean
}

/** 后端后台任务（GET /v1/runs/{id}/jobs）。 */
export interface RunJob {
  id: string
  run_id: string
  kind: string
  status: 'ongoing' | 'stopping' | 'completed' | 'killed' | 'failed'
  description?: string
  started_at: string | null
  finished_at?: string | null
  duration?: number | null
  error?: string | null
}

export interface TrafficApi {
  queue(runId: string, op: QueueOpBody): Promise<{ queue: QueueItem[]; count: number }>
  jobs(runId: string): Promise<RunJob[]>
}

/** REST 实现，直接使用 webui 的 transport（同源 BASE=''）。 */
export const trafficApi: TrafficApi = {
  queue: (runId, op) => patch<{ queue: QueueItem[]; count: number }>(`/v1/runs/${runId}/queue`, op),
  jobs: async (runId) => {
    const res = await get<{ jobs: RunJob[] }>(`/v1/runs/${runId}/jobs`)
    return res.jobs
  },
}