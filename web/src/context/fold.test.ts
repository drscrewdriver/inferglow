import { describe, expect, it } from 'vitest'
import type { ChatMessage } from '../stores/chatStore'
import { fold, aggregateByTurn, deltaOf, legendOf, makeComposition } from './fold'
import { SYSTEM_PROMPT_EST, TOOLS_SCHEMA_EST } from './token'

const NO = { createdAt: 0 } as Partial<ChatMessage>

function user(text: string): ChatMessage {
  return { id: 'u', role: 'user', content: text, createdAt: 1, ...(NO as object) } as ChatMessage
}
function cmd(text: string): ChatMessage {
  return { id: 'c', role: 'user', content: '/think ' + text, createdAt: 1, ...(NO as object) } as ChatMessage
}
function assistant(text: string): ChatMessage {
  return { id: 'a', role: 'assistant', content: text, createdAt: 1, ...(NO as object) } as ChatMessage
}
function tool(name: string, content: string, status = 'ok'): ChatMessage {
  return { id: 't', role: 'tool', toolName: name, toolStatus: status, content, createdAt: 1, ...(NO as object) } as ChatMessage
}

describe('makeComposition', () => {
  it('sums the six buckets into total', () => {
    const c = makeComposition([1, 2, 3, 4, 5, 6])
    expect(c.total).toBe(21)
  })
})

describe('fold', () => {
  it('produces an empty projection for no messages', () => {
    const p = fold([])
    expect(p.requests).toHaveLength(0)
    expect(p.events).toHaveLength(0)
    expect(p.files).toHaveLength(0)
    expect(p.current.total).toBe(SYSTEM_PROMPT_EST + TOOLS_SCHEMA_EST)
  })

  it('chains one turn = user + assistant, carrying user tokens into composition', () => {
    const p = fold([user('你好'), assistant('世界')])
    expect(p.requests).toHaveLength(1)
    const r = p.requests[0]
    expect(r.turn).toBe(1)
    expect(r.step).toBe(1)
    expect(r.tokens.system).toBe(SYSTEM_PROMPT_EST)
    expect(r.tokens.tools).toBe(TOOLS_SCHEMA_EST)
    // 你好 = 2 chars → ceil(2/4)=1
    expect(r.tokens.user).toBe(1)
    // inject re-accounted at turn start
    expect(r.tokens.inject).toBe(SYSTEM_PROMPT_EST + TOOLS_SCHEMA_EST)
    expect(r.tokens.assistant).toBe(1)
    // first request has no net delta
    expect(r.net).toBeUndefined()
  })

  it('second assistant step in the same turn does NOT re-inject', () => {
    const p = fold([cmd('a'), assistant('A'), tool('read_file', '/x'), assistant('B')])
    expect(p.requests).toHaveLength(2)
    expect(p.requests[0].step).toBe(1)
    expect(p.requests[0].tokens.inject).toBe(SYSTEM_PROMPT_EST + TOOLS_SCHEMA_EST)
    expect(p.requests[1].step).toBe(2)
    expect(p.requests[1].tokens.inject).toBe(0)
    // tool batch price lands on the following assistant record
    expect(p.requests[1].tokens.tool).toBeGreaterThan(0)
  })

  it('emits an inject event at each new turn, and a model event per request', () => {
    const p = fold([user('q1'), assistant('a1'), user('q2'), assistant('a2')], { model: 'demo' })
    const injects = p.events.filter((e) => e.kind === 'inject')
    expect(injects).toHaveLength(2)
    const models = p.events.filter((e) => e.kind === 'model')
    expect(models).toHaveLength(2)
    expect(models[0].name).toBe('demo')
  })

  it('captures file ops from tool calls and counts tool calls', () => {
    const p = fold([
      user('q'),
      assistant('a'),
      tool('read_file', 'read /src/a.go'),
      tool('write_file', 'write /src/b.go:\nbody'),
      tool('web_search', 'search node'),
      assistant('done'),
    ])
    const kinds = p.files.map((f) => f.kind).sort()
    expect(kinds).toEqual(['read', 'write'])
    expect(p.toolCalls.get('read_file')).toBe(1)
    // web_search is not a file op
    expect(p.images).toBe(0)
  })

  it('counts image tool calls toward the images stat', () => {
    const p = fold([user('q'), assistant('a'), tool('image_analyze', 'image /x.png'), assistant('b')])
    expect(p.images).toBe(1)
  })

  it('net delta appears from the second request onward', () => {
    const p = fold([user('q1'), assistant('aa'), user('q2'), assistant('bb')])
    expect(p.requests[0].net).toBeUndefined()
    expect(typeof p.requests[1].net).toBe('number')
  })
})

describe('aggregateByTurn', () => {
  it('keeps one bar per turn, stamped with the step count', () => {
    const p = fold([user('q'), assistant('a'), tool('read_file', '/x'), assistant('b')])
    const byTurn = aggregateByTurn(p.requests)
    expect(byTurn).toHaveLength(1)
    expect(byTurn[0].stepCount).toBe(2)
  })
})

describe('deltaOf', () => {
  it('computes signed net change vs the previous total', () => {
    const p = fold([user('a'), assistant('aaaa'), user('b'), assistant('bbbbbbbb')])
    const d = deltaOf(p.requests)
    // delta mode stamps every bar; the first diffs against an empty baseline.
    expect(typeof d[0].net).toBe('number')
    expect(d[1].net).toBeDefined()
  })
})

describe('legendOf', () => {
  it('returns all six buckets with percentages summing to ~100', () => {
    const c = makeComposition([10, 10, 10, 10, 20, 40])
    const legend = legendOf(c)
    expect(legend).toHaveLength(6)
    const sum = legend.reduce((a, b) => a + b.pct, 0)
    expect(sum).toBeCloseTo(100, 1)
  })
})