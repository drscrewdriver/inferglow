import { describe, expect, it } from 'vitest'
import { PLACEHOLDER, buildOccurrences, insertChip, serialize } from './chips'
import type { Chip } from './chips'

const chip: Chip = { id: 'c1', label: 'src/main.go', kind: 'file', path: '/src/main.go' }
const dirChip: Chip = { id: 'c2', label: 'config', kind: 'dir', path: '/config' }

describe('buildOccurrences', () => {
  it('finds each placeholder offset', () => {
    const occ = buildOccurrences(`a${PLACEHOLDER}b${PLACEHOLDER}c`)
    expect(occ.map((o) => o.offset)).toEqual([1, 3])
  })
})

describe('insertChip', () => {
  it('replaces a span with a placeholder', () => {
    const { text, chip: c } = insertChip('read @src/ma now', 5, 12, chip)
    expect(text).toBe(`read ${PLACEHOLDER} now`)
    expect(c.id).toBe('c1')
  })
})

describe('serialize', () => {
  it('replaces placeholders with file references in order', () => {
    const text = `look at ${PLACEHOLDER} and ${PLACEHOLDER}`
    const r = serialize(text, [chip, dirChip])
    expect(r.serialized).toBe('look at src/main.go and dir:config')
    expect(r.count).toBe(2)
  })
  it('returns text unchanged when no placeholders', () => {
    const r = serialize('plain text', [chip])
    expect(r.serialized).toBe('plain text')
    expect(r.count).toBe(0)
  })
})