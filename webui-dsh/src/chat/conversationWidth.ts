/**
 * Conversation content-axis width — mirrors the dsh v0.1.2-alpha.3
 * "conversation adaptive content width" feature (ui-conversation
 * ConversationRoot + WidthHandle): the transcript column is a centered,
 * draggable width, persisted to localStorage. Constants and clamp math come
 * from the dsh reference (CONTENT_MIN / CONTENT_EDGE_BUDGET and the adaptive
 * clamp); the symmetric 2×-outward drag model lives in
 * ConversationWidthHandles.
 */

export const WIDTH_PREF_KEY = 'dsh.conversation.contentWidth'

/** Floor for a dragged content width (mirrors the layout center-column minimum). */
export const CONTENT_MIN = 640

/** Column budget the content must leave free: keeps the two width handles
 * placeable (and draggable back) inside the column. */
export const CONTENT_EDGE_BUDGET = 176

/** Reads the persisted content-width preference. Durable-storage boundary:
 * a missing or corrupt value resolves to "no preference". */
export function readWidthPreference(): number | null {
  const raw = localStorage.getItem(WIDTH_PREF_KEY)
  if (raw === null) return null
  const value = Number(raw)
  return Number.isFinite(value) && value > 0 ? value : null
}

/** Resolves the width the CSS axis would show for a column width + preference.
 * Mirrors the CSS clamp: an adaptive default, or a clamped dragged preference
 * that never dips below CONTENT_MIN or exceeds column width minus the budget. */
export function resolveContentWidth(columnWidth: number, preference: number | null): number {
  const max = Math.max(CONTENT_MIN, columnWidth - CONTENT_EDGE_BUDGET)
  if (preference !== null) return Math.min(Math.max(preference, CONTENT_MIN), max)
  return Math.max(680, Math.min(columnWidth * 0.64, 920))
}