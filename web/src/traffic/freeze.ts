import type { QueueTier } from './tiers'

/**
 * Freeze / resume planning (pure functions — spec Phase 3 Task 12).
 *
 * Freezing captures a per-session `FreezeRecord` so the state is isolated by
 * session. On resume the record is translated (via `planResume`) into a
 * deterministic list of steps that the store applies against chat:
 *
 * - `force` / `cancel` → cancel the (re)started run, then resend the held text.
 * - `safe_point`      → steer the captured items back into the queue (no send).
 * - `queue`           → send each captured queued item directly.
 */

export type FreezeMode = 'force' | 'cancel' | 'safe_point' | 'queue'

/** A single queued item snapshot carried by a freeze record. */
export interface FreezeItem {
  id: string
  tier: QueueTier
  text: string
}

export interface FreezeRecord {
  sessionId: string
  mode: FreezeMode
  /** Held input text (force/cancel resume path). */
  text: string
  /** Snapshot of queued items at freeze time. */
  items: FreezeItem[]
  at: number
}

export type ResumeStep =
  | { action: 'cancel' }
  | { action: 'send'; text: string }
  | { action: 'steer'; item: FreezeItem }

/** Build a freeze record for a session (initialiser order is explicit). */
export function createFreeze(
  sessionId: string,
  mode: FreezeMode,
  text = '',
  items: FreezeItem[] = [],
  at = Date.now(),
): FreezeRecord {
  return { sessionId, mode, text, items, at }
}

/**
 * Translate a freeze record into the ordered list of resume operations.
 * Pure — never touches chat or the queue store itself.
 */
export function planResume(rec: FreezeRecord): ResumeStep[] {
  switch (rec.mode) {
    case 'force':
    case 'cancel': {
      const text = rec.text || rec.items[0]?.text || ''
      const steps: ResumeStep[] = [{ action: 'cancel' }]
      if (text) steps.push({ action: 'send', text })
      return steps
    }
    case 'safe_point':
      return rec.items.map((item) => ({ action: 'steer', item }))
    case 'queue':
      return rec.items.filter((i) => i.text).map((item) => ({ action: 'send', text: item.text }))
  }
}