/**
 * tidychat — Canvas minimap layout + hit-testing (pure geometry).
 *
 * A weight-based fish-eye mapping: turns near the hover point (±R) are spread
 * apart (boosted), far ones compress. `layoutPositions` and `indexFromY`
 * share the same function so the drawn bars and the hit test stay in sync.
 */

import {
  NAV_RAIL_BAR_H,
  NAV_RAIL_FISH_EYE_BOOST,
  NAV_RAIL_FISH_EYE_RADIUS,
  NAV_RAIL_MAX_HEIGHT,
  NAV_RAIL_MIN_HEIGHT,
  NAV_RAIL_TURN_SPACING,
} from './config'

/** Adaptive rail height: tighten for few turns, cap at min(70vh, 660px). */
export function railHeight(n: number, viewportHeight: number): number {
  const cap = Math.min(viewportHeight * 0.7, NAV_RAIL_MAX_HEIGHT)
  return Math.min(cap, Math.max(NAV_RAIL_MIN_HEIGHT, n * NAV_RAIL_TURN_SPACING))
}

/** Centre-y (CSS px) of each turn, fish-eye-weighted relative to hoverIdx. */
export function layoutPositions(n: number, hoverIdx: number | null, H: number): number[] {
  if (n <= 0) return []
  const weights: number[] = []
  for (let i = 0; i < n; i++) {
    let w = 1
    if (hoverIdx !== null) {
      const d = Math.abs(i - hoverIdx)
      if (d <= NAV_RAIL_FISH_EYE_RADIUS) {
        w = 1 + (NAV_RAIL_FISH_EYE_RADIUS - d + 1) * NAV_RAIL_FISH_EYE_BOOST
      }
    }
    weights.push(w)
  }
  const total = weights.reduce((a, b) => a + b, 0)
  const usable = Math.max(H - NAV_RAIL_BAR_H, 1)
  const pos: number[] = []
  let acc = 0
  for (let i = 0; i < n; i++) {
    acc += weights[i]
    pos.push(((acc - weights[i] / 2) / total) * usable + NAV_RAIL_BAR_H / 2)
  }
  return pos
}

/** Nearest turn index to a y offset; positions must be monotonically sorted. */
export function indexFromY(y: number, positions: number[]): number {
  if (positions.length === 0) return 0
  let lo = 0
  let hi = positions.length - 1
  while (lo < hi) {
    const mid = (lo + hi) >> 1
    if (positions[mid] < y) lo = mid + 1
    else hi = mid
  }
  const cur = positions[lo]
  const prev = lo > 0 ? positions[lo - 1] : -Infinity
  const candidate = Math.abs(cur - y) <= Math.abs(prev - y) ? lo : lo - 1
  return Math.max(0, Math.min(positions.length - 1, candidate))
}