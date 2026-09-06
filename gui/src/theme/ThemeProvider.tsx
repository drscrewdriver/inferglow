import { useEffect } from 'react'
import { resolveThemeVars } from './themes'

/** Applies a theme codename to <html>: writes every CSS custom property and
 * switches data-theme to dark/light (port of the prototype's applyThemeCard). */
export function applyTheme(key: string): void {
  const { vars, dark } = resolveThemeVars(key)
  const r = document.documentElement
  for (const [k, v] of Object.entries(vars)) {
    r.style.setProperty(k, v)
  }
  r.setAttribute('data-theme', dark ? 'dark' : 'light')
}

/** React wrapper that re-applies the theme whenever the codename changes. */
export function ThemeProvider({ themeKey, children }: { themeKey: string; children: React.ReactNode }) {
  useEffect(() => {
    applyTheme(themeKey)
  }, [themeKey])
  return <>{children}</>
}
