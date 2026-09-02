import { create } from 'zustand'
import { transport } from '../api'
import type { RunRecord } from '../api'
import { createTodoApi, type TodoApi } from './TodoApi'

/** A single todo row derived from a run step. */
export interface TodoItem {
  step: string
  duration?: string
  /** Done when the backend reported no error for the step. */
  done: boolean
}

export interface TodoState {
  runs: RunRecord[]
  items: TodoItem[]
  runLabel: string
  loading: boolean
  error: string | null
  /** Re-fetch runs and the most recent run's steps from the backend. */
  refresh: () => Promise<void>
}

/** Pick the run most likely to be the "current" one (latest start). */
function newestRun(runs: RunRecord[]): RunRecord | null {
  if (runs.length === 0) return null
  return [...runs].sort((a, b) => {
    const ta = a.started_at ? Date.parse(a.started_at) : 0
    const tb = b.started_at ? Date.parse(b.started_at) : 0
    return tb - ta
  })[0]
}

export function createTodoStore(api: TodoApi) {
  return create<TodoState>()((set) => ({
    runs: [],
    items: [],
    runLabel: '',
    loading: false,
    error: null,

    async refresh() {
      set({ loading: true, error: null })
      try {
        const runs = await api.listRuns()
        const target = newestRun(runs)
        const items = target
          ? (await api.steps(target.run_id)).map((s) => ({ step: s.step, duration: s.duration, done: !s.error }))
          : []
        set({
          runs,
          items,
          runLabel: target ? `${target.flow ?? 'run'} · ${target.run_id}` : '',
          loading: false,
        })
      } catch (e) {
        // silent degradation: panel falls back to the empty state
        set({ runs: [], items: [], runLabel: '', loading: false, error: e instanceof Error ? e.message : '加载失败' })
      }
    },
  }))
}

/** Default app instance wired to the shared transport. */
export const useTodoStore = createTodoStore(createTodoApi(transport))
