// thinking-levels/store.ts — 思考档位配置的 zustand store（持久化 localStorage）。
// 存档位/启用/降档/升档/sampleWindow 等配置，并缓存最近工具调用与上次决策结果。

import { create } from 'zustand'
import {
  DEFAULT_THINKING_CONFIG,
  decideEffort,
  type ThinkingLevelsConfig,
  type ToolCallSample,
} from './decide'
import { AUTO_LEVEL, type ReasoningEffort } from './levels'

const KEY = 'inferglow.thinking-levels.v1'

export type { ThinkingLevelsConfig, ReasoningEffort }

interface ThinkingState {
  config: ThinkingLevelsConfig
  /** 最近观察到的工具调用（供自动调度使用，对 UI 透明）。 */
  calls: ToolCallSample[]
  lastEffort: ReasoningEffort
  setConfig: (patch: Partial<ThinkingLevelsConfig>) => void
  setLevel: (level: number) => void
  toggle: (patch: Partial<{ enabled: boolean; allowDowngrade: boolean; allowUpgrade: boolean }>) => void
  observeCalls: (calls: ToolCallSample[]) => void
  reset: () => void
}

function load(): ThinkingLevelsConfig {
  try {
    const raw = localStorage.getItem(KEY)
    if (!raw) return { ...DEFAULT_THINKING_CONFIG }
    const parsed = JSON.parse(raw) as Partial<ThinkingLevelsConfig>
    const next = { ...DEFAULT_THINKING_CONFIG, ...parsed }
    // 归一化：level=-1 是合法 auto 哨兵，其余落到有效档位范围。
    if (next.level !== AUTO_LEVEL && next.level < 0) next.level = AUTO_LEVEL
    return next
  } catch {
    return { ...DEFAULT_THINKING_CONFIG }
  }
}

export const useThinkingLevelsStore = create<ThinkingState>()((set, get) => ({
  config: load(),
  calls: [],
  lastEffort: 'high',

  setConfig: (patch) => {
    const next = { ...get().config, ...patch }
    try {
      localStorage.setItem(KEY, JSON.stringify(next))
    } catch {
      // 持久化失败时忽略
    }
    set({ config: next })
  },

  setLevel: (level) => {
    // AUTO_LEVEL 让调度引擎决策；固定档位锁定 reasoning_effort。
    get().setConfig({ level })
  },

  toggle: (patch) => {
    get().setConfig(patch)
  },

  observeCalls: (calls) => {
    set({ calls, lastEffort: decideEffort(get().config, calls) })
  },

  reset: () => {
    try {
      localStorage.removeItem(KEY)
    } catch {
      // ignore
    }
    set({ config: { ...DEFAULT_THINKING_CONFIG }, calls: [], lastEffort: 'high' })
  },
}))

/** 取当前档位（数字），无副作用，供非 React 场景直接读取。 */
export function currentLevel(): number {
  return useThinkingLevelsStore.getState().config.level
}