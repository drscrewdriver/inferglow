// thinking-levels/levels.ts — 思考档位枚举/常量（默认三档，level 0~N-1）。
// 调度决策基于档位索引，AUTO_LEVEL 作为「自动」哨兵表示由调度引擎决定。

/** 最低档位索引。 */
export const MIN_LEVEL = 0

/** 默认档位数。 */
export const LEVEL_COUNT = 3

/** 最高档位索引（= LEVEL_COUNT - 1）。 */
export const MAX_LEVEL = LEVEL_COUNT - 1

/** 「自动」哨兵：不属于 0..LEVEL_COUNT-1，表示档位由 decideEffort 决定。 */
export const AUTO_LEVEL = -1

export type ReasoningEffort = 'low' | 'medium' | 'high'

export interface LevelDef {
  /** 档位索引 0..N-1。 */
  level: number
  /** 映射到后端 reasoning_effort。 */
  effort: ReasoningEffort
  label: string
  dot: string
}

/** 档位定义（默认三档：低/中/高）。 */
export const LEVELS: readonly LevelDef[] = [
  { level: 0, effort: 'low', label: '低', dot: 'var(--ok)' },
  { level: 1, effort: 'medium', label: '中', dot: 'var(--warn)' },
  { level: 2, effort: 'high', label: '高', dot: 'var(--accent)' },
]

/** 将档位索引约束到合法范围 [MIN_LEVEL, MAX_LEVEL]。 */
export function clampLevel(level: number): number {
  if (Number.isNaN(level)) return MIN_LEVEL
  return Math.min(MAX_LEVEL, Math.max(MIN_LEVEL, Math.floor(level)))
}

/** 档位索引 → 档位定义；越界时回退到最高档。 */
export function levelDef(level: number): LevelDef {
  return LEVELS.find((l) => l.level === clampLevel(level)) ?? LEVELS[LEVELS.length - 1]
}

/** 档位索引 → reasoning_effort 字符串。 */
export function levelEffort(level: number): ReasoningEffort {
  return levelDef(level).effort
}

/** 档位索引 → 中文标签。 */
export function levelLabel(level: number): string {
  return levelDef(level).label
}