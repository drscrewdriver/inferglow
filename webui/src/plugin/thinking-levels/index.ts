// thinking-levels 插件入口：注册设置卡片到 settings.plugin.item（keyed: thinking-levels）。
// 导出 registerThinkingLevelsPlugin() 供宿主调用。
// 注意：本文件为 .ts（无 JSX，用 createElement），保持 tsc 类型安全。

import { createElement } from 'react'
import { registerSlot, setCardinality } from '../registry'
import { ThinkingLevelsCard } from './ThinkingLevelsCard'

export { ThinkingLevelsCard } from './ThinkingLevelsCard'
export { findEffectiveLevel } from './ThinkingLevelsCard'
export { decideEffort, type ThinkingLevelsConfig, type ToolCallSample } from './decide'
export { LEVELS, AUTO_LEVEL, MIN_LEVEL, MAX_LEVEL, levelEffort, levelLabel } from './levels'
export { useThinkingLevelsStore } from './store'

const SLOT = 'settings.plugin.item'
const KEY = 'thinking-levels'

/**
 * 注册 thinking-levels 插件：把设置卡片挂到 settings.plugin.item（keyed）。
 * 返回注销函数。幂等（按 id 去重），可热重载重复调用。
 */
export function registerThinkingLevelsPlugin(): () => void {
  setCardinality(SLOT, 'keyed')
  return registerSlot<{ key?: string }>(
    SLOT,
    ({ key }) => (key === KEY ? createElement(ThinkingLevelsCard, null) : null),
    { id: 'thinking-levels:card', key: KEY, order: 20 },
  )
}