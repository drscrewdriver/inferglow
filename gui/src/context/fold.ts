// Pure fold: derive a context Projection from InferGlow's ChatMessage stream.
// Mirrors dsh-context's host fold concepts (turn/step chaining, six-bucket
// per-request composition, inject events, file op extraction) but operates on
// the flat message array InferGlow keeps — no harness projection dependency.

import type { ChatMessage } from '../stores/chatStore'
import { estimateTokens, fileOpKindOf, extractPath, SYSTEM_PROMPT_EST, TOOLS_SCHEMA_EST } from './token'
import type { Composition, FileOpKind, Projection, ProjectionEvent, ProjectionRequest } from './types'

/** Fold options. */
export interface FoldOptions {
  contextWindow?: number
  model?: string
  provider?: string
}

/** Build a short semantic brief for a request record (spec Task 15). */
function briefOf(m: ChatMessage): { opener?: string; inputs?: string; response?: string } {
  const text = m.content?.trim() ?? ''
  const sliced = text.length > 72 ? text.slice(0, 72) + '…' : text
  if (m.role === 'tool' && m.toolName) return { inputs: `${m.toolName}: ${sliced || '(…)'}` }
  if (m.role === 'user') return { opener: sliced || '(空输入)' }
  return { response: sliced || '(思考中…)' }
}

export function makeComposition(parts: readonly number[]): Composition {
  return {
    system: parts[0],
    tools: parts[1],
    user: parts[2],
    inject: parts[3],
    assistant: parts[4],
    tool: parts[5],
    total: parts[0] + parts[1] + parts[2] + parts[3] + parts[4] + parts[5],
  }
}

/** Fold the flat message array into a context Projection. */
export function fold(messages: readonly ChatMessage[], opts: FoldOptions = {}): Projection {
  const requests: ProjectionRequest[] = []
  const events: ProjectionEvent[] = []
  const projected: Projection = {
    model: opts.model,
    provider: opts.provider,
    contextWindow: opts.contextWindow,
    current: makeComposition([SYSTEM_PROMPT_EST, TOOLS_SCHEMA_EST, 0, 0, 0, 0]),
    requests,
    events,
    files: [],
    toolCalls: new Map(),
    images: 0,
  }
  const pendingFiles: NonNullable<Projection['files']> = projected.files

  let turn = 0
  let step = 0
  let seq = 0
  const time = 0
  let pendingUserTokens = 0
  let curToolBatch = 0
  let lastTotal = 0
  let images = 0

  for (const m of messages) {
    if (m.role === 'user') {
      turn += 1
      step = 0
      pendingUserTokens = estimateTokens(m.content)
      curToolBatch = 0
      continue
    }
    if (m.role === 'tool') {
      seq += 1
      curToolBatch += estimateTokens(m.content)
      if (m.toolName) projected.toolCalls.set(m.toolName, (projected.toolCalls.get(m.toolName) ?? 0) + 1)
      const kind = fileOpKindOf(m.toolName)
      if (kind) {
        pendingFiles.push({
          seq,
          time: time || Date.now(),
          turn,
          step,
          kind: kind as FileOpKind,
          path: extractPath(m.content) ?? m.toolName!,
          summary: m.content ? m.content.slice(0, 80) : undefined,
        })
      }
      if (kind === 'image') images += 1
      continue
    }
    if (m.role === 'assistant') {
      step += 1
      seq += 1
      const isTurnStart = step === 1
      const inject = SYSTEM_PROMPT_EST + TOOLS_SCHEMA_EST
      const comp = makeComposition([
        SYSTEM_PROMPT_EST,
        TOOLS_SCHEMA_EST,
        pendingUserTokens,
        isTurnStart ? inject : 0,
        estimateTokens(m.content),
        curToolBatch,
      ])
      if (isTurnStart) {
        events.push({
          seq,
          time: time || Date.now(),
          kind: 'inject',
          name: 'system+tools',
          detail: `注入 ${inject} tokens`,
          tokens: inject,
        })
      }
      events.push({ seq, time: time || Date.now(), kind: 'model', name: opts.model, detail: opts.provider })
      requests.push({
        turn,
        step,
        seq,
        time: time || Date.now(),
        tokens: comp,
        cacheRead: 0,
        output: comp.assistant,
        net: requests.length > 0 ? comp.total - lastTotal : undefined,
        ...briefOf(m),
      })
      lastTotal = comp.total
      curToolBatch = 0
      continue
    }
  }

  projected.current = requests.length > 0 ? requests[requests.length - 1].tokens : projected.current
  projected.images = images
  return projected
}

/** Aggregate one bar per turn (each turn's LAST step record, plus stepCount). */
export function aggregateByTurn(requests: readonly ProjectionRequest[]): ProjectionRequest[] {
  const byTurn = new Map<number, ProjectionRequest>()
  const counts = new Map<number, number>()
  for (const r of requests) {
    counts.set(r.turn, (counts.get(r.turn) ?? 0) + 1)
    byTurn.set(r.turn, r)
  }
  return [...byTurn.values()].map((r) => ({
    ...r,
    stepCount: counts.get(r.turn),
    net: undefined,
  }))
}

/** Signed net change vs the previous request (delta-mode plotting). */
export function deltaOf(requests: readonly ProjectionRequest[]): ProjectionRequest[] {
  const out: ProjectionRequest[] = []
  let prev = 0
  for (const r of requests) {
    out.push({ ...r, net: r.tokens.total - prev })
    prev = r.tokens.total
  }
  return out
}

/** Six-bucket legend with percentages for a composition. */
export function legendOf(
  comp: Readonly<Composition>,
): { key: keyof Omit<Composition, 'total'>; tokens: number; pct: number }[] {
  const order: (keyof Omit<Composition, 'total'>)[] = ['system', 'tools', 'user', 'inject', 'assistant', 'tool']
  const total = comp.total || 1
  return order.map((key) => ({ key, tokens: comp[key], pct: (comp[key] / total) * 100 }))
}