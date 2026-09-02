import { describe, expect, it } from 'vitest'
import { parseSSE, splitFrames } from './sse'

describe('splitFrames', () => {
  it('parses a complete event/data frame', () => {
    const { frames, rest } = splitFrames('event: run_start\ndata: {"type":"run_start"}\n\n')
    expect(frames).toHaveLength(1)
    expect(frames[0].event).toBe('run_start')
    expect(JSON.parse(frames[0].data)).toEqual({ type: 'run_start' })
    expect(rest).toBe('')
  })

  it('parses multiple frames in one buffer', () => {
    const { frames, rest } = splitFrames(
      'event: a\ndata: 1\n\nevent: b\ndata: 2\n\n',
    )
    expect(frames).toHaveLength(2)
    expect(frames.map((f) => f.event)).toEqual(['a', 'b'])
    expect(rest).toBe('')
  })

  it('keeps a trailing partial frame in rest for the next chunk', () => {
    const { frames, rest } = splitFrames('event: tool_start\ndata: {"type":')
    expect(frames).toHaveLength(0)
    expect(rest).toBe('event: tool_start\ndata: {"type":')
  })

  it('handles CRLF line endings', () => {
    const { frames } = splitFrames('event: x\r\ndata: y\r\n\r\n')
    expect(frames).toHaveLength(1)
    expect(frames[0].event).toBe('x')
    expect(frames[0].data).toBe('y')
  })
})

function streamResponse(chunks: Uint8Array[]): Response {
  const stream = new ReadableStream<Uint8Array>({
    start(controller) {
      for (const c of chunks) controller.enqueue(c)
      controller.close()
    },
  })
  return new Response(stream)
}

describe('parseSSE', () => {
  it('collects frames split across chunk boundaries', async () => {
    const payload =
      'event: run_start\ndata: {"type":"run_start"}\n\n' +
      'event: tool_start\ndata: {"type":"tool_start","tool_name":"explore"}\n\n' +
      'event: done\ndata: {"agent_id":"a1"}\n\n'
    const encoder = new TextEncoder()
    // Deliberately cut in the middle of a frame and of a UTF-8 sequence.
    const bytes = encoder.encode(payload)
    const mid = Math.floor(bytes.length / 2)
    const resp = streamResponse([bytes.slice(0, mid), bytes.slice(mid)])

    const events: string[] = []
    await parseSSE(resp, (f) => events.push(f.event))
    expect(events).toEqual(['run_start', 'tool_start', 'done'])
  })

  it('decodes UTF-8 multi-byte characters split between chunks', async () => {
    const frame = 'event: x\ndata: {"name":"中文内容"}\n\n'
    const bytes = new TextEncoder().encode(frame)
    const cut = bytes.indexOf('中'.charCodeAt(0) & 0xff) // arbitrary byte cut
    const c = cut >= 0 && cut > 0 ? cut : 4
    const resp = streamResponse([bytes.slice(0, c), bytes.slice(c)])

    const datas: string[] = []
    await parseSSE(resp, (f) => datas.push(f.data))
    expect(datas).toHaveLength(1)
    expect(JSON.parse(datas[0])).toEqual({ name: '中文内容' })
  })

  it('stops early when the signal aborts', async () => {
    const frame = 'event: a\ndata: 1\n\n'
    const encoder = new TextEncoder()
    const bytes = encoder.encode(frame)
    // Infinite-ish stream: never closes, but enqueues one frame.
    const stream = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(bytes)
        // keep the stream open
      },
    })
    const controller = new AbortController()
    const resp = new Response(stream)

    const events: string[] = []
    const p = parseSSE(resp, (f) => events.push(f.event), controller.signal)
    // Give the reader a tick, then abort.
    await new Promise((r) => setTimeout(r, 10))
    controller.abort()
    await p
    expect(events).toEqual(['a'])
  })
})
