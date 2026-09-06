/**
 * InferGlow bridge — glue between the DSH store and the backend API client.
 *
 * Owns: startup hydration (health → agents → sessions), backend session
 * lifecycle (create/select/delete), history backfill, and the default agent
 * selection used for new conversations. Components call these helpers instead
 * of touching the client directly, keeping store.ts free of fetch logic.
 */
import { store, type Message, type Session } from '../store.ts'
import { createInferGlowApi, type InferGlowApi } from '../api/client.ts'
import type { Agent, ChatMessage } from '../api/types.ts'

export const api: InferGlowApi = createInferGlowApi(() => store.settings.apiEndpoint)

let agents: Agent[] = []

/** First agent from the backend — fallback for new sessions. */
export function getDefaultAgent(): Agent | null {
  return agents[0] ?? null
}

/** Resolve the agent id backing the next chat: settings.model wins, else first agent. */
export function getActiveAgentId(): string {
  const fromSettings = store.settings.model.trim()
  if (fromSettings) return fromSettings
  return getDefaultAgent()?.id ?? ''
}

/** Agent display name for an id (falls back to the id itself). */
export function agentName(id: string): string {
  return agents.find(a => a.id === id)?.name ?? id
}

/** All agents known to the backend (populated by bootstrap). */
export function getAgents(): Agent[] {
  return agents
}

/* ── Per-session usage accumulation (llm_end events) ──
 * Surfaced for the context panel's token counters; resets on reload. */
const usageTotals = { promptTokens: 0, completionTokens: 0, totalTokens: 0, llmCalls: 0 }

export function recordUsage(u: {
  prompt_tokens: number
  completion_tokens: number
  total_tokens: number
}): void {
  usageTotals.promptTokens += u.prompt_tokens
  usageTotals.completionTokens += u.completion_tokens
  usageTotals.totalTokens += u.total_tokens
  usageTotals.llmCalls += 1
}

export function getUsageTotals(): {
  promptTokens: number
  completionTokens: number
  totalTokens: number
  llmCalls: number
} {
  return { ...usageTotals }
}

/** Map a backend session record onto the DSH session shape (no messages yet). */
function toDshSession(s: {
  id: string
  title: string
  agent_id?: string
  created_at: string
  updated_at: string
}): Session {
  return {
    id: s.id,
    title: s.title || '新对话',
    messages: [],
    createdAt: Date.parse(s.created_at) || Date.now(),
    updatedAt: Date.parse(s.updated_at) || Date.now(),
    agentId: s.agent_id || undefined,
    messagesLoaded: false,
    localOnly: false,
  }
}

/** Map a backend chat message onto the DSH message shape (already settled). */
function toDshMessage(m: ChatMessage): Message {
  return {
    id: m.id,
    role: m.role,
    content: m.content || (m.tool_name ? `工具调用: ${m.tool_name}` : ''),
    status: 'sent',
    timestamp: Date.parse(m.created_at) || Date.now(),
  }
}

async function refreshSessions(): Promise<boolean> {
  const online = await api.health()
  store.setBackendOnline(online)
  if (!online) return false
  try {
    agents = await api.listAgents()
    const sessions = await api.listSessions()
    store.replaceAllSessions(sessions.map(toDshSession))
    return true
  } catch (err) {
    console.warn('[webui-dsh] bootstrap failed:', err)
    return false
  }
}

/** Startup hydration. Safe to call once from the app root. */
export async function bootstrap(): Promise<void> {
  await refreshSessions()
  // Reconcile the persisted agent choice with what the server actually has:
  // a stale id (config from another backend / deleted agent) falls back to
  // the first listed agent instead of failing at send time.
  const ids = agents.map(a => a.id)
  const current = store.settings.model.trim()
  if (agents.length > 0 && (!current || !ids.includes(current))) {
    store.updateSetting('model', agents[0].id)
  }
}

/**
 * Ensure a persisted session exists for sending a message.
 * - No session yet (hero state): create one on the backend titled from the
 *   first message; falls back to a local-only session when the backend is
 *   unreachable so the composer keeps working (send will surface the error).
 * - An existing local-only session gets persisted in place.
 */
export async function ensureSession(firstMessage: string): Promise<string> {
  const currentId = store.activeSessionId
  const current = store.sessions.find(s => s.id === currentId)
  const needsCreate = !current || (current.localOnly && !current.messagesLoaded)
  if (!needsCreate) return current?.id ?? ''

  const title = firstMessage.replace(/[#*`]/g, '').trim().slice(0, 40) || '新对话'
  try {
    const created = await api.createSession(getActiveAgentId(), title)
    const dsh = toDshSession(created)
    // Preserve any locally typed state: the new backend session replaces the
    // local placeholder; the caller then targets the backend id.
    store.replaceAllSessions([dsh, ...store.sessions.filter(s => s.id !== current?.id)])
    store.selectSession(dsh.id)
    return dsh.id
  } catch (err) {
    console.warn('[webui-dsh] create session failed:', err)
    if (!current) store.createSession()
    return store.activeSessionId ?? ''
  }
}

/** Select a session and lazily backfill its history from the backend. */
export async function selectSession(id: string): Promise<void> {
  store.selectSession(id)
  const session = store.sessions.find(s => s.id === id)
  if (!session || session.messagesLoaded || session.localOnly) return
  try {
    const messages = await api.listMessages(id)
    store.replaceMessages(id, messages.map(toDshMessage))
  } catch (err) {
    console.warn('[webui-dsh] load history failed:', err)
    // Mark loaded to avoid retry loops; empty history is visible to the user.
    store.replaceMessages(id, [])
  }
}

/** Create a session immediately (sidebar "新会话"); falls back to local-only. */
export async function createSession(): Promise<void> {
  const title = ''
  try {
    const created = await api.createSession(getActiveAgentId(), title)
    const dsh = toDshSession(created)
    store.replaceAllSessions([dsh, ...store.sessions])
    store.selectSession(dsh.id)
  } catch {
    store.createSession()
  }
}

/** Delete on the backend (fire-and-forget) and remove locally. */
export function deleteSession(id: string): void {
  store.deleteSession(id)
  void api.deleteSession(id).catch(() => {})
}
