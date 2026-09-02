import { describe, expect, it } from 'vitest'
import { orderedQueue, tierRank, byTier, TIER_ORDER } from './tiers'
import type { TrafficEntry, QueueTier } from './tiers'

const entry = (id: string, tier: QueueTier, text: string): TrafficEntry => ({ id, tier, text })

describe('traffic tiers', () => {
  it('assigns priority now > next > later', () => {
    expect(TIER_ORDER).toEqual({ later: 0, next: 1, now: 2 })
    expect(tierRank('now')).toBeGreaterThan(tierRank('next'))
    expect(tierRank('next')).toBeGreaterThan(tierRank('later'))
    expect(byTier('later', 'now')).toBeLessThan(0)
    expect(byTier('now', 'later')).toBeGreaterThan(0)
    expect(byTier('next', 'next')).toBe(0)
  })

  it('orders a mixed queue by tier priority', () => {
    const q = [
      entry('n1', 'now', 'A'),
      entry('l1', 'later', 'B'),
      entry('x1', 'next', 'C'),
      entry('l2', 'later', 'D'),
      entry('n2', 'now', 'E'),
    ]
    const sorted = orderedQueue(q)
    expect(sorted.map((i) => i.id)).toEqual(['n1', 'n2', 'x1', 'l1', 'l2'])
  })

  it('keeps FIFO order within the same tier', () => {
    const q = [entry('a', 'later', '1'), entry('b', 'later', '2'), entry('c', 'later', '3')]
    expect(orderedQueue(q).map((i) => i.id)).toEqual(['a', 'b', 'c'])
  })

  it('does not mutate the input array', () => {
    const q = [entry('a', 'now', '1')]
    orderedQueue(q)
    expect(q).toHaveLength(1)
    expect(q[0].id).toBe('a')
  })
})