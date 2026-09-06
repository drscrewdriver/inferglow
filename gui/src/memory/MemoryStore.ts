import { create } from 'zustand'
import { transport } from '../api'
import type { MemoryRecord } from '../api'
import { createMemoryApi, type MemoryApi } from './MemoryApi'

export interface MemoryState {
  memories: MemoryRecord[]
  loading: boolean
  error: string | null
  /** Re-fetch the memory list from the backend. */
  refresh: () => Promise<void>
  /** Delete a memory; optimistically removes it locally on success. */
  remove: (id: string) => Promise<void>
}

export function createMemoryStore(api: MemoryApi) {
  return create<MemoryState>()((set) => ({
    memories: [],
    loading: false,
    error: null,

    async refresh() {
      set({ loading: true, error: null })
      try {
        const memories = await api.list(20)
        set({ memories, loading: false })
      } catch (e) {
        // silent degradation: panel falls back to the empty state
        set({ memories: [], loading: false, error: e instanceof Error ? e.message : '加载失败' })
      }
    },

    async remove(id) {
      try {
        await api.remove(id)
        set((s) => ({ memories: s.memories.filter((m) => m.id !== id) }))
      } catch {
        // silent degradation: keep the item on failure
      }
    },
  }))
}

/** Default app instance wired to the shared transport. */
export const useMemoryStore = createMemoryStore(createMemoryApi(transport))
