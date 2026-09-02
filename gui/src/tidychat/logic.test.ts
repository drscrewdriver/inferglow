import { describe, expect, it } from 'vitest'
import { cleanTiming, decideFold, foldLabel, turnGrouping } from './logic'
import type { TzNode } from './logic'

const msg = (role: 'user' | 'assistant' | 'tool' | 'command', content = '', think?: string): TzNode => ({
  kind: role,
  message: { content, think },
})
/** turn-tail nodes carry no message in the real flow; widen for timing tests. */
const tail = (content: string): TzNode =>
  ({ kind: 'turn-tail', message: { content } }) as unknown as TzNode

describe('cleanTiming', () => {
  it('extracts the last mm:ss before 用时 plus the following text', () => {
    expect(cleanTiming('思考 00:12 · 工具 03:45 用时 12.3s 20 tok/s')).toBe('03:45 · 用时 12.3s 20 tok/s')
  })
  it('returns empty when there is no 用时 marker', () => {
    expect(cleanTiming('本次对话结束')).toBe('')
  })
  it('caps trailing text at 50 chars when no tok/s', () => {
    const long = 'x'.repeat(80)
    const out = cleanTiming(`用时 ${long}`)
    expect(out.length).toBeLessThanOrEqual('用时 '.length + 50)
  })
  it('stops at tok/s boundary', () => {
    expect(cleanTiming('用时 1.2s 340 tok/s 尾部被截断')).toBe('用时 1.2s 340 tok/s')
  })
  it('handles empty input', () => {
    expect(cleanTiming('')).toBe('')
  })
})

describe('turnGrouping', () => {
  it('groups assistant/tool steps into one turn per user boundary', () => {
    const nodes: TzNode[] = [
      msg('user', '你好'),
      msg('tool', '', undefined),
      msg('assistant', '思考中', '心里想的'),
      msg('assistant', '最终回复'),
      msg('user', '再来'),
      msg('assistant', '回复二'),
      tail('本次对话结束'),
    ]
    const turns = turnGrouping(nodes, false)
    expect(turns).toHaveLength(2)
    expect(turns[0].turn).toBe(0)
    expect(turns[0].steps).toEqual([2, 3])
    expect(turns[0].toolCalls).toBe(1)
    expect(turns[0].finalIndex).toBe(3) // last step with body
    expect(turns[0].hasTail).toBe(true)
    expect(turns[1].turn).toBe(1)
    expect(turns[1].finalIndex).toBe(5)
    expect(turns[1].hasTail).toBe(true)
  })

  it('keeps the final turn incomplete while streaming', () => {
    const nodes: TzNode[] = [msg('user', 'q'), msg('assistant', '')]
    const streaming = turnGrouping(nodes, true)
    expect(streaming[0].hasTail).toBe(false)
    const done = turnGrouping(nodes, false)
    expect(done[0].hasTail).toBe(true)
  })

  it('assigns timings from a trailing turn-tail when present', () => {
    const nodes: TzNode[] = [msg('user', 'q'), msg('assistant', 'a'), tail('用时 00:05 30 tok/s')]
    const turns = turnGrouping(nodes, false)
    expect(turns[0].timing).toContain('00:05')
  })

  it('marks a turn without body text with finalIndex -1', () => {
    const nodes: TzNode[] = [msg('user', 'q'), msg('assistant', '')]
    const turns = turnGrouping(nodes, false)
    expect(turns[0].finalIndex).toBe(-1)
  })
})

describe('decideFold + foldLabel', () => {
  const nodes: TzNode[] = [msg('user', 'q'), msg('tool', ''), msg('assistant', '中间'), msg('assistant', '正文', '想法')]
  const turn = turnGrouping(nodes, false)[0]

  it('hides non-final steps/tools and the final think when folded', () => {
    const d = decideFold(turn, nodes, true, true)
    expect(d.hiddenIndexes).toEqual([1, 2])
    expect(d.finalThinkHidden).toBe(true)
  })

  it('hides nothing and keeps think when expanded', () => {
    const d = decideFold(turn, nodes, false, true)
    expect(d.hiddenIndexes).toEqual([])
    expect(d.finalThinkHidden).toBe(false)
  })

  it('hides everything when disabled fold hides intermediates regardless', () => {
    const d = decideFold(turn, nodes, true, false)
    expect(d.hiddenIndexes).toEqual([])
  })

  it('builds a 过程 N 步 label that mirrors the reference', () => {
    expect(foldLabel({ ...turn, timing: '00:02 · tips' }, true, true)).toContain('过程 3 步')
    expect(foldLabel({ ...turn, timing: '00:02 · tips' }, false, true)).toContain('已展开 3 步')
  })
})