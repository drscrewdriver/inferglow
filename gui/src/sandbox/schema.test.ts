import { describe, expect, it } from 'vitest'
import { isRiskier, modeInfo, type SandboxMode } from './schema'

describe('modeInfo', () => {
  it('returns local info by default for unknown keys', () => {
    expect(modeInfo('local' as SandboxMode).key).toBe('local')
  })
  it('finds a known mode', () => {
    expect(modeInfo('gvisor').risk).toBe('safe')
  })
})

describe('isRiskier', () => {
  it('same mode is not riskier', () => {
    expect(isRiskier('docker', 'docker')).toBe(false)
  })
  it('safer → riskier is a downgrade', () => {
    expect(isRiskier('docker', 'trusted_local')).toBe(true) // safe → high
    expect(isRiskier('docker', 'local')).toBe(true) // safe → medium
  })
  it('riskier → safer is not a downgrade', () => {
    expect(isRiskier('trusted_local', 'gvisor')).toBe(false)
    expect(isRiskier('local', 'gvisor')).toBe(false)
  })
})