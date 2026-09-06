import { create } from 'zustand'
import { DEFAULT_SETTINGS, SETTINGS_VERSION, type AppSettings } from './settingsSchema'

function load(): AppSettings {
  try {
    const raw = localStorage.getItem(SETTINGS_VERSION)
    const parsed = raw ? (JSON.parse(raw) as Partial<AppSettings>) : {}
    const apiKey = localStorage.getItem('inferglow.apikey') ?? ''
    return {
      ...DEFAULT_SETTINGS,
      ...parsed,
      apiKey,
      shortcuts: { ...DEFAULT_SETTINGS.shortcuts, ...(parsed.shortcuts ?? {}) },
    }
  } catch {
    return DEFAULT_SETTINGS
  }
}

interface SettingsState {
  settings: AppSettings
  set: (patch: Partial<AppSettings>) => void
  reset: () => void
}

export const useSettingsStore = create<SettingsState>()((set, get) => ({
  settings: load(),
  set: (patch) => {
    const next = { ...get().settings, ...patch }
    try {
      localStorage.setItem(SETTINGS_VERSION, JSON.stringify(next))
      // The API key travels separately so transport can read it without
      // parsing the full settings blob.
      if (patch.apiKey !== undefined) {
        if (patch.apiKey) localStorage.setItem('inferglow.apikey', patch.apiKey)
        else localStorage.removeItem('inferglow.apikey')
      }
    } catch {
      // localStorage unavailable (private mode etc.) — keep in-memory state.
    }
    set({ settings: next })
  },
  reset: () => {
    try {
      localStorage.removeItem(SETTINGS_VERSION)
      localStorage.removeItem('inferglow.apikey')
    } catch {
      // ignore
    }
    set({ settings: DEFAULT_SETTINGS })
  },
}))
