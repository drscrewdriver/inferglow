// sandbox/sandboxStore.ts — localStorage-backed runtime sandbox selection.

import { create } from 'zustand'
import { DEFAULT_SANDBOX_CONFIG, type PermissionPreset, type SandboxConfig, type SandboxMode, type ShellEnv } from './schema'

const KEY = 'inferglow.sandbox.v1'

interface SandboxState {
  config: SandboxConfig
  setMode: (mode: SandboxMode) => void
  setPreset: (preset: PermissionPreset) => void
  setShell: (shell: ShellEnv) => void
  setRequireEscalationApproval: (v: boolean) => void
  reset: () => void
}

function load(): SandboxConfig {
  try {
    const raw = localStorage.getItem(KEY)
    return raw ? { ...DEFAULT_SANDBOX_CONFIG, ...(JSON.parse(raw) as Partial<SandboxConfig>) } : DEFAULT_SANDBOX_CONFIG
  } catch {
    return DEFAULT_SANDBOX_CONFIG
  }
}

export const useSandboxStore = create<SandboxState>()((set, get) => {
  const patch = (next: Partial<SandboxConfig>) => {
    const config = { ...get().config, ...next }
    try {
      localStorage.setItem(KEY, JSON.stringify(config))
    } catch {
      // ignore
    }
    set({ config })
  }
  return {
    config: load(),
    setMode: (mode) => patch({ mode }),
    setPreset: (preset) => patch({ preset }),
    setShell: (shell) => patch({ shell }),
    setRequireEscalationApproval: (v) => patch({ requireEscalationApproval: v }),
    reset: () => {
      try {
        localStorage.removeItem(KEY)
      } catch {
        // ignore
      }
      set({ config: { ...DEFAULT_SANDBOX_CONFIG } })
    },
  }
})