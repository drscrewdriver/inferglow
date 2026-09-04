/**
 * Portable boot manifest types.
 * 
 * This is a simplified version of the DSH boot protocol. The original
 * uses a complex Cordis module system with lazy CJS factories;
 * this portable version uses a simpler approach:
 *   - Backend serves bundles from /plugins/<id>/client.js
 *   - Boot manifest declares which bundles to fetch
 *   - Each bundle registers its plugin via window.__DASH_PLUGINS__
 */

/** A single boot entry: bundle URL + metadata */
export interface BootEntry {
  /** Plugin id (used as namespace) */
  id: string
  /** Bundle URL — relative to the page or absolute */
  url: string
  /** Content hash for cache busting */
  rev?: string
  /** Package-name dependency edges (informational) */
  inject?: string[]
  /** Stage-one prefetch: load immediately */
  immediately?: boolean
}

/** The boot manifest served/injected by the host */
export interface BootManifest {
  /** Consistency anchor over the whole graph */
  rev: string
  /** Boot entries to fetch */
  entries: BootEntry[]
}

/** The window API for the portable boot protocol */
export interface DashWindow {
  /** Boot manifest, injected by host before shell loads */
  __DASH_BOOT__?: BootManifest | Record<string, unknown>
  /** Plugin registration sink, installed by boot kernel */
  __DASH_PLUGINS__?: Map<string, Record<string, unknown>>
}

/** Parse the boot manifest from the window */
export function parseBootManifest(wire: unknown): BootManifest {
  if (typeof wire !== 'object' || wire === null) {
    throw new Error('dsh-shell: __DASH_BOOT__ is missing or not an object')
  }
  const raw = wire as Record<string, unknown>
  if (!Array.isArray(raw.entries)) {
    throw new Error('dsh-shell: boot manifest entries must be an array')
  }
  const entries: BootEntry[] = []
  for (const value of raw.entries) {
    if (typeof value !== 'object' || value === null) continue
    const row = value as Record<string, unknown>
    if (typeof row.id !== 'string' || typeof row.url !== 'string') continue
    entries.push({
      id: row.id,
      url: row.url,
      rev: typeof row.rev === 'string' ? row.rev : undefined,
      inject: Array.isArray(row.inject) ? row.inject as string[] : undefined,
      immediately: row.immediately === true,
    })
  }
  return {
    rev: typeof raw.rev === 'string' ? raw.rev : 'unknown',
    entries,
  }
}
