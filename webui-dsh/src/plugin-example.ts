#!/usr/bin/env node
/**
 * Example plugin — demonstrates how plugins register with dsh-shell.
 * 
 * Each plugin bundle should:
 * 1. Export a default function that receives (ctx, options)
 * 2. Call window.__DASH_PLUGINS__.set('plugin-id', exports)
 * 
 * In development, this is just a demonstration.
 * In production, this would be fetched as a script from /plugins/example/client.js
 */

interface PluginContext {
  [key: string]: unknown
}

interface PluginOptions {
  [key: string]: unknown
}

interface PluginMessage {
  [key: string]: unknown
}

interface DashWindowWithPlugins {
  __DASH_PLUGINS__?: { set: (id: string, exports: Record<string, unknown>) => void }
}

// Simulated plugin registration (for demo purposes)
const win = window as unknown as DashWindowWithPlugins
if (typeof window !== 'undefined' && win.__DASH_PLUGINS__) {
  win.__DASH_PLUGINS__.set('example-plugin', {
    name: 'example-plugin',
    version: '1.0.0',
    onMessage: (message: PluginMessage) => console.log('[example-plugin] received:', message),
  })
}

export const name = 'example-plugin'
export const version = '1.0.0'

export function examplePlugin(_ctx: PluginContext, _options: PluginOptions = {}) {
  return {
    name: 'example-plugin',
    onMessage(message: PluginMessage) {
      console.log('[example-plugin]', message)
    },
  }
}
