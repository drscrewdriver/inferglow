/**
 * tidychat — Smart auto-load governor decision (pure reducer).
 *
 * Mirrors the dsh-tidychat `finish()` precedence exactly: a batch that loaded
 * nothing while more history remains pauses; a hard-budget scan pauses;
 * repeated slow scans pause; reaching the front cleanly marks done; otherwise
 * keep going. The component owns the wall-clock of a single measured scan and
 * feeds the numbers in here.
 */

import { CONSECUTIVE_SLOW_LIMIT, HARD_BUDGET_MS, SOFT_BUDGET_MS } from './config'

export type GovernorStatus = 'idle' | 'loading' | 'settling' | 'paused' | 'done'
export type GovernorDecision = 'continue' | 'pause' | 'done'

export interface GovernorState {
  status: GovernorStatus
  consecutiveSlow: number
}

export function createGovernorState(status: GovernorStatus = 'idle'): GovernorState {
  return { status, consecutiveSlow: 0 }
}

export interface GovernorMeasure {
  scanMs: number
  /** Whether this batch actually grew the conversation window. */
  grew: boolean
  /** Whether a "load older" control is still present (more history pending). */
  stillHasMore: boolean
  /** Force-pause (settle timeout / no growth with button still present). */
  timeout?: boolean
  prev: GovernorState
}

export interface GovernorResult {
  decision: GovernorDecision
  next: GovernorState
}

/** Single-batch governor transition (idempotent, no side effects). */
export function decideGovernor(m: GovernorMeasure): GovernorResult {
  let consecutiveSlow = m.prev.consecutiveSlow

  if (m.timeout === true || (!m.grew && m.stillHasMore)) {
    return { decision: 'pause', next: { status: 'paused', consecutiveSlow } }
  }

  if (m.scanMs >= HARD_BUDGET_MS) {
    return { decision: 'pause', next: { status: 'paused', consecutiveSlow } }
  }

  if (m.scanMs >= SOFT_BUDGET_MS) {
    consecutiveSlow += 1
    if (consecutiveSlow >= CONSECUTIVE_SLOW_LIMIT) {
      return { decision: 'pause', next: { status: 'paused', consecutiveSlow } }
    }
  } else {
    consecutiveSlow = 0
  }

  if (m.grew && !m.stillHasMore) {
    return { decision: 'done', next: { status: 'done', consecutiveSlow } }
  }

  return { decision: 'continue', next: { status: 'idle', consecutiveSlow } }
}