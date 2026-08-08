import { create } from 'zustand'
import { DEFAULT_SETTINGS, SETTINGS_VERSION, type AppSettings } from './settingsSchema'

function load(): AppSettings {
  try {
    const raw = localStorage.getItem(SETTINGS_VERSION)
    if (!raw) return DEFAULT_SETTINGS
    const parsed = JSON.parse(raw) as Partial<AppSettings>
    return { ...DEFAULT_SETTINGS, ...parsed, shortcuts: { ...DEFAULT_SETTINGS.shortcuts, ...(parsed.shortcuts ?? {}) } }
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
    } catch {
      // localStorage unavailable (private mode etc.) — keep in-memory state.
    }
    set({ settings: next })
  },
  reset: () => {
    try {
      localStorage.removeItem(SETTINGS_VERSION)
    } catch {
      // ignore
    }
    set({ settings: DEFAULT_SETTINGS })
  },
}))
