/**
 * Pure helpers shared by the 轨迹/上下文 panels and their unit tests.
 * No React / store / fetch imports — keep this module side-effect free.
 */
import type { SpanSummary } from '../api/client.ts'

/** Parse one persisted run summary into SpanSummary lines (end = start+ms). */
export function tracesToSpans(traces: { content: string; created_at: string }[]): SpanSummary[] {
  const out: SpanSummary[] = []
  for (const t of traces) {
    let parsed: { start?: string; spans?: { kind: string; name: string; duration_ms: number; error?: boolean }[] }
    try { parsed = JSON.parse(t.content) } catch { continue }
    const startMs = parsed.start ? Date.parse(parsed.start) : Date.parse(t.created_at)
    for (const sp of parsed.spans ?? []) {
      const end = (Number.isNaN(startMs) ? Date.parse(t.created_at) : startMs) + (sp.duration_ms ?? 0)
      out.push({
        name: sp.name,
        kind: (sp.kind as SpanSummary['kind']) ?? 'internal',
        duration_ns: (sp.duration_ms ?? 0) * 1e6,
        end_time: new Date(end).toISOString(),
        has_error: !!sp.error,
        attrs: {},
      })
    }
  }
  return out
}
