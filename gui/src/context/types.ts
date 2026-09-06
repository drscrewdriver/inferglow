// Context visualization data model (Phase 4, aligned to dsh-context's snapshot
// contract enriched onto InferGlow's ChatMessage stream). Pure types shared by
// the fold (fold.ts) and the view components.

/** The six token buckets the composition/browser/trend render. dsh-context
 * folds 'system' and 'tools' separately from the four surface categories; the
 * slice model keeps them distinct too so the stacked layers stay truthful. */
export type Category = 'system' | 'tools' | 'user' | 'inject' | 'assistant' | 'tool'

export const CATEGORIES: readonly Category[] = [
  'system',
  'tools',
  'user',
  'inject',
  'assistant',
  'tool',
] as const

/** Per-category token sums for one surface snapshot. */
export interface Composition {
  system: number
  tools: number
  user: number
  inject: number
  assistant: number
  tool: number
  readonly total: number
}

/** One answered model call (a step). Consecutive records of one turn chain. */
export interface ProjectionRequest {
  turn: number
  step: number
  /** Number of steps aggregated when trend is turn-mode (last step's record). */
  stepCount?: number
  seq: number
  time: number
  tokens: Readonly<Composition>
  /** Billed cache-read prompt tokens (hit-rate numerator) when known. */
  cacheRead?: number
  /** Billed output tokens when known. */
  output?: number
  /** Net change vs the previous request (delta-mode plotting). */
  net?: number
  /** Student semantic summary of the round (spec Task 15). */
  opener?: string
  inputs?: string
  response?: string
}

/** A notable context event (compaction/prune/injection/model switch). */
export interface ProjectionEvent {
  seq: number
  time: number
  kind: 'compaction' | 'prune' | 'inject' | 'model' | 'mode'
  name?: string
  detail?: string
  tokens?: number
  count?: number
  from?: string
  to?: string
}

/** Agent file activity aggregated from tool calls (spec Task 16). */
export type FileOpKind = 'read' | 'write' | 'search' | 'image' | 'dir'

export interface FileOp {
  seq: number
  time: number
  turn: number
  step: number
  kind: FileOpKind
  path: string
  /** Short producer summary (trimmed). */
  summary?: string
  /** File op whose replacement removed it from the current surface. */
  gone?: number
}

/** The whole projection the Context tab renders. */
export interface Projection {
  model?: string
  provider?: string
  contextWindow?: number
  current: Readonly<Composition>
  requests: ProjectionRequest[]
  events: ProjectionEvent[]
  files: FileOp[]
  /** Per-tool breakdown for the stats board tool-call cell. */
  toolCalls: Map<string, number>
  images: number
}