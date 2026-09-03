/**
 * 外观设置行（AppearanceRow）：dark/light/system 三态选择 + 内置主题选择。
 * 注册到 settings.general.item（keyed: appearance）。
 */
import type { ChangeEvent } from 'react'
import { THEMES, THEME_GROUPS } from './themes'
import { useTheme, type ThemeMode } from './themeStore'
import s from './AppearanceRow.module.css'

const MODES: { m: ThemeMode; label: string; rep: string }[] = [
  { m: 'dark', label: '深色', rep: 'midnight' },
  { m: 'light', label: '浅色', rep: 'high-contrast' },
  { m: 'system', label: '跟随系统', rep: 'mis' },
]

export function AppearanceRow() {
  const mode = useTheme((s) => s.mode)
  const themeKey = useTheme((s) => s.themeKey)
  const setMode = useTheme((s) => s.setMode)
  const setThemeKey = useTheme((s) => s.setThemeKey)

  const onPickMode = (m: ThemeMode, rep: string) => {
    setMode(m)
    if (m !== 'system') {
      const wantDark = m === 'dark'
      const curTheme = THEMES[themeKey]
      // 若当前主题与所选模式相悖，则切换到对应代表主题
      if (curTheme && curTheme.dark !== wantDark) setThemeKey(rep)
    }
  }

  const onPickTheme = (e: ChangeEvent<HTMLSelectElement>) => {
    setThemeKey(e.target.value)
  }

  return (
    <div className={s.themeAppearance}>
      <div className={s.themeModes}>
        {MODES.map(({ m, label, rep }) => (
          <div
            key={m}
            className={`${s.themeModeCard} ${mode === m ? s.active : ''}`}
            onClick={() => onPickMode(m, rep)}
          >
            <div className={s.themeModeSquares}>
              <span className={`${s.themeSquare} ${s.s1}`} />
              <span className={`${s.themeSquare} ${s.s2}`} />
              <span className={`${s.themeSquare} ${s.s3}`} />
            </div>
            <div className={s.themeModeLabel}>{label}</div>
          </div>
        ))}
      </div>
      <div className={s.themePickRow}>
        <span className={s.themePickLabel}>内置主题</span>
        <select className={s.themePickSelect} value={themeKey} onChange={onPickTheme}>
          {THEME_GROUPS.map((g) => (
            <optgroup key={g.label} label={g.label}>
              {g.keys.map((k) => (
                <option key={k} value={k}>
                  {THEMES[k].name} · {THEMES[k].idea}
                </option>
              ))}
            </optgroup>
          ))}
        </select>
      </div>
      <div className={s.themeCommunityNote}>社区主题通过 config.dshTheme 叠加覆盖（overrideTokens 机制）</div>
    </div>
  )
}