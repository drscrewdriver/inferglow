/**
 * Simplified boot kernel for portable dsh-shell.
 * 
 * The original DSH boot uses a full Cordis module system with lazy CJS
 * factories. This portable version:
 *   1. Reads window.__DASH_BOOT__ (injected by host or pre-configured)
 *   2. Fetches plugin bundles sequentially
 *   3. Each bundle calls __DASH_PLUGINS__.set(id, exports)
 *   4. Renders the app shell once all plugins are loaded
 * 
 * This is NOT a Cordis replacement — it's a minimal boot system
 * that lets the UI render and communicate with any backend.
 */

import { createRoot, type Root } from 'react-dom/client'
import type { BootManifest, BootEntry } from './types/boot.ts'
import { AppShell } from './app/App.tsx'

/** Plugin registration map: id → exports */
const pluginRegistry = new Map<string, Record<string, unknown>>()

interface DashBootWindow {
  __DASH_BOOT__?: BootManifest | Record<string, unknown>
  __DASH_PLUGINS__?: { set: (id: string, exports: Record<string, unknown>) => void }
}

/** Global registration function installed on window */
function installPluginLoader(): void {
  const win = window as unknown as DashBootWindow
  win.__DASH_PLUGINS__ = {
    set(id: string, exports: Record<string, unknown>) {
      pluginRegistry.set(id, exports)
    },
  }
}

/**
 * Load a single plugin bundle as a classic script.
 * Each bundle is expected to call window.__DASH_PLUGINS__.set(id, exports)
 * when loaded.
 */
async function loadPluginBundle(entry: BootEntry): Promise<void> {
  const url = entry.rev ? `${entry.url}?rev=${entry.rev}` : entry.url
  return new Promise((resolve, reject) => {
    const script = document.createElement('script')
    script.src = url
    script.async = false // Maintain load order
    script.onload = () => resolve()
    script.onerror = () => reject(new Error(`Failed to load plugin: ${entry.id} from ${url}`))
    document.head.appendChild(script)
  })
}

/** Parse boot manifest with fallback */
function getBootManifest(): BootManifest {
  const win = window as unknown as DashBootWindow
  const raw = win.__DASH_BOOT__
  if (!raw) {
    // Default: empty manifest, no plugins
    return { rev: 'default', entries: [] }
  }
  try {
    return raw as BootManifest
  } catch {
    console.warn('dsh-shell: failed to parse boot manifest, using empty manifest')
    return { rev: 'default', entries: [] }
  }
}

/**
 * Fetch prefetched bundles, then render the app.
 * This is the main boot entry point.
 */
export async function boot(appContainer: HTMLElement): Promise<void> {
  installPluginLoader()

  const manifest = getBootManifest()

  // Prefetch immediately-marked entries
  const immediateEntries = manifest.entries.filter(e => e.immediately)
  if (immediateEntries.length > 0) {
    const results = await Promise.allSettled(
      immediateEntries.map(loadPluginBundle)
    )
    const failed = results.filter((r): r is PromiseRejectedResult => r.status === 'rejected')
    if (failed.length > 0) {
      console.warn(`dsh-shell: ${failed.length} immediate plugins failed to load`)
    }
  }

  // Render app shell (plugins load lazily as needed)
  const root: Root = createRoot(appContainer)
  root.render(<AppShell />)
}
