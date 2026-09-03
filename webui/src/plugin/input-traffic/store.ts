/* input-traffic 插件 — 拖拽队列状态(三层) + 会话级冻结状态 freezeStore */

import { create } from 'zustand'
import { trafficApi } from './api'
import type { QueueTier, QueueOpBody, RunJob } from './api'

/* ────────────────────────────────────────────────────────────────
 * 三层队列语义（later / next / now）
 * ──────────────────────────────────────────────────────────────── */
export const TIER_ORDER: Record<QueueTier, number> = { later: 0, next: 1, now: 2 }
export const TIER_LABEL: Record<QueueTier, string> = { later: '稍后', next: '下一步', now: '立即' }
export const TIER_ICON: Record<QueueTier, string> = { later: '🟢', next: '🟡', now: '🔴' }
export const TIERS: QueueTier[] = ['later', 'next', 'now']

/** 按层级优先级(now→next→later)稳定排序；同层级保持 FIFO。 */
export function orderedQueue<T extends { id: string; tier: QueueTier; text: string }>(items: T[]): T[] {
  return [...items].sort((a, b) => TIER_ORDER[b.tier] - TIER_ORDER[a.tier])
}

/* ────────────────────────────────────────────────────────────────
 * 冻结 / 恢复规划（纯函数）
 * ──────────────────────────────────────────────────────────────── */
export type FreezeMode = 'force' | 'cancel' | 'safe_point' | 'queue'

export interface FreezeItem {
  id: string
  tier: QueueTier
  text: string
}

export interface FreezeRecord {
  sessionId: string
  mode: FreezeMode
  /** force/cancel 恢复路径下暂存的输入文本 */
  text: string
  /** 冻结时刻的队列项快照 */
  items: FreezeItem[]
  at: number
}

export type ResumeStep =
  | { action: 'cancel' }
  | { action: 'send'; text: string }
  | { action: 'steer'; item: FreezeItem }

export function createFreeze(
  sessionId: string,
  mode: FreezeMode,
  text = '',
  items: FreezeItem[] = [],
  at = Date.now(),
): FreezeRecord {
  return { sessionId, mode, text, items, at }
}

/**
 * 将冻结记录翻译为确定性的恢复操作序列（纯函数，不改状态本身）。
 * - force/cancel → 取消当前运行，重发暂存文本
 * - safe_point  → 把快照项放回队列（不发送）
 * - queue       → 直接发送每个快照项
 */
export function planResume(rec: FreezeRecord): ResumeStep[] {
  switch (rec.mode) {
    case 'force':
    case 'cancel': {
      const text = rec.text || rec.items[0]?.text || ''
      const steps: ResumeStep[] = [{ action: 'cancel' }]
      if (text) steps.push({ action: 'send', text })
      return steps
    }
    case 'safe_point':
      return rec.items.map((item) => ({ action: 'steer', item }))
    case 'queue':
      return rec.items.filter((i) => i.text).map((item) => ({ action: 'send', text: item.text }))
  }
}

/* ────────────────────────────────────────────────────────────────
 * 聊天接线（webui 无独立 chatStore，由宿主注入）
 * ──────────────────────────────────────────────────────────────── */
export interface ChatAdapter {
  chatStop: () => void
  chatSend: (sessionId: string, agentId: string, text: string) => Promise<void> | void
}

let chat: ChatAdapter = {
  chatStop: () => {},
  chatSend: () => Promise.resolve(),
}

/** 允许宿主（如 Task7 集成）在运行时接入真实发送/停止逻辑。 */
export function setChatAdapter(partial: Partial<ChatAdapter>): void {
  chat = { ...chat, ...partial }
}

/* ────────────────────────────────────────────────────────────────
 * 队列 store（按会话隔离）
 * ──────────────────────────────────────────────────────────────── */
export interface TrafficQueueItem {
  id: string
  tier: QueueTier
  text: string
  createdAt: number
}

interface QueueSession {
  items: TrafficQueueItem[]
}

let uid = 0
const newId = () => `tq-${Date.now()}-${++uid}`

interface TrafficState {
  activeSessionId: string | null
  runId: string | null
  agentId: string
  sessions: Record<string, QueueSession>
  jobs: RunJob[]

  setSession: (id: string | null) => void
  setRun: (runId: string | null) => void
  setAgent: (agentId: string) => void
  enqueue: (text: string, tier: QueueTier) => void
  edit: (itemId: string, text: string) => void
  remove: (itemId: string) => void
  /** 按计划的插入档位操纵一条队列项（对齐 DSH 三档语义）。 */
  steer: (itemId: string, tier: QueueTier, toFront?: boolean) => void
  move: (itemId: string, dir: -1 | 1) => void
  clear: () => void
  /** 手动整体投放（对齐 DSH 的显式 flush；不再被 auto-drain 隐式触发）。 */
  flush: () => Promise<string[]>
  refreshJobs: () => Promise<void>
  /** 由 freezeStore 在恢复(safe_point)时回写队列；内部使用。 */
  restoreItems: (sessionId: string, items: TrafficQueueItem[]) => void
}

export const useTrafficStore = create<TrafficState>()((set, get) => {
  const emptySession = (): QueueSession => ({ items: [] })
  const sessionOf = (id: string | null = get().activeSessionId): QueueSession | null =>
    id ? (get().sessions[id] ?? null) : null

  const patchItems = (id: string, items: TrafficQueueItem[]) =>
    set({
      sessions: {
        ...get().sessions,
        [id]: { ...(get().sessions[id] ?? emptySession()), items },
      },
    })

  /** 将变更镜像到后端；无 runId 或调用失败时静默（本地始终为 UI 真相源）。 */
  const mirror = (op: QueueOpBody): void => {
    const { runId } = get()
    if (!runId) return
    trafficApi.queue(runId, op).catch(() => {
      // silent degradation
    })
  }

  return {
    activeSessionId: null,
    runId: null,
    agentId: 'a1',
    sessions: {},
    jobs: [],

    setSession(id) {
      const sessions = { ...get().sessions }
      if (id && !sessions[id]) sessions[id] = emptySession()
      set({ activeSessionId: id, sessions })
      useFreezeStore.getState().setSession(id)
    },

    setRun(runId) {
      set({ runId })
    },

    setAgent(agentId) {
      set({ agentId })
    },

    enqueue(text, tier = 'later') {
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
      patchItems(id, s.items.map((i) => (i.id === itemId ? { ...i, text } : i)))
      mirror({ kind: 'edit', item_id: itemId, text })
    },

    remove(itemId) {
      const id = get().activeSessionId
      const s = sessionOf(id)
      if (!id || !s) return
      patchItems(id, s.items.filter((i) => i.id !== itemId))
      mirror({ kind: 'remove', item_id: itemId })
    },

    /**
     * 按计划的插入档位操纵一条队列项（对齐 DSH steer 语义）：
     * - now    → 打断当前运行（chatStop），从队列移除该项，并把文本作为新消息重发；
     * - next   → 重档到 next：当前 action 结束后插入（保持排队，不打断）；
     * - later  → 重档回 later：排到下一轮（撤销 next 的档位）。
     */
    steer(itemId, tier, toFront = false) {
      const id = get().activeSessionId
      const s = sessionOf(id)
      if (!id || !s) return
      const item = s.items.find((i) => i.id === itemId)
      if (!item) return
      if (tier === 'now') {
        // 打断 + 移除 + 重发：重新武装 wake latch，把文字作为下一轮的输入。
        chat.chatStop()
        patchItems(id, s.items.filter((i) => i.id !== itemId))
        mirror({ kind: 'remove', item_id: itemId })
        void Promise.resolve(chat.chatSend(id, get().agentId, item.text)).catch(() => {
          // 发送失败由聊天层自行呈现
        })
        return
      }
      const moved = { ...item, tier }
      const rest = s.items.filter((i) => i.id !== itemId)
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

    restoreItems(sessionId, items) {
      const sessions = { ...get().sessions }
      sessions[sessionId] = { ...(sessions[sessionId] ?? emptySession()), items }
      set({ sessions })
    },

    async flush() {
      const id = get().activeSessionId
      const s = sessionOf(id)
      if (!id || !s || useFreezeStore.getState().isFrozen(id)) return []
      const ordered = orderedQueue(s.items)
      if (ordered.length === 0) return []
      const sent: string[] = []
      for (const item of ordered) {
        if (item.tier === 'now') chat.chatStop()
        await Promise.resolve(chat.chatSend(id, get().agentId, item.text)).catch(() => {
          // 继续排空
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
        set({ jobs: await trafficApi.jobs(runId) })
      } catch {
        // 静默降级
      }
    },
  }
})

/* 队列 store 选择器：必须返回稳定引用，否则 Zustand 默认 Object.is 比较会触发
 * "getSnapshot should be cached" 死循环。 */
const EMPTY_ITEMS: TrafficQueueItem[] = []

export const selectItems = (s: TrafficState): TrafficQueueItem[] => {
  const id = s.activeSessionId
  return id ? (s.sessions[id]?.items ?? EMPTY_ITEMS) : EMPTY_ITEMS
}

export const selectCount = (s: TrafficState): number => selectItems(s).length

/**
 * 投放位置（对齐 input-traffic 的 placement 语义）：
 * - queued   → 主队列（later / now 档位），渲染在扁平主列表
 * - steering → 已投放入运行轮次（next 档位），渲染在独立 steering 列表，可撤销回 later
 *
 * 注意：`.filter()` 每次都会产出新数组引用，Zustand 默认用 Object.is 比较，
 * 会让组件无限重渲染（"getSnapshot should be cached" 死循环）。因此按源数组
 * 引用做 WeakMap 一级缓存，只有源数组真正变化（patchItems 总是新建数组）时才重算。
 */
const queuedCache = new WeakMap<TrafficQueueItem[], TrafficQueueItem[]>()
const steeringCache = new WeakMap<TrafficQueueItem[], TrafficQueueItem[]>()
function filterCached(
  src: TrafficQueueItem[],
  cache: WeakMap<TrafficQueueItem[], TrafficQueueItem[]>,
  pred: (i: TrafficQueueItem) => boolean,
): TrafficQueueItem[] {
  // EMPTY_ITEMS 恒定，恒返回 EMPTY_ITEMS 以保引用稳定
  if (src === EMPTY_ITEMS) return EMPTY_ITEMS
  let hit = cache.get(src)
  if (!hit) {
    hit = src.filter(pred)
    cache.set(src, hit)
  }
  return hit
}

export const selectQueued = (s: TrafficState): TrafficQueueItem[] =>
  filterCached(selectItems(s), queuedCache, (i) => i.tier !== 'next')
export const selectSteering = (s: TrafficState): TrafficQueueItem[] =>
  filterCached(selectItems(s), steeringCache, (i) => i.tier === 'next')

/* ────────────────────────────────────────────────────────────────
 * 会话级冻结 store（与队列 store 并存、互相协调）
 * ──────────────────────────────────────────────────────────────── */
interface FreezeState {
  activeSessionId: string | null
  sessions: Record<string, FreezeRecord | null>

  setSession: (id: string | null) => void
  setFreeze: (on: boolean, mode?: FreezeMode) => void
  isFrozen: (sessionId?: string | null) => boolean
}

export const useFreezeStore = create<FreezeState>()((set, get) => ({
  activeSessionId: null,
  sessions: {},

  setSession(id) {
    set({ activeSessionId: id })
  },

  setFreeze(on, mode = 'queue') {
    const id = get().activeSessionId
    if (!id) return

    if (on) {
      const qs = useTrafficStore.getState()
      const snapshots: FreezeItem[] = (qs.sessions[id]?.items ?? []).map((i) => ({
        id: i.id,
        tier: i.tier,
        text: i.text,
      }))
      const record = createFreeze(id, mode, '', snapshots)
      set({ sessions: { ...get().sessions, [id]: record } })
      return
    }

    const rec = get().sessions[id]
    if (!rec) return
    const steps = planResume(rec)
    const qs = useTrafficStore.getState()
    const items: TrafficQueueItem[] = [...(qs.sessions[id]?.items ?? [])]
    const removed = new Set<string>()
    for (const step of steps) {
      if (step.action === 'cancel') {
        chat.chatStop()
      } else if (step.action === 'send') {
        if (!step.text) continue
        void Promise.resolve(chat.chatSend(id, qs.agentId, step.text)).catch(() => {
          // 发送失败由聊天层自行呈现
        })
        const hit = items.findIndex((i) => i.text === step.text && !removed.has(i.id))
        if (hit !== -1) removed.add(items[hit].id)
      } else if (step.action === 'steer') {
        const found = items.findIndex((i) => i.id === step.item.id)
        if (found === -1) items.push({ ...step.item, createdAt: Date.now() })
        else items[found] = { ...items[found], tier: step.item.tier }
      }
    }
    useTrafficStore.getState().restoreItems(id, items.filter((i) => !removed.has(i.id)))
    set({ sessions: { ...get().sessions, [id]: null } })
  },

  isFrozen(sessionId) {
    const id = sessionId ?? get().activeSessionId
    return id ? get().sessions[id] != null : false
  },
}))

/* 冻结 store 选择器 */
export const selectFreeze = (s: FreezeState): FreezeRecord | null => {
  const id = s.activeSessionId
  return id ? (s.sessions[id] ?? null) : null
}

export const selectFrozen = (s: FreezeState): boolean => selectFreeze(s) !== null