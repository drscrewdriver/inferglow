/**
 * tidychat — turn grouping, timing extraction and fold decisions (pure logic).
 *
 * Ported from the dsh-tidychat fold surgery but re-expressed as pure functions
 * over the FlowNode message sequence, so conversation components derive the
 * DOM (fold control bars, hidden steps, think/body dividers) from data instead
 * of running a MutationObserver DOM-surgery automaton.
 */

/** Minimal structural node shape handled by grouping (FlowNode is compatible). */
export type TzNode =
  | { kind: 'user' | 'assistant' | 'tool' | 'command'; message: { content: string; think?: string } }
  | { kind: 'compaction' | 'turn-error' | 'turn-tail' }

/** One assistant/tool run between two user boundaries. */
export interface TzTurn {
  /** Stable per-session turn ordinal (the foldState/nav/anchor key). */
  turn: number
  /** Node indexes of the assistant steps that belong to this turn. */
  steps: number[]
  /** Node indexes of the tool-call cards in this turn. */
  toolIndexes: number[]
  toolCalls: number
  /** True when the turn is finished (a later user follows, or streaming stopped). */
  hasTail: boolean
  /** Human timing summary extracted from the trailing turn-tail text (may be ''). */
  timing: string
  /** Index of the first member node (where the fold control bar is inserted). */
  firstIndex: number
  /** Index of the final step that carries body text to keep visible, or -1. */
  finalIndex: number
}

function isBoundary(n: TzNode): boolean {
  return n.kind === 'user' || n.kind === 'command'
}

/**
 * Extract a compact timing label from a raw turn-tail line. The reference
 * looks for the last `mm:ss` segment before 「用时」 plus the 50 chars after.
 */
export function cleanTiming(raw: string): string {
  if (typeof raw !== 'string' || raw === '') return ''
  const yongshi = raw.indexOf('用时')
  if (yongshi === -1) return ''
  const before = raw.slice(0, yongshi)
  const times = before.match(/\d{1,2}:\d{2}/g)
  const lead = times !== null && times.length > 0 ? times[times.length - 1] : ''
  const rest = raw.slice(yongshi)
  const tok = rest.indexOf('tok/s')
  const body = tok === -1 ? rest.slice(0, 50) : rest.slice(0, tok + 5)
  return (lead !== '' ? lead + ' · ' : '') + body
}

/** Does a TzNode carry visible body text? (think-only rows do not.) */
function hasBody(node: TzNode | undefined): boolean {
  if (node === undefined) return false
  if ('message' in node) return typeof node.message.content === 'string' && node.message.content !== ''
  return false
}

/**
 * Split the message flow into turns. A turn starts at the first assistant step
 * (or leading tool call) after a user/command boundary and runs until the next
 * boundary. Only completed turns are foldable: any turn that is not the last is
 * always complete; the final turn completes when streaming is no longer active.
 */
export function turnGrouping(nodes: readonly TzNode[], streaming: boolean): TzTurn[] {
  const turns: TzTurn[] = []
  let cur: TzTurn | null = null
  let ordinal = 0

  for (let i = 0; i < nodes.length; i++) {
    const n = nodes[i]
    if (isBoundary(n)) {
      cur = null
      continue
    }
    if (n.kind === 'assistant') {
      if (cur === null) {
        cur = { turn: ordinal++, steps: [], toolIndexes: [], toolCalls: 0, hasTail: false, timing: '', firstIndex: i, finalIndex: -1 }
        turns.push(cur)
      }
      cur.steps.push(i)
    } else if (n.kind === 'tool') {
      if (cur === null) {
        cur = { turn: ordinal++, steps: [], toolIndexes: [], toolCalls: 0, hasTail: false, timing: '', firstIndex: i, finalIndex: -1 }
        turns.push(cur)
      }
      cur.toolIndexes.push(i)
      cur.toolCalls += 1
    } else if (n.kind === 'turn-tail') {
      if (cur !== null && 'message' in n) {
        // A turn-tail may optionally carry timing text on a message payload.
        const tail = n as unknown as { message?: { content?: string } }
        if (typeof tail.message?.content === 'string') cur.timing = cleanTiming(tail.message.content)
      }
      cur = null
    }
  }

  // Complete turns + resolve the final visible step.
  for (let ti = 0; ti < turns.length; ti++) {
    const t = turns[ti]
    const isFinalTurn = ti === turns.length - 1
    t.hasTail = isFinalTurn ? !streaming : true
    let finalIdx = -1
    for (let s = t.steps.length - 1; s >= 0; s--) {
      const idx = t.steps[s]
      if (hasBody(nodes[idx])) {
        finalIdx = idx
        break
      }
    }
    t.finalIndex = finalIdx
  }

  return turns
}

export interface FoldDecision {
  /** Node indexes to visually hide when the turn is folded. */
  hiddenIndexes: number[]
  /** True when the kept final step's think line should also be hidden. */
  finalThinkHidden: boolean
}

/**
 * Map a completed turn + fold flag onto concrete DOM decisions. Also lets the
 * caller compute the control-bar label via `totalSteps`.
 */
export function decideFold(turn: TzTurn, nodes: readonly TzNode[], folded: boolean, enabled: boolean): FoldDecision {
  const hiddenIndexes: number[] = []
  let finalThinkHidden = false
  if (enabled && folded) {
    // Preserve original encounter order (steps + tool cards interleave).
    const members = [...turn.steps, ...turn.toolIndexes].sort((a, b) => a - b)
    for (const m of members) if (m !== turn.finalIndex) hiddenIndexes.push(m)
    if (turn.finalIndex !== -1) {
      const final = nodes[turn.finalIndex]
      if (final && 'message' in final && typeof (final as Extract<TzNode, { message: { think?: string } }>).message.think === 'string' && (final as Extract<TzNode, { message: { think?: string } }>).message.think !== '') {
        finalThinkHidden = true
      }
    }
  }
  return { hiddenIndexes, finalThinkHidden }
}

/** Label for a fold control bar: 「过程 N 步」+ timing (mirrors reference). */
export function foldLabel(turn: TzTurn, folded: boolean, enabled: boolean): string {
  const thinkCount = enabled ? turn.steps.length : 0
  const totalSteps = thinkCount + turn.toolCalls
  const parts = [folded ? `过程 ${totalSteps} 步` : `已展开 ${totalSteps} 步`]
  if (turn.timing !== '') parts.push(turn.timing)
  return parts.join(' · ')
}

/** Button text (展开 / 收起) for a fold control bar. */
export function foldButtonLabel(folded: boolean): string {
  return folded ? '展开' : '收起'
}