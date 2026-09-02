import type { QueueTier } from './tiers'

/** Backend contract queue entry (Phase 1 — PATCH /v1/runs/{runId}/queue). */
export interface QueueItem {
  id: string
  run_id: string
  tier: QueueTier
  text: string
  session_id?: string
  created_at: string
  updated_at: string
}

/** Body of a PATCH /v1/runs/{runId}/queue request. */
export interface QueueOpBody {
  kind: 'push' | 'edit' | 'remove' | 'steer' | 'clear'
  item_id?: string
  tier?: QueueTier
  text?: string
  to_front?: boolean
}

/** Backend background job (GET /v1/runs/{runId}/jobs). */
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