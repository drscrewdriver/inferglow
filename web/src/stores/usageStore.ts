import { create } from 'zustand'
import { transport, type CacheReport, type Transport } from '../api'

interface UsageState {
  report: CacheReport | null
  loading: boolean
  error: string | null
  fetchReport: () => Promise<void>
}

export const createUsageStore = (t: Transport) =>
  create<UsageState>()((set) => ({
    report: null,
    loading: false,
    error: null,

    async fetchReport() {
      set({ loading: true, error: null })
      try {
        const report = await t.request<CacheReport>('GET', '/usage/report')
        set({ report, loading: false })
      } catch (err) {
        set({ loading: false, error: err instanceof Error ? err.message : String(err) })
      }
    },
  }))

export const useUsageStore = createUsageStore(transport)
