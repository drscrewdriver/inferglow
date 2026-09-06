// thinking/levels.ts — display metadata for the thinking-level selector.

import type { ThinkingLevel } from './decide'

export const LEVEL_OPTIONS: { key: ThinkingLevel; label: string; dot: string }[] = [
  { key: 'low', label: '低', dot: 'var(--ok)' },
  { key: 'medium', label: '中', dot: 'var(--warn)' },
  { key: 'high', label: '高', dot: 'var(--warn-2, #e0a13c)' },
  { key: 'max', label: '最大', dot: 'var(--err)' },
  { key: 'auto', label: '自动', dot: 'var(--accent)' },
]

export function levelLabel(key: ThinkingLevel): string {
  return LEVEL_OPTIONS.find((o) => o.key === key)?.label ?? key
}