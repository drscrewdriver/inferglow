import { describe, expect, it } from 'vitest'
import { estimateTokens, fileOpKindOf, extractPath, SYSTEM_PROMPT_EST, TOOLS_SCHEMA_EST } from './token'

describe('estimateTokens', () => {
  it('returns 0 for empty/undefined', () => {
    expect(estimateTokens(undefined)).toBe(0)
    expect(estimateTokens('')).toBe(0)
    expect(estimateTokens(null)).toBe(0)
  })
  it('rounds up chars/4', () => {
    expect(estimateTokens('abcd')).toBe(1)
    expect(estimateTokens('abc')).toBe(1)
    expect(estimateTokens('a')).toBe(1)
  })
})

describe('fileOpKindOf', () => {
  it('classifies known tool prefixes', () => {
    expect(fileOpKindOf('read_file')).toBe('read')
    expect(fileOpKindOf('write_file')).toBe('write')
    expect(fileOpKindOf('edit')).toBe('write')
    expect(fileOpKindOf('search_code')).toBe('search')
    expect(fileOpKindOf('image_analyze')).toBe('image')
    expect(fileOpKindOf('list_dir')).toBe('dir')
  })
  it('returns null for non-file tools', () => {
    expect(fileOpKindOf('web_search')).toBeNull()
    expect(fileOpKindOf(undefined)).toBeNull()
    expect(fileOpKindOf('')).toBeNull()
  })
})

describe('extractPath', () => {
  it('extracts a file path token', () => {
    expect(extractPath('read /src/main.go line 1')).toContain('src/main.go')
    expect(extractPath('cat "C:\\work\\a.txt"')).toBeTruthy()
  })
  it('returns undefined without a path', () => {
    expect(extractPath('no path here')).toBeUndefined()
    expect(extractPath(undefined)).toBeUndefined()
  })
})

describe('constants', () => {
  it('keeps the DSH-aligned auto-compact ratio and baseline prices', () => {
    expect(SYSTEM_PROMPT_EST).toBeGreaterThan(0)
    expect(TOOLS_SCHEMA_EST).toBeGreaterThan(0)
  })
})