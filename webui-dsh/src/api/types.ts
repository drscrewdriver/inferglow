/* InferGlow Web UI — 后端 API 类型定义 */

export interface Agent {
  id: string
  name: string
  description?: string
  model?: string
  system_prompt?: string
  created_at: string
  updated_at: string
}

export interface Session {
  id: string
  title: string
  agent_id: string
  group?: string
  pinned?: boolean
  status?: string
  created_at: string
  updated_at: string
  message_count?: number
}

export interface ChatMessage {
  id: string
  role: 'user' | 'assistant' | 'system' | 'tool'
  content: string
  tool_name?: string
  tool_call_id?: string
  created_at: string
}

export interface RunState {
  id: string
  session_id: string
  status: 'idle' | 'planning' | 'active' | 'paused' | 'done' | 'error'
  current_step?: string
  created_at: string
}

export interface SpanSummary {
  name: string
  kind: string
  duration_ns: number
  end_time: string
  has_error: boolean
  attrs?: Record<string, unknown>
}

export interface AggregatedStats {
  total_spans: number
  recent_errors: number
  by_kind: Record<string, {
    count: number
    p50_ns: number
    p95_ns: number
    avg_ns: number
    errors: number
  }>
}
