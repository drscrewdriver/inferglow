import { describe, expect, it } from 'vitest'
import {
  decideEffort,
  heaviestArg,
  isSimpleCall,
  simpleRatio,
  TOOL_WINDOW,
  type ThinkingLevelsConfig,
  type ToolCallSample,
} from './decide'

const AUTO: ThinkingLevelsConfig = { enabled: true, level: 'auto', allowDowngrade: true, allowUpgrade: false }

const simple = (argSize = 40): ToolCallSample => ({ name: 'read', argSize })
const heavy = (argSize = HEAVY_ARGS_BIG): ToolCallSample => ({ name: 'write', argSize: argSize })
const HEAVY_ARGS_BIG = 5000

describe('simpleRatio', () => {
  it('returns 1 for empty window', () => {
    expect(simpleRatio([])).toBe(1)
  })
  it('computes the simple fraction', () => {
    expect(simpleRatio([simple(), simple(), heavy()])).toBeCloseTo(2 / 3)
  })
})

describe('heaviestArg', () => {
  it('returns 0 for empty window', () => {
    expect(heaviestArg([])).toBe(0)
  })
  it('finds the largest argument', () => {
    expect(heaviestArg([simple(10), simple(900), simple(3)])).toBe(900)
  })
})

describe('isSimpleCall', () => {
  it('treats a small arg as simple', () => {
    expect(isSimpleCall('read', 799)).toBe(true)
  })
  it('treats an arg at/over HEAVY_ARGS as heavy', () => {
    expect(isSimpleCall('read', 800)).toBe(false)
  })
})

describe('decideEffort — fixed levels', () => {
  it('returns high when disabled', () => {
    expect(decideEffort({ ...AUTO, enabled: false }, [])).toBe('high')
  })

  const fixed: ThinkingLevelsConfig[] = [
    { enabled: true, level: 'low', allowDowngrade: true, allowUpgrade: true },
    { enabled: true, level: 'medium', allowDowngrade: true, allowUpgrade: true },
    { enabled: true, level: 'high', allowDowngrade: true, allowUpgrade: true },
    { enabled: true, level: 'max', allowDowngrade: true, allowUpgrade: true },
  ]
  const expectations = ['low', 'medium', 'high', 'max']

  it('returns the configured level for fixed non-auto levels', () => {
    fixed.forEach((c, i) => {
      expect(decideEffort(c, [])).toBe(expectations[i])
    })
  })
})

describe('decideEffort — auto scheduler', () => {
  it('empty window: downgrade→low when allowed', () => {
    expect(decideEffort(AUTO, [])).toBe('low')
  })
  it('empty window: high when downgrade disallowed', () => {
    expect(decideEffort({ ...AUTO, allowDowngrade: false }, [])).toBe('high')
  })

  it('≥75% simple → low', () => {
    expect(decideEffort(AUTO, [simple(), simple(), simple(), heavy()])).toBe('low')
  })
  it('mixed <75% simple → high', () => {
    expect(decideEffort(AUTO, [simple(), heavy(), heavy(), simple()])).toBe('high')
  })

  it('huge arg upgrades to max only when allowed', () => {
    expect(decideEffort(AUTO, [heavy(4000)])).toBe('high') // allowUpgrade=false
    expect(decideEffort({ ...AUTO, allowUpgrade: true }, [heavy(4000)])).toBe('max')
  })
  it('single simple call stays low when upgrade off', () => {
    expect(decideEffort({ ...AUTO, allowUpgrade: true }, [simple(10)])).toBe('low')
  })

  it('respects the sliding window (drops old calls)', () => {
    const many = Array.from({ length: TOOL_WINDOW * 2 }, () => heavy(9000))
    expect(decideEffort({ ...AUTO, allowUpgrade: true }, many)).toBe('max')
  })
})