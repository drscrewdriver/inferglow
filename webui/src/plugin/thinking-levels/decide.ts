// thinking-levels/decide.ts — 自动思考档位调度引擎（对齐 dsh-thinking-levels）。
// 纯函数：给定配置与最近工具调用历史，在 enable/downgrade/upgrade 开关约束下
// 自动升降档。宿主在消息发送前调用 decideEffort() 注入 reasoning_effort。
// 本模块不依赖 React，保持 decidable。
//
// 决策逻辑（镜像 dsh-thinking-levels `thinking-level.ts`）：
//   - 目标（hub）= 最高档 high
//   - 空采样窗口 → allowDowngrade ? low : high
//   - simpleRatio >= 0.75 && allowDowngrade → low（简单工具降档）
//   - heaviestArg >= HEAVY_ARGS*4 && allowUpgrade → high（超大参数升档）
//   - 其余 → high

import { AUTO_LEVEL, clampLevel, levelEffort, MAX_LEVEL, MIN_LEVEL, type ReasoningEffort } from './levels'

/** 默认滑动窗口：参与自动决策的最近工具调用数量。 */
export const TOOL_WINDOW = 8

/** 单个参数超过该字符数即视为「重」。 */
export const HEAVY_ARGS = 800

/** 简单工具下调判定的简单占比阈值（≥75% → low）。 */
export const SIMPLE_RATIO_THRESHOLD = 0.75

export interface ToolCallSample {
  name: string
  /** 该工具调用中最大参数的大小（字符）。 */
  argSize: number
}

export interface ThinkingLevelsConfig {
  /** 是否启用自动调度；禁用时固定返回最高档。 */
  enabled: boolean
  /** 当前档位：AUTO_LEVEL = 自动，0..LEVEL_COUNT-1 = 固定档。 */
  level: number
  allowDowngrade: boolean
  allowUpgrade: boolean
  /** 参与决策的最近工具调用采样窗口。 */
  sampleWindow: number
}

export const EMPTY_CALLS: ToolCallSample[] = []

export const DEFAULT_THINKING_CONFIG: ThinkingLevelsConfig = {
  enabled: true,
  level: AUTO_LEVEL,
  allowDowngrade: true,
  allowUpgrade: false,
  sampleWindow: TOOL_WINDOW,
}

/** 一次简单调用：最大参数远低于 HEAVY_ARGS（低推理风险，可降档）。 */
export function isSimpleCall(_name: string, argSize: number): boolean {
  return argSize < HEAVY_ARGS
}

/** 最近采样中「简单」调用占比（0..1）；空窗口返回 1。 */
export function simpleRatio(calls: ToolCallSample[]): number {
  if (calls.length === 0) return 1
  const simple = calls.filter((c) => isSimpleCall(c.name, c.argSize)).length
  return simple / calls.length
}

/** 最近采样中单个最大参数大小（字符）；空窗口返回 0。 */
export function heaviestArg(calls: ToolCallSample[]): number {
  return calls.reduce((max, c) => (c.argSize > max ? c.argSize : max), 0)
}

/** 将最近调用裁剪到滑动窗口。 */
export function recentCalls(calls: ToolCallSample[], window = TOOL_WINDOW): ToolCallSample[] {
  return calls.slice(-window)
}

/**
 * 决策下一步的 reasoning effort（返回档位索引 0..MAX_LEVEL）。
 * 固定档（level !== AUTO_LEVEL）直接返回该档位；auto 模式应用上面的升降档规则。
 */
export function decideLevel(
  config: ThinkingLevelsConfig,
  calls: ToolCallSample[],
): number {
  if (!config.enabled) return MAX_LEVEL

  if (config.level !== AUTO_LEVEL) return clampLevel(config.level)

  const win = recentCalls(calls, config.sampleWindow > 0 ? config.sampleWindow : TOOL_WINDOW)

  // 超大参数升档：单个参数 ≥ HEAVY_ARGS*4 → 最高档。
  if (config.allowUpgrade && win.length > 0 && heaviestArg(win) >= HEAVY_ARGS * 4) return MAX_LEVEL

  // 简单工具降档：简单占比 ≥75% → 最低档。
  if (config.allowDowngrade && simpleRatio(win) >= SIMPLE_RATIO_THRESHOLD) return MIN_LEVEL

  return MAX_LEVEL
}

/**
 * 决策最终的 reasoning_effort（'low' | 'medium' | 'high'）。
 * 宿主在消息发送前调用此函数，把结果注入 LLM 请求的 reasoning_effort 字段。
 */
export function decideEffort(
  config: ThinkingLevelsConfig,
  calls: ToolCallSample[],
): ReasoningEffort {
  return levelEffort(decideLevel(config, calls))
}