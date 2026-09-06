/**
 * Terminal theming + font resolution — a pure module (no DOM, no xterm), so
 * the palette/fallback chain stays unit-testable without mounting a terminal.
 * Mirrors DSH-better-sidebar's TerminalView/terminal-font house style: the
 * surface colors ride the app theme tokens, and the 16 ANSI colors are the
 * one-dark / one-light syntax families shared with the editor surfaces.
 */

export const ANSI_DARK = {
  black: '#282c34',
  red: '#e06c75',
  green: '#98c379',
  yellow: '#e5c07b',
  blue: '#61afef',
  magenta: '#c678dd',
  cyan: '#56b6c2',
  white: '#abb2bf',
  brightBlack: '#5c6370',
  brightRed: '#e06c75',
  brightGreen: '#98c379',
  brightYellow: '#e5c07b',
  brightBlue: '#61afef',
  brightMagenta: '#c678dd',
  brightCyan: '#56b6c2',
  brightWhite: '#ffffff',
} as const

export const ANSI_LIGHT = {
  black: '#383a42',
  red: '#e45649',
  green: '#50a14f',
  yellow: '#c18401',
  blue: '#0184bc',
  magenta: '#a626a4',
  cyan: '#0997b3',
  white: '#a0a1a7',
  brightBlack: '#4f525e',
  brightRed: '#e45649',
  brightGreen: '#50a14f',
  brightYellow: '#c18401',
  brightBlue: '#0184bc',
  brightMagenta: '#a626a4',
  brightCyan: '#0997b3',
  brightWhite: '#fafafa',
} as const

/** Built-in fallback stack (better-sidebar's default terminal font). */
export const DEFAULT_TERMINAL_FONT_FAMILY =
  '"SF Mono", Menlo, Consolas, "Liberation Mono", monospace'

/**
 * Icon fonts appended so Nerd-Font prompt glyphs (starship / p10k PUA code
 * points) resolve instead of tofu. Strictly appended: xterm derives cell
 * metrics from the first family, so the base monospace must stay in front.
 */
export const ICON_FONT_FALLBACKS: readonly string[] = [
  '"Symbols Nerd Font Mono"',
  '"Symbols Nerd Font"',
  '"Hack Nerd Font Mono"',
  '"Hack Nerd Font"',
  '"JetBrainsMono Nerd Font Mono"',
  '"JetBrainsMono Nerd Font"',
  '"CaskaydiaCove Nerd Font Mono"',
  '"CaskaydiaCove Nerd Font"',
]

/** The full xterm fontFamily value: base stack first, icon fonts after. */
export function terminalFontStack(base: string = DEFAULT_TERMINAL_FONT_FAMILY): string {
  return [base, ...ICON_FONT_FALLBACKS].join(', ')
}

export interface TerminalSurface {
  dark: boolean
  /** Live theme-token values (may be '' when the token is unavailable). */
  background?: string
  foreground?: string
}

/** The xterm theme for the current scheme (surface from tokens, ANSI curated). */
export function xtermTheme(s: TerminalSurface): Record<string, string> {
  const background = s.background || (s.dark ? '#151517' : '#ffffff')
  const foreground = s.foreground || (s.dark ? '#e6e6e6' : '#1a1a1a')
  return {
    background,
    foreground,
    cursor: foreground,
    cursorAccent: background,
    selectionBackground: s.dark ? 'rgba(255,255,255,0.22)' : 'rgba(0,0,0,0.12)',
    ...(s.dark ? ANSI_DARK : ANSI_LIGHT),
  }
}
