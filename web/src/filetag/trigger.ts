// @file trigger detection (Phase 5, Task 17) — pure text scanning.
// Finds an active `@` mention at a cursor position (backward scan with a word
// boundary) and yields the partial query the candidate menu should filter by.

/** Trigger characters: `@` for file/dir mention. (Spec: TriggerChar = '@'.) */
export const MENTION_CHAR = '@'

export interface TriggerHit {
  /** Anchored text trailing the cursor is part of the query. */
  active: boolean
  /** Query text after the '@' ('' when nothing typed yet). */
  query: string
  /** Index of the '@' character in the source. */
  start: number
  /** Cursor index (end of the partial token). */
  end: number
}

/** True when the char before `at` is a word boundary (space/start/punctuation). */
const BOUNDARY_CHARS = " \t/()[]{}'\"`,.:;=\\<>-"

function isWordBoundary(text: string, at: number): boolean {
  if (at <= 0) return true
  const before = text[at - 1]
  return /\s/.test(before) || BOUNDARY_CHARS.includes(before)
}

/**
 * Scan backward from `cursor` for a `@` mention. Returns an inactive hit when
 * the '@' is embedded in a longer token (e.g. `foo@bar`) so it isn't triggered.
 */
export function detectMention(text: string, cursor: number): TriggerHit {
  const pos = Math.max(0, Math.min(cursor, text.length))
  // Walk back from the cursor to the start of the current token.
  let i = pos
  while (i > 0 && !/[\s]/.test(text[i - 1])) i -= 1
  const tokenStart = i
  const token = text.slice(tokenStart, pos)
  const atIdx = token.lastIndexOf(MENTION_CHAR)
  if (atIdx === -1) {
    return { active: false, query: '', start: pos, end: pos }
  }
  const absStart = tokenStart + atIdx
  // `@` must sit at a word boundary to count as a mention trigger.
  if (!isWordBoundary(text, absStart)) {
    return { active: false, query: '', start: pos, end: pos }
  }
  const query = text.slice(absStart + 1, pos)
  return { active: true, query, start: absStart, end: pos }
}

export interface Candidate {
  id: string
  label: string
  kind: 'file' | 'dir' | 'skill'
  desc?: string
}

/** Case-insensitive prefix match against a query. */
export function matchCandidates(candidates: Candidate[], query: string): Candidate[] {
  const ql = query.trim().toLowerCase()
  if (!ql) return candidates
  return candidates.filter((c) => c.label.toLowerCase().includes(ql))
}