// Backend contract types, aligned 1:1 with the InferGlow REST API.

export interface SessionRecord {
  id: string
  owner?: string
  agent_id?: string
  status: 'active' | 'stopped' | 'archived'
  title?: string
  group?: string
  pinned?: boolean
  created_at: string
  updated_at: string
}

export interface MessageRecord {
  id: string
  session_id: string
  role: 'user' | 'assistant' | 'tool'
  content?: string
  tool_name?: string
  tool_status?: string
  created_at: string
}

export interface MessagePage {
  messages: MessageRecord[]
  has_more: boolean
  next_before: string | null
}

export interface ToolStreamEvent {
  type: string
  tool_name?: string
  round?: number
  tokens?: number
  error?: string
  timestamp: string
}

export interface CacheReportSummary {
  model?: string
  total_prompt_tokens: number
  total_cached_tokens: number
  cache_hit_rate: number
  total_cost: number
  actual_cost: number
  savings: number
  currency: string
  session_count: number
}

export interface CacheReport {
  generated_at: string
  from: string
  to: string
  overall: CacheReportSummary
  by_model?: CacheReportSummary[]
}

export interface CredentialRecord {
  id: string
  name: string
  provider: string
  username?: string
  secret?: string
  created_at: string
}

export interface ScheduleRecord {
  id: string
  name: string
  flow: string
  interval: number
  stateful: boolean
  enabled: boolean
  created_at: string
}

export interface SkillRecord {
  name: string
  description: string
  tags?: string[]
  executable: boolean
}

export interface MCPToolRecord {
  name: string
  description: string
  input_schema?: Record<string, unknown>
}

export interface AuditVerifyResult {
  valid: boolean
  chain_length?: number
  [key: string]: unknown
}

export interface AuditEntry {
  prev_hash: string
  hash: string
  id: string
  timestamp: string
  source: string
  action: string
  error?: string
  metadata?: Record<string, string>
}

export interface AuditEntriesResult {
  entries: AuditEntry[]
  count: number
}

// ─── approvals (Phase 6, Task 22) — mirror approval.Record / Request ───

export type ApprovalStatus = 'approved' | 'denied' | 'pending' | 'allowed'

export type RiskLevel = 'none' | 'low' | 'medium' | 'high'

export interface ApprovalRequest {
  request_id?: string
  source?: string
  capability: string
  subject: string
  risk?: RiskLevel
  payload?: Record<string, unknown>
  timeout?: string
  escalation?: string
}

export interface ApprovalRecord {
  id: string
  request?: ApprovalRequest
  status: ApprovalStatus
  approver?: string
  created_at: string
  updated_at?: string
}

export interface ApprovalListResult {
  approvals: ApprovalRecord[]
  count: number
}

/** POST /v1/approvals response returns the (possibly pending) record. */
export type ApprovalSubmitResult = ApprovalRecord
