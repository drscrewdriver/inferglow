/**
 * tidychat — Phase 3.5 configuration, fold-state helpers and shared constants.
 *
 * Ported from the dsh-tidychat plugin (pure data + navigation palette + the
 * smart auto-load governor budgets). All values here are intentionally kept
 * identical to the reference so behavior, colors and tuning match.
 */

/** The 8 tidychat toggles / colors persisted and applied live. */
export interface TidychatConfig {
  /** 自动折叠已完成轮次 */
  fold: boolean
  /** 思考↔正文分隔线 */
  divider: boolean
  /** 左缘定位条（Canvas Minimap） */
  navigator: boolean
  /** 智能加载更早历史 */
  autoLoad: boolean
  /** 定位条默认色：'auto'（随背景明暗）或 NGC 色系 key */
  navColor: string
  /** 默认色明度 l1..l5（auto 时忽略但保留） */
  navColorLight: string
  /** 强调色（当前/悬停回合）色系 key */
  navAccent: string
  /** 强调色明度 l1..l5 */
  navAccentLight: string
}

export const DEFAULT_TIDYCHAT_CONFIG: TidychatConfig = {
  fold: true,
  divider: true,
  navigator: true,
  autoLoad: true,
  navColor: 'auto',
  navColorLight: 'l3',
  navAccent: 'blue',
  navAccentLight: 'l3',
}

// ─── Fold state (per-session per-turn boolean map) ────────────────────────
export type FoldState = Record<number, boolean>

/** A turn defaults to collapsed when absent from the map. */
export function isTurnFolded(state: FoldState | undefined, turn: number): boolean {
  return state?.[turn] ?? true
}

/** Immutably set one turn's fold flag. */
export function withTurnFold(state: FoldState, turn: number, folded: boolean): FoldState {
  return { ...state, [turn]: folded }
}

/** Immutably flip one turn's fold flag (default-collapsed base). */
export function toggleTurnFold(state: FoldState, turn: number): FoldState {
  return withTurnFold(state, turn, !isTurnFolded(state, turn))
}

// ─── Navigator palette ─────────────────────────────────────────────────────
export type NavLightKey = 'l1' | 'l2' | 'l3' | 'l4' | 'l5'

/** 9 hue families × 5 lightness levels (l1 极浅 … l5 极深) — verbatim. */
export const NAV_HUE_PALETTE: Record<string, [string, string, string, string, string]> = {
  gray: ['rgba(225,225,225,0.9)', 'rgba(190,190,190,0.78)', 'rgba(128,128,128,0.8)', 'rgba(70,70,70,0.85)', 'rgba(20,20,20,0.92)'],
  black: ['rgba(90,90,90,0.8)', 'rgba(60,60,60,0.85)', 'rgba(30,30,30,0.9)', 'rgba(12,12,12,0.94)', 'rgba(0,0,0,0.97)'],
  white: ['rgba(255,255,255,0.95)', 'rgba(250,250,250,0.9)', 'rgba(240,240,240,0.85)', 'rgba(225,225,225,0.8)', 'rgba(205,205,205,0.75)'],
  blue: ['#93c5fd', '#60a5fa', '#3b82f6', '#2563eb', '#1e40af'],
  violet: ['#c4b5fd', '#a78bfa', '#8b5cf6', '#7c3aed', '#5b21b6'],
  cyan: ['#67e8f9', '#22d3ee', '#06b6d4', '#0891b2', '#155e75'],
  green: ['#86efac', '#4ade80', '#22c55e', '#16a34a', '#166534'],
  orange: ['#fdba74', '#fb923c', '#f97316', '#ea580c', '#9a3412'],
  red: ['#fca5a5', '#f87171', '#ef4444', '#dc2626', '#991b1b'],
}

export const NAV_LIGHT_IDX: Record<string, number> = { l1: 0, l2: 1, l3: 2, l4: 3, l5: 4 }

/** Resolve a (hue, light) pair against the palette; falls back when unknown. */
export function hueColor(hue: unknown, light: unknown, fallback: string): string {
  if (typeof hue === 'string') {
    const palette = NAV_HUE_PALETTE[hue]
    if (palette !== undefined) {
      const idx = NAV_LIGHT_IDX[typeof light === 'string' ? light : 'l3']
      return palette[idx ?? 2]
    }
  }
  return fallback
}

export interface NavHueOption {
  key: string
  label: string
  preview: string
}

export const NAV_HUE_OPTIONS: ReadonlyArray<NavHueOption> = [
  { key: 'gray', label: '灰', preview: '#9e9e9e' },
  { key: 'black', label: '黑', preview: '#111111' },
  { key: 'white', label: '白', preview: '#f5f5f5' },
  { key: 'blue', label: '蓝', preview: '#3b82f6' },
  { key: 'violet', label: '紫', preview: '#8b5cf6' },
  { key: 'cyan', label: '青', preview: '#06b6d4' },
  { key: 'green', label: '绿', preview: '#22c55e' },
  { key: 'orange', label: '橙', preview: '#f97316' },
  { key: 'red', label: '红', preview: '#ef4444' },
]

export const NAV_LIGHT_OPTIONS: ReadonlyArray<{ key: string; label: string }> = [
  { key: 'l1', label: '极浅' },
  { key: 'l2', label: '浅' },
  { key: 'l3', label: '中' },
  { key: 'l4', label: '深' },
  { key: 'l5', label: '极深' },
]

// ─── Smart auto-load governor budgets (verbatim from the reference) ───────
export const SOFT_BUDGET_MS = 30
export const HARD_BUDGET_MS = 50
export const CONSECUTIVE_SLOW_LIMIT = 3
export const SETTLE_QUIET_MS = 300

// ─── Navigator geometry ────────────────────────────────────────────────────
export const NAV_RAIL_WIDTH = 48
export const NAV_RAIL_BAR_H = 3
export const NAV_RAIL_BAR_LEN = 14
export const NAV_RAIL_BAR_LEN_NEAR = 26
export const NAV_RAIL_BAR_LEN_CURRENT = 22
export const NAV_RAIL_FISH_EYE_RADIUS = 4
export const NAV_RAIL_FISH_EYE_BOOST = 0.5
export const NAV_RAIL_TURN_SPACING = 12
export const NAV_RAIL_MIN_HEIGHT = 48
export const NAV_RAIL_MAX_HEIGHT = 660
export const HEADER_OFFSET = 64