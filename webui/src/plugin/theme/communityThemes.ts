/**
 * 社区主题样例集：以 `--dsw-alias-*` 变量覆盖形式给 webui 换肤。
 * 用户可在配置声明 `dshTheme: "catppuccin-mocha"` 等，由 theme 插件通过
 * overrideTokens() 叠加到 <html>。此处仅收录少数精选主题；扩展时追加即可。
 */
export interface CommunityTheme {
  name: string
  dark: boolean
  /** 需要覆盖的核心 InferGlow / DSH 变量（写入 <html> 内联 style 叠加） */
  tokens: Record<string, string>
}

export const COMMUNITY_THEMES: Record<string, CommunityTheme> = {
  'catppuccin-mocha': {
    name: 'Catppuccin Mocha',
    dark: true,
    tokens: {
      '--bg': '#1e1e2e', '--bg2': '#181825', '--surface': '#313244', '--surface-hover': '#45475a',
      '--border': '#45475a', '--border-soft': '#313244',
      '--text': '#cdd6f4', '--text-dim': '#a6adc8', '--text-faint': '#7f849c',
      '--accent': '#89b4fa', '--accent-soft': 'rgba(137,180,250,.16)',
      '--sidebar-bg': '#181825', '--sidebar-hover': '#313244', '--sidebar-active': '#45475a',
    },
  },
  'solarized-dark': {
    name: 'Solarized Dark',
    dark: true,
    tokens: {
      '--bg': '#002b36', '--bg2': '#073642', '--surface': '#073642', '--surface-hover': '#0a3b47',
      '--border': '#2a6b7a', '--border-soft': '#1a5b6a',
      '--text': '#93a1a1', '--text-dim': '#839496', '--text-faint': '#586e75',
      '--accent': '#268bd2', '--accent-soft': 'rgba(38,139,210,.16)',
      '--sidebar-bg': '#002b36', '--sidebar-hover': '#073642', '--sidebar-active': '#0a3b47',
    },
  },
}

export const DSH_THEME_KEYS = Object.keys(COMMUNITY_THEMES)