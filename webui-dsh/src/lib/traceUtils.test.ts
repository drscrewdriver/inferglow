import { describe, it, expect } from 'vitest'
import { tracesToSpans } from './traceUtils.ts'

describe('tracesToSpans', () => {
  it('parses persisted run summaries into span lines', () => {
    const spans = tracesToSpans([
      {
        content: JSON.stringify({
          agent_id: 'fake',
          start: '2026-01-15T10:30:00Z',
          duration: '1.5s',
          spans: [
            { kind: 'agent', name: 'inferglow.agent.run', duration_ms: 1500 },
            { kind: 'llm', name: 'inferglow.llm.call.0', duration_ms: 900 },
          ],
        }),
        created_at: '2026-01-15T10:30:02Z',
      },
    ])
    expect(spans).toHaveLength(2)
    expect(spans[0]).toMatchObject({
      name: 'inferglow.agent.run',
      kind: 'agent',
      duration_ns: 1.5e9,
      has_error: false,
    })
    // end = start + duration_ms
    expect(Date.parse(spans[0].end_time)).toBe(Date.parse('2026-01-15T10:30:00Z') + 1500)
  })

  it('marks spans with error', () => {
    const spans = tracesToSpans([
      {
        content: JSON.stringify({
          start: '2026-01-15T10:30:00Z',
          spans: [{ kind: 'tool', name: 'inferglow.tool.write_file', duration_ms: 10, error: true }],
        }),
        created_at: '2026-01-15T10:30:01Z',
      },
    ])
    expect(spans[0].has_error).toBe(true)
  })

  it('skips corrupt JSON lines and empty span arrays (old snapshots degrade)', () => {
    const spans = tracesToSpans([
      { content: 'not-json', created_at: '2026-01-15T10:30:00Z' },
      { content: JSON.stringify({ start: '2026-01-15T10:30:00Z' }), created_at: '2026-01-15T10:30:01Z' },
    ])
    expect(spans).toHaveLength(0)
  })

  it('falls back to created_at when start is missing', () => {
    const spans = tracesToSpans([
      {
        content: JSON.stringify({ spans: [{ kind: 'llm', name: 'x', duration_ms: 1000 }] }),
        created_at: '2026-01-15T10:30:00Z',
      },
    ])
    expect(Date.parse(spans[0].end_time)).toBe(Date.parse('2026-01-15T10:30:00Z') + 1000)
  })
})
