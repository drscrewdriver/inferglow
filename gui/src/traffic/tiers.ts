/**
 * Traffic queue tier semantics (pure functions — spec Phase 3 Task 11).
 *
 * Three scheduling tiers with strict priority order:
 * - 🟢 `later` — sent only after the current turn completes naturally.
 * - 🟡 `next`  — injected and sent right after the current action ends.
 * - 🔴 `now`   — cancels the current run and sends immediately.
 *
 * Priority when draining is `now > next > later`; within the same tier the
 * queue stays FIFO (stable sort preserves insertion order).
 */

export type QueueTier = 'later' | 'next' | 'now'

/** Ascending rank; higher drains first. now=2 > next=1 > later=0. */
export const TIER_ORDER: Record<QueueTier, number> = { later: 0, next: 1, now: 2 }

export const TIER_LABEL: Record<QueueTier, string> = { later: '稍后', next: '下一步', now: '立即' }
export const TIER_ICON: Record<QueueTier, string> = { later: '🟢', next: '🟡', now: '🔴' }

/** All tiers in ascending priority order (later, next, now). */
export const TIERS: QueueTier[] = ['later', 'next', 'now']

/** Numeric rank of a single tier. */
export function tierRank(tier: QueueTier): number {
  return TIER_ORDER[tier]
}

/** Comparator over the two tiers; negative when `a` should drain first. */
export function byTier(a: QueueTier, b: QueueTier): number {
  return tierRank(a) - tierRank(b)
}

/** Minimal shape a queue entry needs to be priority-sorted. */
export interface TrafficEntry {
  id: string
  tier: QueueTier
  text: string
}

/**
 * Return a copy of `items` ordered by tier priority (now → next → later);
 * the sort is stable so items sharing a tier keep their original (FIFO) order.
 */
export function orderedQueue<T extends TrafficEntry>(items: T[]): T[] {
  return [...items].sort((a, b) => tierRank(b.tier) - tierRank(a.tier))
}