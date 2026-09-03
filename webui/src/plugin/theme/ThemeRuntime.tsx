/**
 * 主题应用组件：把 store 中解析出的 CSS 变量写入 <html> 内联 style，
 * 并同步 data-theme 与 body[data-ds-dark-theme]（DSH 暗色切换惯用法）。
 * 变量以「实色值」直接写入（见 themes.ts 说明），避免 CSS guaranteed-invalid 缓存陷阱。
 */
import { useEffect, useMemo, useState } from 'react'
import { useTheme, type ThemeMode } from './themeStore'
import { resolveThemeVars } from './themes'

const REP_DARK = 'midnight'
const REP_LIGHT = 'high-contrast'

/** 依据三态模式解析最终生效的主题 key。 */
function effectiveKey(mode: ThemeMode, themeKey: string, sysDark: boolean): string {
  if (mode === 'system') return sysDark ? REP_DARK : REP_LIGHT
  const wantDark = mode === 'dark'
  const resolved = resolveThemeVars(themeKey)
  if (resolved.dark !== wantDark) return wantDark ? REP_DARK : REP_LIGHT
  return themeKey
}

/** 将解析结果写到 <html>/<body>。返回实际是否为暗色。 */
export function applyThemeToDom(key: string, communityTokens: Record<string, string> | null): boolean {
  const r = document.documentElement
  const resolved = resolveThemeVars(key)
  const vars = communityTokens ? { ...resolved.vars, ...communityTokens } : resolved.vars
  for (const [k, v] of Object.entries(vars)) r.style.setProperty(k, v)
  r.setAttribute('data-theme', resolved.dark ? 'dark' : 'light')
  document.body.dataset.dsDarkTheme = resolved.dark ? 'dark' : 'light'
  return resolved.dark
}

export function ThemeRuntime() {
  const mode = useTheme((s) => s.mode)
  const themeKey = useTheme((s) => s.themeKey)
  const communityTokens = useTheme((s) => s.communityTokens)
  const [sysDark, setSysDark] = useState(
    () => (typeof window !== 'undefined' ? window.matchMedia('(prefers-color-scheme: dark)').matches : true),
  )

  // 系统暗色跟随
  useEffect(() => {
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    const onChange = (e: MediaQueryListEvent) => setSysDark(e.matches)
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }, [])

  const effective = useMemo(
    () => effectiveKey(mode, themeKey, sysDark),
    [mode, themeKey, sysDark],
  )

  useEffect(() => {
    applyThemeToDom(effective, communityTokens)
  }, [effective, communityTokens])

  return null
}