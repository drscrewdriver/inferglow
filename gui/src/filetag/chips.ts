// Chip model + codec for @file (Phase 5, Task 18): the editor holds a single
// text string in which each chip is represented by a U+FFFC placeholder. An
// occurrence table maps each placeholder offset to a chip record; the codec
// serializes placeholders back into the file references the harness sends.

/** U+FFFC — OBJECT REPLACEMENT CHARACTER: the single character a chip occupies
 * in the underlying text so caret/keyboard metrics stay 1:1 with the editor. */
export const PLACEHOLDER = '\uFFFC'

export interface Chip {
  id: string
  label: string
  kind: 'file' | 'dir' | 'skill'
  path: string
  /** Marked when the referenced file is known to be gone/removed. */
  invalid?: boolean
}

export interface Occurrence {
  offset: number
  chip: Chip
}

/** Build the occurrence table by scanning a text for placeholders. */
export function buildOccurrences(text: string): Occurrence[] {
  const out: Occurrence[] = []
  let idx = text.indexOf(PLACEHOLDER)
  while (idx !== -1) {
    const chip: Chip = { id: `chip:${out.length}`, label: `#${out.length}`, kind: 'file', path: '' }
    out.push({ offset: idx, chip })
    idx = text.indexOf(PLACEHOLDER, idx + 1)
  }
  return out
}

export interface SerializeResult {
  /** Placeholders replaced by their chip's reference payload. */
  serialized: string
  /** Number of chips embedded. */
  count: number
}

/**
 * Codec: walk the editor text and replace each placeholder with the chip's
 * reference string (`@<label>` by default, `dir:<label>` for dir chips).
 */
export function serialize(text: string, chips: Chip[]): SerializeResult {
  const label = (c: Chip) => (c.kind === 'dir' ? `dir:${c.label}` : c.label)
  const occs = buildOccurrences(text)
  if (occs.length === 0) return { serialized: text, count: 0 }
  // Chips are assigned to occurrence slots in order (insertChip sets them up).
  const bySlot = new Map<number, Chip>()
  for (let i = 0; i < occs.length; i++) {
    const chip = chips[i]
    if (chip) bySlot.set(occs[i].offset, chip)
  }
  let out = ''
  let last = 0
  for (const { offset } of occs) {
    out += text.slice(last, offset)
    const chip = bySlot.get(offset)
    out += chip ? label(chip) : ''
    last = offset + 1
  }
  out += text.slice(last)
  return { serialized: out, count: occs.length }
}

export interface ReplaceResult {
  text: string
  chip: Chip
}

/**
 * Replace the `@query` span [start, end) with a single placeholder and record
 * the chip. Returns the new editor text plus the chip to store.
 */
export function insertChip(text: string, start: number, end: number, chip: Chip): ReplaceResult {
  const next = text.slice(0, start) + PLACEHOLDER + text.slice(end)
  return { text: next, chip: { ...chip } }
}

/** Replace a chip's placeholder (set its label) by locating it via index. */
export function replaceChipLabelAt(text: string, offset: number, label: string): string {
  return text.slice(0, offset) + label + text.slice(offset + 1)
}