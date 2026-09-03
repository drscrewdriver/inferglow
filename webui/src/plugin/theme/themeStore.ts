/**
 * 主题运行时状态（dark/light/system 三态 + 内置主题 + 社区 token 叠加）。
 * 通过 zustand 管理；应用 CSS 变量的副作用在 ThemeRuntime 组件里执行。
 */
import { create } from 'zustand'
import { resolveThemeVars, THEMES } from './themes'

export type ThemeMode = 'dark' | 'light' | 'system'

export interface ThemeState {
  mode: ThemeMode
  /** 当前内置主题 key（dark/light 模式下决定色板；system 模式忽略） */
  themeKey: string
  /** 社区主题叠加的 CSS 变量（overrideTokens 的输入） */
  communityTokens: Record<string, string> | null
  setMode: (m: ThemeMode) => void
  setThemeKey: (k: string) => void
  overrideTokens: (t: Record<string, string> | null) => void
}

const STORE_KEY = 'igw.theme'

function load(): Pick<ThemeState, 'mode' | 'themeKey'> {
  try {
    const raw = localStorage.getItem(STORE_KEY)
    if (raw) {
      const p = JSON.parse(raw)
      return { mode: p.mode ?? 'system', themeKey: p.themeKey ?? 'midnight' }
    }
  } catch { /* ignore */ }
  return { mode: 'system', themeKey: 'midnight' }
}

const persist = (s: Pick<ThemeState, 'mode' | 'themeKey'>) => {
  try { localStorage.setItem(STORE_KEY, JSON.stringify(s)) } catch { /* ignore */ }
}

export const useTheme = create<ThemeState>((set, get) => ({
  mode: load().mode,
  themeKey: load().themeKey in THEMES ? load().themeKey : 'midnight',
  communityTokens: null,
  setMode: (m) => { set({ mode: m }); persist({ mode: m, themeKey: get().themeKey }) },
  setThemeKey: (k) => { set({ themeKey: k }); persist({ mode: get().mode, themeKey: k }) },
  overrideTokens: (t) => set({ communityTokens: t }),
}))

/** 解析出当前最终生效的变量（互补 resolver：供非 React 场景直接调用）。 */
export function currentVars(): { vars: Record<string, string>; dark: boolean } {
  const s = useTheme.getState()
  const resolved = resolveThemeVars(s.themeKey)
  if (s.communityTokens) {
    return { vars: { ...resolved.vars, ...s.communityTokens }, dark: resolved.dark }
  }
  return resolved
}