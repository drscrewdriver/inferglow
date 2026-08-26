import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createTrafficStore } from './trafficStore'
import type { TrafficDeps } from './trafficStore'
import type { QueueOpBody, RunJob } from './types'

/** Mockable api double surfaced as a store slice via its `trafficApi` dep. */
function makeStore() {
  const sent: Array<{ sessionId: string; agentId: string; text: string }> = []
  const stopped: string[] = []
  const ops: Array<{ runId: string; kind: string }> = []
  const chatSend = vi.fn(async (sessionId: string, agentId: string, text: string) => {
    sent.push({ sessionId, agentId, text })
  })
  const chatStop = vi.fn(() => stopped.push('stop'))
  const mockApi = {
    queue: vi.fn(async (runId: string, op: QueueOpBody) => {
      ops.push({ runId, kind: op.kind })
      return { queue: [], count: 0 }
    }),
    jobs: vi.fn(async () => [] as RunJob[]),
  }
  const deps: TrafficDeps = { chatSend, chatStop, trafficApi: mockApi }
  const store = createTrafficStore(deps)
  return { store, sent, stopped, ops, chatSend, chatStop, mockApi }
}

describe('trafficStore — enqueue / edit / remove / steer / clear', () => {
  let h: ReturnType<typeof makeStore>
  beforeEach(() => {
    h = makeStore()
    h.store.getState().setSession('s1')
  })

  it('enqueues items into the active session tier', () => {
    h.store.getState().enqueue('第一条', 'later')
    h.store.getState().enqueue('第二条', 'now')
    const items = h.store.getState().sessions['s1'].items
    expect(items).toHaveLength(2)
    expect(items[0]).toMatchObject({ text: '第一条', tier: 'later' })
    expect(items[1]).toMatchObject({ text: '第二条', tier: 'now' })
  })

  it('edits the text of an item', () => {
    h.store.getState().enqueue('旧文本', 'next')
    const id = h.store.getState().sessions['s1'].items[0].id
    h.store.getState().edit(id, '新文本')
    expect(h.store.getState().sessions['s1'].items[0].text).toBe('新文本')
  })

  it('removes an item', () => {
    h.store.getState().enqueue('会被删', 'later')
    const id = h.store.getState().sessions['s1'].items[0].id
    h.store.getState().remove(id)
    expect(h.store.getState().sessions['s1'].items).toHaveLength(0)
  })

  it('steer moves an item across tiers', () => {
    h.store.getState().enqueue('移动项', 'later')
    const id = h.store.getState().sessions['s1'].items[0].id
    h.store.getState().steer(id, 'now')
    expect(h.store.getState().sessions['s1'].items[0].tier).toBe('now')
  })

  it('clear empties the queue', () => {
    h.store.getState().enqueue('a', 'later')
    h.store.getState().enqueue('b', 'now')
    h.store.getState().clear()
    expect(h.store.getState().sessions['s1'].items).toHaveLength(0)
  })
})

describe('trafficStore — drain', () => {
  let h: ReturnType<typeof makeStore>
  beforeEach(() => {
    h = makeStore()
    h.store.getState().setSession('s1')
  })

  it('drains by tier priority and FIFO, calling chatSend in order', async () => {
    h.store.getState().enqueue('later-1', 'later')
    h.store.getState().enqueue('now-1', 'now')
    h.store.getState().enqueue('later-2', 'later')
    h.store.getState().enqueue('next-1', 'next')
    const sentTexts = await h.store.getState().drain()
    expect(sentTexts).toEqual(['now-1', 'next-1', 'later-1', 'later-2'])
    // now-cancel semantics: the 'now' item stops the run before send.
    expect(h.stopped).toHaveLength(1)
    expect(h.sent.map((s) => s.text)).toEqual(['now-1', 'next-1', 'later-1', 'later-2'])
    expect(h.store.getState().sessions['s1'].items).toHaveLength(0)
  })

  it('is a no-op while frozen', async () => {
    h.store.getState().enqueue('x', 'later')
    h.store.getState().setFreeze(true)
    const sent = await h.store.getState().drain()
    expect(sent).toEqual([])
    expect(h.chatSend).not.toHaveBeenCalled()
    expect(h.store.getState().sessions['s1'].items).toHaveLength(1)
  })

  it('sends nothing when the queue is empty', async () => {
    const sent = await h.store.getState().drain()
    expect(sent).toEqual([])
  })
})

describe('trafficStore — freeze / resume', () => {
  let h: ReturnType<typeof makeStore>
  beforeEach(() => {
    h = makeStore()
    h.store.getState().setSession('s1')
  })

  it('freeze is isolated per session', () => {
    h.store.getState().setSession('s1')
    h.store.getState().setFreeze(true)
    expect(h.store.getState().sessions['s1'].freeze).not.toBeNull()
    h.store.getState().setSession('s2')
    expect(h.store.getState().sessions['s2'].freeze).toBeNull()
  })

  it('resume with queue mode sends the snapshotted items', async () => {
    h.store.getState().enqueue('A', 'next')
    h.store.getState().enqueue('B', 'later')
    h.store.getState().setFreeze(true)
    h.store.getState().setFreeze(false)
    expect(h.sent.map((s) => s.text).sort()).toEqual(['A', 'B'])
    expect(h.store.getState().sessions['s1'].freeze).toBeNull()
    expect(h.store.getState().sessions['s1'].items).toHaveLength(0)
  })

  it('resume with safe_point steers items back into the queue (no send)', async () => {
    h.store.getState().enqueue('C', 'next')
    h.store.getState().setFreeze(true, 'safe_point')
    h.store.getState().setFreeze(false)
    expect(h.chatSend).not.toHaveBeenCalled()
    const items = h.store.getState().sessions['s1'].items
    expect(items).toHaveLength(1)
    expect(items[0].text).toBe('C')
    expect(items[0].tier).toBe('next')
  })
})

describe('trafficStore — backend sync', () => {
  it('mirrors mutations to the backend when a run id is present', () => {
    const h = makeStore()
    h.store.getState().setSession('s1')
    h.store.getState().setRun('run-7')
    h.store.getState().enqueue('投喂', 'next')
    h.store.getState().enqueue('立即', 'now')
    expect(h.ops).toEqual([
      { runId: 'run-7', kind: 'push' },
      { runId: 'run-7', kind: 'push' },
    ])
  })

  it('stays local (no backend calls) when no run id is set', () => {
    const h = makeStore()
    h.store.getState().setSession('s1')
    h.store.getState().enqueue('本地', 'later')
    expect(h.ops).toEqual([])
  })

  it('does not throw when backend sync fails', async () => {
    const h = makeStore()
    h.store.getState().setSession('s1')
    h.store.getState().setRun('run-1')
    h.mockApi.queue.mockRejectedValueOnce(new Error('boom'))
    await expect(h.store.getState().drain()).resolves.toEqual([])
  })

  it('loads jobs via refreshJobs and can cancel/retry locally', async () => {
    const h = makeStore()
    h.store.getState().setRun('run-1')
    h.mockApi.jobs.mockResolvedValue([
      { id: 'j1', run_id: 'run-1', kind: 'explore', status: 'ongoing', description: '调研', started_at: new Date().toISOString() },
    ] as unknown as RunJob[])
    await h.store.getState().refreshJobs()
    expect(h.store.getState().jobs).toHaveLength(1)

    h.store.getState().cancelJob('j1')
    expect(h.store.getState().jobs[0].status).toBe('killed')
  })
})