/**
 * theme 插件入口：
 *  - 注册设置槽位 `settings.general.item`（keyed: appearance）→ AppearanceRow
 *  - 按配置 `dshTheme` 通过 overrideTokens 叠加社区主题
 */
import { registerSlot, setCardinality } from '../registry'
import { AppearanceRow } from './AppearanceRow'
import { useTheme } from './themeStore'
import { COMMUNITY_THEMES } from './communityThemes'

const SLOT = 'settings.general.item'

export function registerThemePlugin(): () => void {
  setCardinality(SLOT, 'keyed')
  const unregister = registerSlot<{ key?: string }>(
    SLOT,
    ({ key }) => (key === 'appearance' ? <AppearanceRow /> : null),
    { id: 'theme:appearance', key: 'appearance', order: 10 },
  )
  return () => {
    unregister()
  }
}

/** 若配置声明了 dshTheme，则叠加对应社区主题 token。返回清理函数。 */
export function initCommunityTheme(dshTheme?: string | null): () => void {
  if (!dshTheme) return () => {}
  const theme = COMMUNITY_THEMES[dshTheme]
  if (!theme) return () => {}
  useTheme.getState().overrideTokens(theme.tokens)
  return () => useTheme.getState().overrideTokens(null)
}