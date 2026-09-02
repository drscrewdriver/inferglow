import { useCallback, useEffect, useMemo } from 'react'
import type { StoreApi } from 'zustand'
import { useStore as useZustandStore } from 'zustand'
import type { SessionRecord } from '../api'
import { useSessionStore, type SessionQuery } from '../stores/sessionStore'

/** Generic selector hook over any zustand store (thin typed wrapper). */
export function useStore<TState, TResult>(
  store: StoreApi<TState>,
  selector: (state: TState) => TResult,
): TResult {
  return useZustandStore(store, selector)
}

/** Time basis for fallback date-sorting when updated_at is absent/equal. */
export const UNGROUPED_LABEL = '未归类'

export interface SessionSortMode {
  /** pinned-first is the default; secondary always falls back to recency. */
  pinned?: 'first'
  by: 'updated' | 'title'
}

/** Sort sessions within the caller's chosen mode (pinned first, then by). */
export function sortSessions(list: SessionRecord[], by: 'updated' | 'title'): SessionRecord[] {
  const pinnedFirst = (a: SessionRecord, b: SessionRecord) => Number(b.pinned ?? false) - Number(a.pinned ?? false)
  const ts = (s: SessionRecord) => Date.parse(s.updated_at) || 0
  const sorted = [...list].sort((a, b) => pinnedFirst(a, b))
  if (by === 'title') {
    return sorted.sort((a, b) => a.title!.localeCompare(b.title!))
  }
  return sorted.sort((a, b) => ts(b) - ts(a) || a.title!.localeCompare(b.title!))
}

export interface SessionGroup {
  group: string
  sessions: SessionRecord[]
}

/** Group a list by SessionRecord.group (empty/absent → 未归类). */
export function groupSessions(list: SessionRecord[]): SessionGroup[] {
  const map = new Map<string, SessionRecord[]>()
  for (const s of list) {
    const key = s.group || UNGROUPED_LABEL
    const arr = map.get(key)
    if (arr) arr.push(s)
    else map.set(key, [s])
  }
  return [...map.entries()].map(([group, sessions]) => ({ group, sessions }))
}

/** The currently active session, or null when none is selected. */
export function useSession(): SessionRecord | null {
  const activeId = useSessionStore((s) => s.activeId)
  const sessions = useSessionStore((s) => s.sessions)
  return useMemo(() => sessions.find((s) => s.id === activeId) ?? null, [sessions, activeId])
}

export interface UseSessionsOptions extends SessionQuery {
  /** Debounce (ms) before the remote fetch fires on q changes. 0 = immediate. */
  debounce?: number
  /** Local title/group substring filter applied on top of the remote filter. */
  q?: string
}

export interface UseSessionsResult {
  /** Fully filtered (remote + local) flat session list. */
  sessions: SessionRecord[]
  /** Sessions grouped by workspace, each group sorted by recency. */
  byGroup: SessionGroup[]
  loading: boolean
  error: string | null
  /** Manually re-run the remote fetch with the current options. */
  refetch: () => void
}

/**
 * Fetch sessions through the Phase-1 backend filters (q/group/pinned) with a
 * short debounce, then apply a local metadata filter and group them. Returns
 * both the flat list and the per-workspace view.
 */
export function useSessions(options: UseSessionsOptions = {}): UseSessionsResult {
  const { q, group, pinned, debounce = 250 } = options
  const sessions = useSessionStore((s) => s.sessions)
  const loading = useSessionStore((s) => s.loading)
  const error = useSessionStore((s) => s.error)
  const fetchSessions = useSessionStore((s) => s.fetchSessions)

  const query: SessionQuery = useMemo(
    () => ({ q: q || undefined, group: group || undefined, pinned: pinned ?? undefined }),
    [q, group, pinned],
  )

  const fetchNow = useCallback(() => void fetchSessions(query), [fetchSessions, query])

  // Fire on mount and whenever the backend filter changes (debounced for q).
  useEffect(() => {
    const timer = setTimeout(fetchNow, debounce)
    return () => clearTimeout(timer)
  }, [fetchNow, debounce])

  // Local metadata filter (title/group) applied on top of whatever the server
  // returned, so the client can narrow further without a round-trip.
  const localQ = (q ?? '').trim().toLowerCase()
  const filtered = useMemo(() => {
    if (!localQ) return sessions
    return sessions.filter((s) => {
      const title = (s.title ?? '').toLowerCase().includes(localQ)
      const grp = (s.group ?? UNGROUPED_LABEL).toLowerCase().includes(localQ)
      return title || grp
    })
  }, [sessions, localQ])

  const byGroup = useMemo(
    () => groupSessions(filtered).map((g) => ({ group: g.group, sessions: sortSessions(g.sessions, 'updated') })),
    [filtered],
  )

  return { sessions: filtered, byGroup, loading, error, refetch: fetchNow }
}