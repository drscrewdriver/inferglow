// Shared palette + small view helpers for the context components.

import type { Category } from './types'

export const CATEGORY_COLORS: Record<Category, string> = {
  system: '#8a94a6',
  tools: '#b08a2e',
  user: '#3e8fe0',
  inject: '#7a5fbf',
  assistant: '#2fae72',
  tool: '#c25c4a',
}

export const CATEGORY_LABELS: Record<Category, string> = {
  system: '系统',
  tools: '工具',
  user: '用户',
  inject: '注入',
  assistant: '助手',
  tool: '工具结果',
}

/** Order used by stacked bars and legend (visual bottom → top). */
export const CATEGORY_ORDER: readonly Category[] = [
  'system',
  'tools',
  'user',
  'inject',
  'assistant',
  'tool',
]

export const EVENT_ICONS: Record<string, string> = {
  compaction: '✂',
  prune: '✂',
  inject: '⇪',
  model: '◈',
  mode: '⚙',
}

export const FILE_BADGES: Record<string, { label: string; color: string }> = {
  read: { label: '读', color: '#3e8fe0' },
  write: { label: '写', color: '#2fae72' },
  search: { label: '搜', color: '#b08a2e' },
  image: { label: '图', color: '#7a5fbf' },
  dir: { label: '录', color: '#8a94a6' },
}

/** Format a wall-clock timestamp as HH:MM:SS. */
export function fmtClock(ms: number): string {
  if (!ms) return '--:--'
  const d = new Date(ms)
  const p = (n: number) => String(n).padStart(2, '0')
  return `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`
}

/** Format tokens with thousands separators. */
export function fmtTokens(n: number): string {
  if (!Number.isFinite(n)) return '—'
  return n.toLocaleString('en-US')
}