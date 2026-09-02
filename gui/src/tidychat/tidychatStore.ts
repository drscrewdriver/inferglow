/**
 * tidychat — client store: config + per-session fold + governor statuses.
 *
 * Fold and governor state are isolated per session id. Config persists to
 * localStorage (best-effort) and is the single source of truth consumed by the
 * fold / divider / navigator / autoLoad components and the settings card.
 */

import { create } from 'zustand'
import {
  DEFAULT_TIDYCHAT_CONFIG,
  type FoldState,
  type TidychatConfig,
} from './config'
import { createGovernorState, type GovernorState } from './governor'

const STORAGE_KEY = 'inferglow.tidychat.v1'

function loadConfig(): TidychatConfig {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return { ...DEFAULT_TIDYCHAT_CONFIG }
    const parsed = JSON.parse(raw) as Partial<TidychatConfig>
    return { ...DEFAULT_TIDYCHAT_CONFIG, ...parsed }
  } catch {
    return { ...DEFAULT_TIDYCHAT_CONFIG }
  }
}

export function persistConfig(config: TidychatConfig): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(config))
  } catch {
    // localStorage unavailable (private mode) — keep in-memory.
  }
}

export type FoldBySession = Record<string, FoldState>
export type GovernorBySession = Record<string, GovernorState>

interface TidychatState {
  config: TidychatConfig
  foldStates: FoldBySession
  governors: GovernorBySession
  /** Percentage-mapped live color hint for the navigator canvas. */
  busy: boolean

  setConfig: (patch: Partial<TidychatConfig>) => void
  /** Initialize a turn's fold default (inserted only when absent). */
  initFold: (sessionId: string, turn: number) => void
  toggleFold: (sessionId: string, turn: number) => void
  setGovernor: (sessionId: string, state: GovernorState) => void
  resetGovernor: (sessionId: string) => void
  dropSession: (sessionId: string) => void
}

export const useTidychatStore = create<TidychatState>()((set, get) => ({
  config: loadConfig(),
  foldStates: {},
  governors: {},
  busy: false,

  setConfig(patch) {
    const config = { ...get().config, ...patch }
    persistConfig(config)
    set({ config })
  },

  initFold(sessionId, turn) {
    const per = get().foldStates[sessionId] ?? {}
    if (per[turn] !== undefined) return
    set({ foldStates: { ...get().foldStates, [sessionId]: { ...per, [turn]: true } } })
  },

  toggleFold(sessionId, turn) {
    const per = get().foldStates[sessionId] ?? {}
    const prev = per[turn] ?? true
    set({ foldStates: { ...get().foldStates, [sessionId]: { ...per, [turn]: !prev } } })
  },

  setGovernor(sessionId, state) {
    set({ governors: { ...get().governors, [sessionId]: state } })
  },

  resetGovernor(sessionId) {
    set({ governors: { ...get().governors, [sessionId]: createGovernorState('idle') } })
  },

  dropSession(sessionId) {
    const { foldStates, governors } = get()
    const next: typeof foldStates = { ...foldStates }
    const nextG: typeof governors = { ...governors }
    delete next[sessionId]
    delete nextG[sessionId]
    set({ foldStates: next, governors: nextG })
  },
}))

// ─── Selectors ─────────────────────────────────────────────────────────────

/** Fold flag for a turn in a session (default collapsed). */
export function selectTurnFolded(
  foldStates: FoldBySession,
  sessionId: string,
  turn: number,
): boolean {
  return foldStates[sessionId]?.[turn] ?? true
}

/** Governor phase for a session, defaulting to idle. The default reference is
 * cached: returning a fresh object each call would make zustand's snapshot
 * change identity every check, tripping the "getSnapshot should be cached"
 * guard and an infinite re-render loop. */
const IDLE_GOVERNOR: GovernorState = createGovernorState('idle')

export function selectGovernor(governors: GovernorBySession, sessionId: string): GovernorState {
  return governors[sessionId] ?? IDLE_GOVERNOR
}