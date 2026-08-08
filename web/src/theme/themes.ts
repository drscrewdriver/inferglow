// Theme registry migrated verbatim from prototypes/inferglow-gui/index.html
// (THEMES object). Each theme is a complete CSS-variable palette; dark themes
// are the default. OpenHana-group entries omit accentFg/accentStrong/goal/
// avatar fields, which applyTheme derives with fallbacks.

export interface Theme {
  /** Group: "openhana" (12 ported themes) or "original" (8 Culture ships). */
  g?: string
  name: string
  /** Mode badge; the original ship group omits it. */
  mode?: string
  dark: boolean
  idea: string
  origin?: string
  bg: string
  bg2: string
  panel: string
  panel2: string
  border: string
  borderSoft: string
  text: string
  textDim: string
  textFaint: string
  accent: string
  accentSoft: string
  accentFg?: string
  accentStrong?: string
  coral?: string
  gold?: string
  goal?: string
  goalStrong?: string
  aA?: string
  aB?: string
  aA2?: string
  aB2?: string
}

export const THEMES: Record<string, Theme> = {
  /* ── OpenHana 组（openhanako v0.442.0 原封配色） ── */
  midnight: {
    g: 'openhana', name: '青夜', mode: '夜间', dark: true, idea: '开放式深蓝 · 默认',
    bg: '#0d1016', bg2: '#12161d', panel: '#161c25', panel2: '#1d2430', border: '#2b3442', borderSoft: '#232c39',
    text: '#e6ecf4', textDim: '#8fa1b5', textFaint: '#5e6f81', accent: '#5aa2e0', accentSoft: 'rgba(90,162,224,.14)',
    coral: '#e07a6a', gold: '#d9b36a',
  },
  auto: {
    g: 'openhana', name: '自动', mode: '跟随系统', dark: true, idea: '深色 · 草木',
    bg: '#141412', bg2: '#1a1a17', panel: '#1d1d19', panel2: '#23231f', border: '#30302a', borderSoft: '#282824',
    text: '#eae8e1', textDim: '#9c9b91', textFaint: '#71716b', accent: '#7fb069', accentSoft: 'rgba(127,176,105,.14)',
    coral: '#d98a6a', gold: '#d9b36a',
  },
  'warm-paper': {
    g: 'openhana', name: '暖纸', mode: '白天', dark: false, idea: '暖纸 · 蜜茶',
    bg: '#f8f4ed', bg2: '#f0ebe0', panel: '#fdfbf6', panel2: '#f4efe4', border: '#e0d8c6', borderSoft: '#e8e1d2',
    text: '#2b2a26', textDim: '#6d6a5f', textFaint: '#9c987f', accent: '#7f8f5a', accentSoft: 'rgba(127,143,90,.12)',
    coral: '#c97a5a', gold: '#b8923f',
  },
  'high-contrast': {
    g: 'openhana', name: '素白', mode: '高对比', dark: false, idea: '素白 · 高对比',
    bg: '#faf8f7', bg2: '#f1efec', panel: '#ffffff', panel2: '#f5f3f0', border: '#d5d0c8', borderSoft: '#e0dcd5',
    text: '#1f1e1c', textDim: '#5a574f', textFaint: '#8a877e', accent: '#2f6f4f', accentSoft: 'rgba(47,111,79,.12)',
    coral: '#c05a3a', gold: '#a87a20',
  },
  'grass-aroma': {
    g: 'openhana', name: '草香', mode: 'Butter', dark: false, idea: '草香 · 皂绿',
    bg: '#f5f8f3', bg2: '#eaf0e4', panel: '#fbfdf8', panel2: '#f0f5ea', border: '#d8e0cd', borderSoft: '#e2e8d8',
    text: '#2a2f26', textDim: '#5f6b52', textFaint: '#8b977c', accent: '#6f9a4a', accentSoft: 'rgba(111,154,74,.12)',
    coral: '#c97a5a', gold: '#a8a03f',
  },
  contemplation: {
    g: 'openhana', name: '沉思', mode: 'Ming', dark: false, idea: '沉思 · 雾蓝',
    bg: '#f3f5f7', bg2: '#e9edf1', panel: '#fafbfd', panel2: '#eef2f5', border: '#d5dce2', borderSoft: '#dfe5ea',
    text: '#282d33', textDim: '#5f6b75', textFaint: '#8b97a0', accent: '#4a7fa8', accentSoft: 'rgba(74,127,168,.12)',
    coral: '#c97a5a', gold: '#a87a20',
  },
  absolutely: {
    g: 'openhana', name: 'Absolutely', mode: '有一点点熟悉', dark: false, idea: '米白 · 极简',
    bg: '#f4f3ee', bg2: '#eae8e0', panel: '#fbfaf6', panel2: '#efeee7', border: '#d8d5c9', borderSoft: '#e2dfd4',
    text: '#2b2a26', textDim: '#6b675c', textFaint: '#989588', accent: '#8a6f3a', accentSoft: 'rgba(138,111,58,.12)',
    coral: '#c97a5a', gold: '#a87a20',
  },
  delve: {
    g: 'openhana', name: '随时准备接住你', mode: '探究一下', dark: false, idea: '纯白 · 简明',
    bg: '#ffffff', bg2: '#f5f5f3', panel: '#ffffff', panel2: '#f8f8f6', border: '#d8d8d3', borderSoft: '#e3e3df',
    text: '#262626', textDim: '#5f5f5a', textFaint: '#8c8c87', accent: '#5f8f4a', accentSoft: 'rgba(95,143,74,.12)',
    coral: '#c97a5a', gold: '#b8923f',
  },
  'deep-think': {
    g: 'openhana', name: '用户彻底怒了', mode: '小鲸鱼', dark: false, idea: '净白 · 靛蓝',
    bg: '#fcfcfd', bg2: '#f2f2f4', panel: '#ffffff', panel2: '#f5f5f7', border: '#d8d8de', borderSoft: '#e3e3e8',
    text: '#26262a', textDim: '#5f5f68', textFaint: '#8c8c95', accent: '#5a5aa8', accentSoft: 'rgba(90,90,168,.12)',
    coral: '#c97a7a', gold: '#a8a03f',
  },
  'new-warm-paper': {
    g: 'openhana', name: '新暖纸', mode: '纸本', dark: false, idea: '新暖纸 · 焦糖',
    bg: '#f5efe4', bg2: '#ece4d4', panel: '#fbf7ee', panel2: '#f1eadb', border: '#dcd2be', borderSoft: '#e5dcc9',
    text: '#2b2a26', textDim: '#6d685c', textFaint: '#99927f', accent: '#8a6f3a', accentSoft: 'rgba(138,111,58,.12)',
    coral: '#c97a5a', gold: '#a87a20',
  },
  'midnight-contrast': {
    g: 'openhana', name: '青夜·高对比', mode: '清晰', dark: true, idea: '青夜 · 高对比',
    bg: '#26343d', bg2: '#202d34', panel: '#2c3b45', panel2: '#33434d', border: '#4a5a64', borderSoft: '#3f4e57',
    text: '#eef2f5', textDim: '#a5b6c2', textFaint: '#7a8b97', accent: '#7fb0a0', accentSoft: 'rgba(127,176,160,.16)',
    coral: '#e07a6a', gold: '#d9b36a',
  },
  coral: {
    g: 'openhana', name: '珊瑚', mode: '春日和纸', dark: false, idea: '珊瑚 · 春日和纸',
    bg: '#fdf6ec', bg2: '#f5ebdd', panel: '#fffbf4', panel2: '#f6eee0', border: '#e3d3ba', borderSoft: '#eaddc8',
    text: '#2b2a26', textDim: '#6d685c', textFaint: '#99927f', accent: '#c97a5a', accentSoft: 'rgba(201,122,90,.14)',
    coral: '#e06a4a', gold: '#b8923f',
  },
  /* ── 原创组（SpaceX 弹道回收船命名） ── */
  ocisly: {
    name: '当然我还爱你', idea: '靛蓝夜 · 落海燃料橙', dark: true, origin: 'OCISLY · 《The Player of Games》',
    bg: '#141319', bg2: '#1a1921', panel: '#1f1e27', panel2: '#262431', border: '#322f3e', borderSoft: '#2a2836',
    text: '#ece9f2', textDim: '#a29db0', textFaint: '#6f6a7d', accent: '#ff9e5e', accentFg: '#2a1508', accentStrong: '#ffb37d', accentSoft: 'rgba(255,158,94,.16)',
    coral: '#ff9e5e', gold: '#e8c46a', goal: '#8f7fd8', goalStrong: '#b0a4e8', aA: '#ffb37d', aB: '#b06a44', aA2: '#8f7fd8', aB2: '#5a4a9f',
  },
  jrti: {
    name: '读一下说明书', idea: '香草纸 · 枫糖', dark: false, origin: 'JRTI · 《The Player of Games》',
    bg: '#f5f1e8', bg2: '#ede7d9', panel: '#faf7f0', panel2: '#f1ecdf', border: '#ddd6c4', borderSoft: '#e6e0d1',
    text: '#2b2822', textDim: '#6f6a5e', textFaint: '#9c967f', accent: '#a8793f', accentFg: '#ffffff', accentStrong: '#c2925a', accentSoft: 'rgba(168,121,63,.13)',
    coral: '#c97a5a', gold: '#b8923f', goal: '#4a7fa8', goalStrong: '#6a9fd8', aA: '#a8793f', aB: '#c9a87a', aA2: '#6a9fd8', aB2: '#4a7fa8',
  },
  gravitas: {
    name: '重力不足', idea: '深紫 · 信号绿', dark: true, origin: 'A Shortfall of Gravitas · 《文明》',
    bg: '#141218', bg2: '#1a1820', panel: '#1f1c26', panel2: '#262332', border: '#322e3e', borderSoft: '#2a2736',
    text: '#ebe7f0', textDim: '#a29cae', textFaint: '#6d6678', accent: '#7fd8a0', accentFg: '#0a2415', accentStrong: '#9ce6ba', accentSoft: 'rgba(127,216,160,.16)',
    coral: '#d8a07f', gold: '#d8c17f', goal: '#8f9fd8', goalStrong: '#b0bce8', aA: '#9ce6ba', aB: '#4a8a66', aA2: '#8f9fd8', aB2: '#5a66a0',
  },
  lover: {
    name: '新恋人将至的期待', idea: '陶土红 · 玫瑰金', dark: true, origin: "The Anticipation of a New Lover's Arrival · 《Use of Weapons》",
    bg: '#171316', bg2: '#1d181c', panel: '#221d21', panel2: '#2a2429', border: '#382f36', borderSoft: '#2f282e',
    text: '#efe6ea', textDim: '#a89aa1', textFaint: '#74666e', accent: '#e08a9a', accentFg: '#2a1116', accentStrong: '#f0a3b1', accentSoft: 'rgba(224,138,154,.16)',
    coral: '#e08a9a', gold: '#e0c08a', goal: '#8fb0d8', goalStrong: '#b0cce8', aA: '#f0a3b1', aB: '#a05a6a', aA2: '#8fb0d8', aB2: '#5a7aa0',
  },
  attitude: {
    name: '态度调节器', idea: '冷钢灰 · 空调青', dark: true, origin: 'Attitude Adjuster · 《Consider Phlebas》',
    bg: '#131518', bg2: '#191c20', panel: '#1e2126', panel2: '#25292f', border: '#31363d', borderSoft: '#292d34',
    text: '#e8ecf1', textDim: '#9aa2ad', textFaint: '#666e79', accent: '#6fd3e8', accentFg: '#0a1a1f', accentStrong: '#8fe0f0', accentSoft: 'rgba(111,211,232,.16)',
    coral: '#e8a06f', gold: '#e8d06f', goal: '#8f9fd8', goalStrong: '#b0bce8', aA: '#8fe0f0', aB: '#4a7a8a', aA2: '#8f9fd8', aB2: '#5a66a0',
  },
  killing: {
    name: '消磨时光', idea: '碳黑 · 琥珀', dark: true, origin: 'Killing Time · 《Consider Phlebas》/《Excession》',
    bg: '#0e0f10', bg2: '#141517', panel: '#181a1c', panel2: '#1f2124', border: '#2c2f33', borderSoft: '#26282b',
    text: '#ecebe7', textDim: '#9d9b95', textFaint: '#6a6863', accent: '#d9a45b', accentFg: '#2a1c08', accentStrong: '#ecc383', accentSoft: 'rgba(217,164,91,.16)',
    coral: '#d97a5b', gold: '#d9c95b', goal: '#7fa8d9', goalStrong: '#a0c4ec', aA: '#ecc383', aB: '#8a6a3a', aA2: '#7fa8d9', aB2: '#4a6a9f',
  },
  funny: {
    name: '奇怪，上次还好使', idea: '石板灰 · 酸橙', dark: true, origin: 'Funny, It Worked Last Time... · 《Look to Windward》',
    bg: '#121417', bg2: '#181b1f', panel: '#1c1f24', panel2: '#23262c', border: '#2f333a', borderSoft: '#272b31',
    text: '#e9ece9', textDim: '#9aa69c', textFaint: '#667068', accent: '#a8d66a', accentFg: '#1a2408', accentStrong: '#bfe08a', accentSoft: 'rgba(168,214,106,.15)',
    coral: '#d8a06a', gold: '#d8d06a', goal: '#8fa8d8', goalStrong: '#b0c4e8', aA: '#bfe08a', aB: '#5a8a3a', aA2: '#8fa8d8', aB2: '#5a6aa0',
  },
  windward: {
    name: '迎风远眺', idea: '炭灰 · 月银蓝', dark: true, origin: 'Look To Windward · 《Look to Windward》',
    bg: '#14161a', bg2: '#1a1d22', panel: '#1f2228', panel2: '#262a31', border: '#32363e', borderSoft: '#2a2e35',
    text: '#e9ebef', textDim: '#9ba0aa', textFaint: '#676c76', accent: '#9fb4d8', accentFg: '#101827', accentStrong: '#bccbe8', accentSoft: 'rgba(159,180,216,.16)',
    coral: '#d8a0a0', gold: '#d8c8a0', goal: '#8fa8d8', goalStrong: '#b0c4e8', aA: '#bccbe8', aB: '#5a6a8a', aA2: '#8fa8d8', aB2: '#5a6aa0',
  },
}

export const THEME_GROUPS: { label: string; keys: string[] }[] = [
  { label: 'OpenHana', keys: ['midnight', 'auto', 'warm-paper', 'high-contrast', 'grass-aroma', 'contemplation', 'absolutely', 'delve', 'deep-think', 'new-warm-paper', 'midnight-contrast', 'coral'] },
  { label: '原创 · 飞船', keys: ['ocisly', 'jrti', 'gravitas', 'lover', 'attitude', 'killing', 'funny', 'windward'] },
]

/** Resolve a theme into the complete set of CSS custom properties, deriving
 * fallbacks for fields the OpenHana group omits (mirrors applyThemeCard). */
export function resolveThemeVars(key: string): { vars: Record<string, string>; dark: boolean } {
  const t = THEMES[key] ?? THEMES.midnight
  const accentFg = t.accentFg ?? (t.dark ? '#0a0a08' : '#ffffff')
  const accentStrong = t.accentStrong ?? t.accent
  const goal = t.goal ?? '#5aa2e0'
  const goalStrong = t.goalStrong ?? goal
  const aA = t.aA ?? '#8ab4a0'
  const aB = t.aB ?? '#5f7f6a'
  const aA2 = t.aA2 ?? '#5aa2e0'
  const aB2 = t.aB2 ?? '#4a7fa8'
  const vars: Record<string, string> = {
    '--bg': t.bg, '--bg2': t.bg2, '--panel': t.panel, '--panel2': t.panel2,
    '--border': t.border, '--border-soft': t.borderSoft,
    '--text': t.text, '--text-dim': t.textDim, '--text-faint': t.textFaint,
    '--accent': t.accent, '--accent-fg': accentFg, '--accent-strong': accentStrong, '--accent-soft': t.accentSoft,
    '--coral': t.coral ?? '', '--gold': t.gold ?? '', '--warn': t.gold ?? '', '--ok': t.accent,
    '--goal': goal, '--goal-strong': goalStrong, '--goal-soft': t.accentSoft,
    '--avatar-a': aA, '--avatar-b': aB, '--avatar-a2': aA2, '--avatar-b2': aB2,
    '--sidebar-active': t.accentSoft,
  }
  return { vars, dark: t.dark }
}
