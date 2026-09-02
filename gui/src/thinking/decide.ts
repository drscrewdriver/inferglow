// thinking/decide.ts — Auto reasoning-effort scheduler (Phase 6, align
// dsh-thinking-levels). Pure functions: given a config and the recent tool
// calls, pick a reasoning level and map it to a reasoning_effort the backend
// can inject into the LLM request.
//
// The Auto policy mirrors dsh-thinking-levels' `thinking-level.ts`:
//   - Hub = high (official default)
//   - empty calls  → allowDowngrade ? low : high
//   - simpleRatio >= 0.75 && allowDowngrade → low (simple-tool downgrade)
//   - heaviest single arg >= HEAVY_ARGS*4 && allowUpgrade → max (huge-arg upgrade)
//   - otherwise → high

/** Sliding window of recent tool calls consulted by the Auto scheduler. */
export const TOOL_WINDOW = 8

/** Arg-size (in chars) past which a single argument counts as "heavy". */
export const HEAVY_ARGS = 800

/** A "simple" tool call is one whose biggest argument stays well under HEAVY_ARGS
 * and carries a small argument total — i.e. low reasoning risk that can downgrade. */
export function isSimpleCall(_name: string, argSize: number): boolean {
  return argSize < HEAVY_ARGS
}

export type ThinkingLevel = 'auto' | 'low' | 'medium' | 'high' | 'max'

export type ReasoningEffort = 'low' | 'medium' | 'high' | 'max'

export interface ThinkingLevelsConfig {
  enabled: boolean
  /** 'auto' asks the scheduler to choose; a fixed level bypasses it. */
  level: ThinkingLevel
  allowDowngrade: boolean
  allowUpgrade: boolean
}

export interface ToolCallSample {
  name: string
  /** Size of the largest argument passed to the tool (chars). */
  argSize: number
}

export const DEFAULT_THINKING_CONFIG: ThinkingLevelsConfig = {
  enabled: true,
  level: 'auto',
  allowDowngrade: true,
  allowUpgrade: false,
}

/** A sample carrying no tool calls at all. */
export const EMPTY_CALLS: ToolCallSample[] = []

/** Ratio (0..1) of recent calls considered "simple". */
export function simpleRatio(calls: ToolCallSample[]): number {
  if (calls.length === 0) return 1
  const simple = calls.filter((c) => isSimpleCall(c.name, c.argSize)).length
  return simple / calls.length
}

/** Biggest single argument across the recent window (chars). */
export function heaviestArg(calls: ToolCallSample[]): number {
  return calls.reduce((max, c) => (c.argSize > max ? c.argSize : max), 0)
}

/** Recent tool calls trimmed to the scheduler's sliding window. */
export function recentCalls(calls: ToolCallSample[], window = TOOL_WINDOW): ToolCallSample[] {
  return calls.slice(-window)
}

/**
 * Decide the reasoning effort for the next step given a config and recent
 * tool calls. Fixed `level` (non-auto) returns that level's effort directly.
 * In `auto` mode the scheduler applies the simple/huge rules above.
 */
export function decideEffort(
  config: ThinkingLevelsConfig,
  calls: ToolCallSample[],
): ReasoningEffort {
  if (!config.enabled) return 'high'

  const level: ThinkingLevel = config.level === 'auto' ? 'auto' : config.level

  if (level !== 'auto') return normalize(level)

  const win = recentCalls(calls)
  if (win.length === 0) return config.allowDowngrade ? 'low' : 'high'

  // Huge-arg upgrade: a single argument past HEAVY_ARGS*4 → max.
  if (config.allowUpgrade && heaviestArg(win) >= HEAVY_ARGS * 4) return 'max'

  // Simple-tool downgrade: ≥75% simple → low.
  if (config.allowDowngrade && simpleRatio(win) >= 0.75) return 'low'

  return 'high'
}

/** Map a fixed ThinkingLevel to its ReasoningEffort. */
export function normalize(level: ThinkingLevel): ReasoningEffort {
  switch (level) {
    case 'auto':
    case 'high':
      return 'high'
    case 'low':
      return 'low'
    case 'medium':
      return 'medium'
    case 'max':
      return 'max'
  }
}