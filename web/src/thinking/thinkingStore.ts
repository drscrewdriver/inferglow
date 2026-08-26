// thinking/thinkingStore.ts — localStorage-backed store for the reasoning
// auto-scheduler config. Mirrors the convention used by tidychat/traffic stores.

import { create } from 'zustand'
import { decideEffort, DEFAULT_THINKING_CONFIG, type ReasoningEffort, type ThinkingLevel, type ThinkingLevelsConfig, type ToolCallSample } from './decide'

const KEY = 'inferglow.thinking.v1'

export type { ThinkingLevelsConfig, ReasoningEffort }

interface ThinkingState {
  config: ThinkingLevelsConfig
  /** Latest observed tool calls feeding the Auto scheduler (opaque to the UI). */
  calls: ToolCallSample[]
  lastEffort: ReasoningEffort
  setConfig: (patch: Partial<ThinkingLevelsConfig>) => void
  setLevel: (level: ThinkingLevel) => void
  toggle: (patch: Partial<{ enabled: boolean; allowDowngrade: boolean; allowUpgrade: boolean }>) => void
  observeCalls: (calls: ToolCallSample[]) => void
  reset: () => void
}

function load(): ThinkingLevelsConfig {
  try {
    const raw = localStorage.getItem(KEY)
    return raw ? { ...DEFAULT_THINKING_CONFIG, ...(JSON.parse(raw) as Partial<ThinkingLevelsConfig>) } : DEFAULT_THINKING_CONFIG
  } catch {
    return DEFAULT_THINKING_CONFIG
  }
}

export const useThinkingStore = create<ThinkingState>()((set, get) => ({
  config: load(),
  calls: [],
  lastEffort: 'high',

  setConfig: (patch) => {
    const next = { ...get().config, ...patch }
    try {
      localStorage.setItem(KEY, JSON.stringify(next))
    } catch {
      // ignore
    }
    set({ config: next })
  },

  setLevel: (level) => {
    // 'auto' is the only level that consults the scheduler; any fixed level
    // effectively locks reasoning to that effort.
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