import { describe, expect, it } from 'vitest'
import { createFreeze, planResume } from './freeze'

const mk = () =>
  createFreeze('s1', 'queue', '', [
    { id: 'a', tier: 'next', text: '任务一' },
    { id: 'b', tier: 'later', text: '任务二' },
  ])

describe('freeze resume planning', () => {
  it('force → cancel then resend the held text', () => {
    const rec = createFreeze('s1', 'force', '待发送内容', [])
    expect(planResume(rec)).toEqual([
      { action: 'cancel' },
      { action: 'send', text: '待发送内容' },
    ])
  })

  it('cancel → cancel plus send (falls back to first captured item)', () => {
    const rec = createFreeze('s1', 'cancel', '', [{ id: 'x', tier: 'next', text: '兜底文本' }])
    expect(planResume(rec)).toEqual([
      { action: 'cancel' },
      { action: 'send', text: '兜底文本' },
    ])
  })

  it('cancel with no payload yields only a cancel step', () => {
    const rec = createFreeze('s1', 'cancel', '', [])
    expect(planResume(rec)).toEqual([{ action: 'cancel' }])
  })

  it('safe_point → steer every item back into the queue', () => {
    const rec = createFreeze('s1', 'safe_point', '', [
      { id: 'a', tier: 'next', text: '一' },
      { id: 'b', tier: 'later', text: '二' },
    ])
    expect(planResume(rec)).toEqual([
      { action: 'steer', item: { id: 'a', tier: 'next', text: '一' } },
      { action: 'steer', item: { id: 'b', tier: 'later', text: '二' } },
    ])
  })

  it('queue → send each captured item directly', () => {
    const rec = createFreeze('s1', 'queue', '', [
      { id: 'a', tier: 'next', text: '一' },
      { id: 'b', tier: 'later', text: '二' },
    ])
    expect(planResume(rec)).toEqual([
      { action: 'send', text: '一' },
      { action: 'send', text: '二' },
    ])
  })

  it('queue ignores blank items', () => {
    const rec = createFreeze('s1', 'queue', '', [
      { id: 'a', tier: 'next', text: '一' },
      { id: 'b', tier: 'later', text: '' },
    ])
    expect(planResume(rec)).toEqual([{ action: 'send', text: '一' }])
  })
})

describe('freeze record isolation', () => {
  it('records the owning session and freeze timestamp', () => {
    const rec = createFreeze('session-9', 'safe_point', '', [], 12345)
    expect(rec.sessionId).toBe('session-9')
    expect(rec.at).toBe(12345)
    expect(mk().sessionId).toBe('s1')
  })
})