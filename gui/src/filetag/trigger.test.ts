import { describe, expect, it } from 'vitest'
import { detectMention, MENTION_CHAR, matchCandidates } from './trigger'
import type { Candidate } from './trigger'

describe('detectMention', () => {
  it('is inactive with no @ before cursor', () => {
    expect(detectMention('hello world', 5).active).toBe(false)
  })
  it('activates on @ at word start', () => {
    const r = detectMention('read @src/ma', 12)
    expect(r.active).toBe(true)
    expect(r.query).toBe('src/ma')
    expect(r.start).toBe(5)
  })
  it('ignores @ embedded in a token (no word boundary)', () => {
    expect(detectMention('foo@bar', 7).active).toBe(false)
  })
  it('returns empty query for a bare @', () => {
    const r = detectMention('read @', 6)
    expect(r.active).toBe(true)
    expect(r.query).toBe('')
  })
  it('clamps cursor to text bounds', () => {
    const r = detectMention('@a', 99)
    expect(r.active).toBe(true)
    expect(r.query).toBe('a')
  })
})

describe('matchCandidates', () => {
  const cs: Candidate[] = [
    { id: '1', label: 'src/main.go', kind: 'file' },
    { id: '2', label: 'src/util.go', kind: 'file' },
    { id: '3', label: 'README.md', kind: 'file' },
  ]
  it('prefix/contains filters case-insensitively', () => {
    const m = matchCandidates(cs, 'SRC')
    expect(m).toHaveLength(2)
  })
  it('empty query returns all', () => {
    expect(matchCandidates(cs, '')).toHaveLength(3)
  })
})

describe('MENTION_CHAR', () => {
  it('is the @ trigger (spec)', () => {
    expect(MENTION_CHAR).toBe('@')
  })
})