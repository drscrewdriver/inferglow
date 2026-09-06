import { describe, expect, it } from 'vitest'
import { CONSECUTIVE_SLOW_LIMIT, HARD_BUDGET_MS, SOFT_BUDGET_MS } from './config'
import { createGovernorState, decideGovernor, type GovernorState } from './governor'

const idle = (): GovernorState => createGovernorState('idle')

describe('decideGovernor', () => {
  it('continues on a fast, growing batch with more history', () => {
    const r = decideGovernor({ scanMs: 5, grew: true, stillHasMore: true, prev: idle() })
    expect(r.decision).toBe('continue')
    expect(r.next.status).toBe('idle')
    expect(r.next.consecutiveSlow).toBe(0)
  })

  it('pauses when a batch grew nothing but more history remains', () => {
    const r = decideGovernor({ scanMs: 5, grew: false, stillHasMore: true, prev: idle() })
    expect(r.decision).toBe('pause')
    expect(r.next.status).toBe('paused')
  })

  it('pauses on any batch above the hard budget', () => {
    const r = decideGovernor({ scanMs: HARD_BUDGET_MS + 1, grew: true, stillHasMore: true, prev: idle() })
    expect(r.decision).toBe('pause')
  })

  it('pauses after the consecutive slow limit', () => {
    const prev: GovernorState = { status: 'idle', consecutiveSlow: CONSECUTIVE_SLOW_LIMIT - 1 }
    const r = decideGovernor({ scanMs: SOFT_BUDGET_MS + 1, grew: true, stillHasMore: true, prev })
    expect(r.decision).toBe('pause')
  })

  it('resets the slow counter on a fast batch', () => {
    const prev: GovernorState = { status: 'idle', consecutiveSlow: 2 }
    const r = decideGovernor({ scanMs: 5, grew: true, stillHasMore: true, prev })
    expect(r.next.consecutiveSlow).toBe(0)
    expect(r.decision).toBe('continue')
  })

  it('marks done when it reaches the front cleanly', () => {
    const r = decideGovernor({ scanMs: 5, grew: true, stillHasMore: false, prev: idle() })
    expect(r.decision).toBe('done')
    expect(r.next.status).toBe('done')
  })

  it('forces pause on timeout regardless of growth', () => {
    const r = decideGovernor({ scanMs: 2, grew: true, stillHasMore: true, timeout: true, prev: idle() })
    expect(r.decision).toBe('pause')
  })
})