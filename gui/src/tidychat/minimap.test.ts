import { describe, expect, it } from 'vitest'
import { indexFromY, layoutPositions, railHeight } from './minimap'

describe('railHeight', () => {
  it('tightens for few turns below the min', () => {
    // 3 turns never reaches the 48px min.
    expect(railHeight(3, 900)).toBe(48)
  })
  it('scales at 12px/turn up to the cap', () => {
    expect(railHeight(40, 900)).toBe(480)
    expect(railHeight(40, 700)).toBe(480) // 700*0.7=490 > 480
    expect(railHeight(40, 600)).toBe(420) // capped by 0.7*600
  })
  it('caps at min(70vh, 660)', () => {
    expect(railHeight(200, 2000)).toBe(660)
    expect(railHeight(200, 400)).toBe(280)
  })
})

describe('layoutPositions', () => {
  it('returns n positions within the rail height', () => {
    const pos = layoutPositions(5, null, 200)
    expect(pos).toHaveLength(5)
    for (const y of pos) expect(y).toBeGreaterThanOrEqual(0)
    for (const y of pos) expect(y).toBeLessThanOrEqual(200)
  })
  it('produces monotonically increasing centers', () => {
    const pos = layoutPositions(8, null, 300)
    for (let i = 1; i < pos.length; i++) expect(pos[i]).toBeGreaterThan(pos[i - 1])
  })
  it('spreads turns near the hover point (fish-eye boost)', () => {
    const base = layoutPositions(9, null, 360)
    const fish = layoutPositions(9, 4, 360)
    // The gap between turn 4 and 5 grows (both boosted near hover 4).
    const dist = (arr: number[], a: number, b: number) => Math.abs(arr[b] - arr[a])
    expect(dist(fish, 4, 5)).toBeGreaterThan(dist(base, 4, 5))
  })
  it('centers a single turn in the rail', () => {
    const pos = layoutPositions(1, null, 100)
    // Single turn lands at the vertical center: usable/2 + barH/2.
    expect(pos[0]).toBeCloseTo((100 - 3) / 2 + 1.5)
  })
})

describe('indexFromY', () => {
  it('maps a y offset to the nearest turn index', () => {
    const pos = layoutPositions(5, null, 200)
    // The midpoint should point at the middle turn.
    const mid = indexFromY(100, pos)
    expect(mid).toBeGreaterThan(0)
    expect(mid).toBeLessThan(4)
    // Clamp to the first / last index at the extremes.
    expect(indexFromY(-50, pos)).toBe(0)
    expect(indexFromY(500, pos)).toBe(4)
  })
  it('returns 0 for an empty layout', () => {
    expect(indexFromY(10, [])).toBe(0)
  })
  it('indexFromY is consistent with layoutPositions directions', () => {
    const pos = layoutPositions(4, null, 200)
    expect(indexFromY(pos[2], pos)).toBe(2)
  })
})