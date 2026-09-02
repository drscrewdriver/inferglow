import { afterEach, describe, expect, it, vi } from 'vitest'
import { restTransport } from './transport'

/** Minimal localStorage double for the node test environment. */
function stubLocalStorage(entries: Record<string, string> = {}): void {
  const store = new Map(Object.entries(entries))
  ;(globalThis as Record<string, unknown>).localStorage = {
    getItem: (k: string) => store.get(k) ?? null,
    setItem: (k: string, v: string) => void store.set(k, v),
    removeItem: (k: string) => void store.delete(k),
    clear: () => store.clear(),
    key: (i: number) => [...store.keys()][i] ?? null,
    get length() {
      return store.size
    },
  }
}

afterEach(() => {
  vi.restoreAllMocks()
  delete (globalThis as Record<string, unknown>).localStorage
})

describe('restTransport auth header', () => {
  it('omits Authorization when no API key is stored', async () => {
    stubLocalStorage({})
    const fetchMock = vi.fn().mockResolvedValue(new Response('{}', { status: 200, headers: { 'Content-Type': 'application/json' } }))
    vi.stubGlobal('fetch', fetchMock)

    await restTransport.request('GET', '/sessions')
    const init = fetchMock.mock.calls[0][1] as RequestInit
    expect(init.headers).not.toContain('Authorization')
  })

  it('attaches Bearer token when inferglow.apikey is stored', async () => {
    stubLocalStorage({ 'inferglow.apikey': 'sk-test-123' })
    const fetchMock = vi.fn().mockResolvedValue(new Response('{}', { status: 200, headers: { 'Content-Type': 'application/json' } }))
    vi.stubGlobal('fetch', fetchMock)

    await restTransport.request('GET', '/sessions')
    const init = fetchMock.mock.calls[0][1] as RequestInit
    const headers = init.headers as Record<string, string>
    expect(headers.Authorization).toBe('Bearer sk-test-123')
  })

  it('attaches Bearer token on SSE stream requests too', async () => {
    stubLocalStorage({ 'inferglow.apikey': 'sk-test-456' })
    const fetchMock = vi.fn().mockResolvedValue(
      new Response('event: done\ndata: {"agent_id":"a1"}\n\n', {
        status: 200,
        headers: { 'Content-Type': 'text/event-stream' },
      }),
    )
    vi.stubGlobal('fetch', fetchMock)

    await restTransport.streamRun(
      'a1',
      { message: 'hi' },
      { onEvent: () => {} },
      new AbortController().signal,
    )
    const init = fetchMock.mock.calls[0][1] as RequestInit
    const headers = init.headers as Record<string, string>
    expect(headers.Authorization).toBe('Bearer sk-test-456')
  })
})
