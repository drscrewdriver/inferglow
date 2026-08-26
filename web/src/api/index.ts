import { detectTransport } from './transport'

export { detectTransport, restTransport } from './transport'
export { parseSSE, splitFrames, type SSEFrame } from './sse'
export type { Transport, StreamRunHandlers } from './transport'
export type {
  SessionRecord,
  MessageRecord,
  MessagePage,
  ToolStreamEvent,
  CacheReport,
  CredentialRecord,
  ScheduleRecord,
  SkillRecord,
  MCPToolRecord,
  AuditVerifyResult,
  AuditEntry,
  AuditEntriesResult,
  ApprovalRecord,
  ApprovalRequest,
  ApprovalStatus,
  ApprovalListResult,
  ApprovalSubmitResult,
  RiskLevel,
} from './types'

/** Shared transport instance; stores and hooks depend only on this. */
export const transport = detectTransport()
