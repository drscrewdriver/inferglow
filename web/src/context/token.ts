import type { FileOpKind } from './types'

// Heuristic token estimation (mirrors dsh-context's chars/token estimate) and
// the file-op extraction used by the fold. Pure functions — no React, no I/O.

/** Base system-prompt epoch price (tokens). Fixed estimate for InferGlow. */
export const SYSTEM_PROMPT_EST = 3200
/** Tool-schema epoch price (tokens). */
export const TOOLS_SCHEMA_EST = 1800
/** 80% auto-compaction reserve (dsh AUTO_COMPACT_RATIO). */
export const AUTO_COMPACT_RATIO = 0.8

/** Heuristic: ~4 chars per token, CJK-agnostic, matches the harness line. */
export function estimateTokens(text: string | undefined | null): number {
  if (!text) return 0
  return Math.ceil(text.length / 4)
}

/** Format a token count with locale digit grouping. */
export function fmtNum(n: number): string {
  return Number.isFinite(n) ? String(Math.round(n)) : '—'
}

/** Human-friendly thousand separator (e.g. 128,000). */
export function fmtThousand(n: number): string {
  if (!Number.isFinite(n)) return '—'
  const sign = n < 0 ? '-' : ''
  const digits = String(Math.round(Math.abs(n)))
  let out = ''
  for (let i = 0; i < digits.length; i++) {
    if (i > 0 && (digits.length - i) % 3 === 0) out += ','
    out += digits[i]
  }
  return sign + out
}

/** Classify a tool name into a file-op kind, or null when not a file op. */
export function fileOpKindOf(toolName: string | undefined): FileOpKind | null {
  const name = (toolName ?? '').trim().toLowerCase()
  const map: [FileOpKind, string[]][] = [
    ['read', ['read']],
    ['write', ['write', 'edit', 'patch', 'create', 'append']],
    ['search', ['search', 'grep', 'find', 'rg']],
    ['image', ['image', 'img', 'vision', 'screenshot']],
    ['dir', ['dir', 'ls', 'list', 'dir']],
  ]
  for (const [kind, needles] of map) {
    if (needles.some((n) => name === n || name.startsWith(n + '_') || name.startsWith(n + '-'))) return kind
  }
  return null
}

/** Extract the first sensible path-looking token from a call body. */
export function extractPath(content: string | undefined): string | undefined {
  if (!content) return undefined
  const m = content.match(/["'`]?((?:[A-Za-z]:)?[\\/][^"'`\n]*[\w])/i)
  return m ? m[1] : undefined
}