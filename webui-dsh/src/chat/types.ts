/**
 * Message types for the chat system.
 */

export type MessageRole = 'user' | 'assistant' | 'system'

export type MessageStatus = 'sending' | 'sent' | 'streaming' | 'error'

export interface ChatMessage {
  id: string
  role: MessageRole
  content: string
  status: MessageStatus
  timestamp: number
  /** Tool calls associated with this message */
  toolCalls?: ToolCall[]
  /** For assistant: streaming chunks or final content */
  reasoning?: string
}

export interface ToolCall {
  id: string
  name: string
  args: Record<string, unknown>
  result?: unknown
}

/** Generate a stable id */
let _msgId = 0
export function generateMessageId(): string {
  return `msg-${Date.now()}-${++_msgId}`
}
