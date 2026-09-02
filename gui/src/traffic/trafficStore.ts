import { create } from 'zustand'
import { orderedQueue } from './tiers'
import type { QueueTier } from './tiers'
import { createFreeze, planResume } from './freeze'
import type { FreezeMode, FreezeRecord } from './freeze'
import type { TrafficApi } from './trafficApi'
import { createTrafficApi } from './trafficApi'
import { useChatStore } from '../stores/chatStore'
import { transport } from '../api'
import type { QueueOpBody, RunJob } from './types'

/** A client-side queue entry for the traffic dock. */
export interface TrafficQueueItem {
  id: string
  tier: QueueTier
  text: string
  createdAt: number
}

/** Per-session traffic state (queues + freeze are isolated by session). */
export interface SessionTraffic {
  items: TrafficQueueItem[]
  freeze: FreezeRecord | null
}

/** Injectable dependencies: chat wiring + backend api (both mockable). */
export interface TrafficDeps {
  chatStop: () => void
  chatSend: (sessionId: string, agentId: string, text: string) => Promise<void> | void
  trafficApi: TrafficApi
}

let uid = 0
const newId = () => `tq-${Date.now()}-${++uid}`

export interface TrafficState {
  activeSessionId: string | null
  agentId: string
  /** Backend run id; when null all mutations stay local (silent degrade). */
  runId: string | null
  sessions: Record<string, SessionTraffic>
  jobs: RunJob[]

  setSession: (id: string | null) => void
  setRun: (runId: string | null) => void
  setAgent: (agentId: string) => void
  enqueue: (text: string, tier: QueueTier) => void
  edit: (itemId: string, text: string) => void
  remove: (itemId: string) => void
  steer: (itemId: string, tier: QueueTier, toFront?: boolean) => void
  move: (itemId: string, dir: -1 | 1) => void
  clear: () => void
  setFreeze: (on: boolean, mode?: FreezeMode) => void
  drain: () => Promise<string[]>
  refreshJobs: () => Promise<void>
  cancelJob: (jobId: string) => void
  retryJob: (jobId: string) => Promise<void>
}

export function createTrafficStore({ chatStop, chatSend, trafficApi }: TrafficDeps) {
  return create<TrafficState>()((set, get) => {
    const emptySession = (): SessionTraffic => ({ items: [], freeze: null })
    const sessionOf = (id: string | null = get().activeSessionId): SessionTraffic | null =>
      id ? (get().sessions[id] ?? null) : null

    const patchItems = (id: string, items: TrafficQueueItem[]) =>
      set({
        sessions: {
          ...get().sessions,
          [id]: { ...(get().sessions[id] ?? emptySession()), items },
        },
      })

    /** Mirror a mutation to the backend; silently skip when no run id. */
    const mirror = (op: QueueOpBody): void => {
      const { runId } = get()
      if (!runId) return
      trafficApi.queue(runId, op).catch(() => {
        // silent degradation: local is always the source of truth for the UI
      })
    }

    return {
      activeSessionId: null,
      agentId: 'a1',
      runId: null,
      sessions: {},
      jobs: [],

      setSession(id) {
        const sessions = { ...get().sessions }
        if (id && !sessions[id]) sessions[id] = emptySession()
        set({ activeSessionId: id, sessions })
      },

      setRun(runId) {
        set({ runId })
      },

      setAgent(agentId) {
        set({ agentId })
      },

      enqueue(text, tier) {
        const id = get().activeSessionId
        const s = sessionOf(id)
        if (!id || !s || !text.trim()) return
        const item: TrafficQueueItem = { id: newId(), tier, text: text.trim(), createdAt: Date.now() }
        patchItems(id, [...s.items, item])
        mirror({ kind: 'push', text: item.text, tier })
      },

      edit(itemId, text) {
        const id = get().activeSessionId
        const s = sessionOf(id)
        if (!id || !s) return
        const items = s.items.map((i) => (i.id === itemId ? { ...i, text } : i))
        patchItems(id, items)
        mirror({ kind: 'edit', item_id: itemId, text })
      },

      remove(itemId) {
        const id = get().activeSessionId
        const s = sessionOf(id)
        if (!id || !s) return
        patchItems(id, s.items.filter((i) => i.id !== itemId))
        mirror({ kind: 'remove', item_id: itemId })
      },

      steer(itemId, tier, toFront = false) {
        const id = get().activeSessionId
        const s = sessionOf(id)
        if (!id || !s) return
        const items = s.items
        const idx = items.findIndex((i) => i.id === itemId)
        if (idx === -1) return
        const moved = { ...items[idx], tier }
        const rest = items.filter((i) => i.id !== itemId)
        patchItems(id, toFront ? [moved, ...rest] : [...rest, moved])
        mirror({ kind: 'steer', item_id: itemId, tier, to_front: toFront })
      },

      move(itemId, dir) {
        const id = get().activeSessionId
        const s = sessionOf(id)
        if (!id || !s) return
        const items = [...s.items]
        const idx = items.findIndex((i) => i.id === itemId)
        if (idx === -1) return
        const tier = items[idx].tier
        // Scan for the nearest sibling of the same tier in the given direction.
        let j = idx + dir
        while (j >= 0 && j < items.length && items[j].tier !== tier) j += dir
        if (j < 0 || j >= items.length) return
        ;[items[idx], items[j]] = [items[j], items[idx]]
        patchItems(id, items)
      },

      clear() {
        const id = get().activeSessionId
        const s = sessionOf(id)
        if (!id || !s) return
        patchItems(id, [])
        mirror({ kind: 'clear' })
      },

      setFreeze(on, mode = 'queue') {
        const id = get().activeSessionId
        const s = sessionOf(id)
        if (!id || !s) return

        if (on) {
          const record = createFreeze(
            id,
            mode,
            '',
            s.items.map((it) => ({ id: it.id, tier: it.tier, text: it.text })),
          )
          set({ sessions: { ...get().sessions, [id]: { ...s, freeze: record } } })
          return
        }

        const rec = s.freeze
        if (!rec) return
        const steps = planResume(rec)
        const items = [...s.items]
        const removed = new Set<string>()
        for (const step of steps) {
          if (step.action === 'cancel') {
            chatStop()
          } else if (step.action === 'send') {
            if (!step.text) continue
            void Promise.resolve(chatSend(id, get().agentId, step.text)).catch(() => {
              // local send failure is surfaced by chatStore itself
            })
            const hit = items.findIndex((i) => i.text === step.text && !removed.has(i.id))
            if (hit !== -1) removed.add(items[hit].id)
          } else if (step.action === 'steer') {
            const found = items.findIndex((i) => i.id === step.item.id)
            if (found === -1) items.push({ ...step.item, createdAt: Date.now() })
            else items[found] = { ...items[found], tier: step.item.tier }
          }
        }
        set({ sessions: { ...get().sessions, [id]: { items: items.filter((i) => !removed.has(i.id)), freeze: null } } })
      },

      async drain() {
        const id = get().activeSessionId
        const s = sessionOf(id)
        if (!id || !s || s.freeze) return []
        const ordered = orderedQueue(s.items)
        if (ordered.length === 0) return []
        const agent = get().agentId
        const sent: string[] = []
        for (const item of ordered) {
          if (item.tier === 'now') chatStop()
          await Promise.resolve(chatSend(id, agent, item.text)).catch(() => {
            // chatStore already surfaces stream errors; continue draining
          })
          sent.push(item.text)
        }
        patchItems(id, [])
        mirror({ kind: 'clear' })
        return sent
      },

      async refreshJobs() {
        const { runId } = get()
        if (!runId) {
          set({ jobs: [] })
          return
        }
        try {
          const jobs = await trafficApi.jobs(runId)
          set({ jobs })
        } catch {
          // silent degradation when the endpoint is unavailable
        }
      },

      cancelJob(jobId) {
        const now = new Date().toISOString()
        set({ jobs: get().jobs.map((j) => (j.id === jobId ? { ...j, status: 'killed', finished_at: now } : j)) })
      },

      async retryJob(jobId) {
        const job = get().jobs.find((j) => j.id === jobId)
        const id = get().activeSessionId
        if (!job || !id || !job.description) return
        await Promise.resolve(chatSend(id, get().agentId, job.description)).catch(() => {
          // failure surfaced through chatStore
        })
      },
    }
  })
}

// ─── Selectors ────────────────────────────────────────────────────────────
// Zustand's default equality is Object.is: selectors MUST return stable
// references (cached) or the snapshot identity changes every check and React
// falls into a "getSnapshot should be cached" infinite loop.
// Stable empty reference shared by selectItems (never mutated; the store only
// ever replaces items arrays immutably, so sharing is safe).
const EMPTY_ITEMS: TrafficQueueItem[] = []

export const selectItems = (s: TrafficState): TrafficQueueItem[] => {
  const id = s.activeSessionId
  return id ? (s.sessions[id]?.items ?? EMPTY_ITEMS) : EMPTY_ITEMS
}
// Group arrays are cached by the underlying items-array identity, which is
// replaced immutably in the store, so the filter result stays stable until the
// items actually change.
const tierGroupCache = new WeakMap<TrafficQueueItem[], Map<QueueTier, TrafficQueueItem[]>>()
export const selectTierGroup = (s: TrafficState, tier: QueueTier): TrafficQueueItem[] => {
  const items = selectItems(s)
  let byTier = tierGroupCache.get(items)
  if (!byTier) {
    byTier = new Map()
    tierGroupCache.set(items, byTier)
  }
  let group = byTier.get(tier)
  if (!group) {
    group = items.filter((i) => i.tier === tier)
    byTier.set(tier, group)
  }
  return group
}
export const selectCount = (s: TrafficState): number => selectItems(s).length
export const selectFreeze = (s: TrafficState): FreezeRecord | null => {
  const id = s.activeSessionId
  return id ? (s.sessions[id]?.freeze ?? null) : null
}
export const selectFrozen = (s: TrafficState): boolean => selectFreeze(s) !== null
export const selectJobs = (s: TrafficState): RunJob[] => s.jobs

// ─── Default app instance (wired to chat + transport) ─────────────────────
export const useTrafficStore = createTrafficStore({
  chatStop: () => useChatStore.getState().stop(),
  chatSend: (sessionId, agentId, text) => useChatStore.getState().sendMessage(sessionId, agentId, text),
  trafficApi: createTrafficApi(transport),
})