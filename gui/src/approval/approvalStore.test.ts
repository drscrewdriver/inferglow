import { describe, expect, it } from 'vitest'
import type { Transport } from '../api'
import { createApprovalStore } from './approvalStore'

/** In-memory transport double for /v1/approvals. */
function approvalTransport(): Transport & { calls: string[] } {
  const calls: string[] = []
  return {
    calls,
    async request<T>(method: string, path: string, body?: unknown): Promise<T> {
      calls.push(`${method} ${path}`)
      if (path === '/approvals' && method === 'GET') {
        return {
          approvals: [
            { id: 'p1', request: { capability: 'bash_execute', subject: 'rm -rf /tmp/x', risk: 'high' }, status: 'pending', created_at: '2026-08-13T00:00:00Z' },
            { id: 'r1', request: { capability: 'network_access', subject: 'curl', risk: 'low' }, status: 'approved', approver: 'webui', created_at: '2026-08-12T00:00:00Z' },
          ],
          count: 2,
        } as T
      }
      if (path === '/approvals' && method === 'POST') {
        return { id: 'p2', request: { capability: 'bash_execute', subject: 'ls' }, status: 'pending', created_at: '2026-08-13T00:00:00Z' } as T
      }
      if (path.startsWith('/approvals/') && method === 'POST') {
        const id = path.split('/')[2]
        return { id, status: (body as { approve?: boolean }).approve ? 'approved' : 'denied', created_at: '2026-08-13T00:00:00Z' } as T
      }
      throw new Error(`unexpected ${method} ${path}`)
    },
    async streamRun(): Promise<void> {
      throw new Error('not used')
    },
  }
}

describe('approvalStore', () => {
  it('splits pending vs history on fetch', async () => {
    const t = approvalTransport()
    const store = createApprovalStore(t)
    await store.getState().fetch()
    expect(store.getState().pending).toHaveLength(1)
    expect(store.getState().history).toHaveLength(1)
    expect(store.getState().pending[0].id).toBe('p1')
  })

  it('decide moves a pending rec into history', async () => {
    const t = approvalTransport()
    const store = createApprovalStore(t)
    await store.getState().fetch()
    const ok = await store.getState().decide({ recordId: 'p1', approve: true })
    expect(ok).toBe(true)
    expect(store.getState().pending).toHaveLength(0)
    expect(store.getState().history[0].id).toBe('p1')
    expect(store.getState().history[0].status).toBe('approved')
    expect(t.calls).toContain('POST /approvals/p1/decision')
  })

  it('decide returns false on error', async () => {
    const t = approvalTransport()
    const failing: Transport = { ...t, request: async () => { throw new Error('conflict') } }
    const store = createApprovalStore(failing)
    const ok = await store.getState().decide({ recordId: 'boom', approve: true })
    expect(ok).toBe(false)
    expect(store.getState().error).toBe('conflict')
  })

  it('submit adds a pending rec to the pending list', async () => {
    const t = approvalTransport()
    const store = createApprovalStore(t)
    const rec = await store.getState().submit({ capability: 'bash_execute', subject: 'ls' })
    expect(rec?.id).toBe('p2')
    expect(store.getState().pending[0].id).toBe('p2')
  })
})