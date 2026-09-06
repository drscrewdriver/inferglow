/**
 * Simple store — zustand-like state management for the portable shell.
 * 
 * This replaces the DSH runtime/session manager with a lightweight
 * in-memory store that survives remounts.
 */

export interface Message {
  id: string
  role: 'user' | 'assistant' | 'system' | 'tool'
  content: string
  /** Tool calls embedded in assistant message */
  toolCalls?: ToolCall[]
  /** For assistant: reasoning/pre-computation text */
  reasoning?: string
  /** For user: attached images */
  images?: string[]
  status: 'sending' | 'streaming' | 'sent' | 'error'
  timestamp: number
}

export interface ToolCall {
  id: string
  name: string
  args: Record<string, unknown>
  output?: string
}

export interface Session {
  id: string
  title: string
  messages: Message[]
  createdAt: number
  updatedAt: number
  /** Backend agent this session is bound to (from agent_id, when known). */
  agentId?: string
  /** True once history has been fetched from the backend (InferGlow bridge). */
  messagesLoaded?: boolean
  /** True for sessions that only exist locally and are not yet persisted. */
  localOnly?: boolean
  /** Workspace this conversation belongs to (R8 sidebar grouping). */
  workspace?: string
}

export type ThemeSetting = 'dark' | 'light' | 'system'

export interface Settings {
  /** 'system' follows prefers-color-scheme (matchMedia). */
  theme: ThemeSetting
  /** InferGlow agent id backing new sessions (resolved from GET /v1/agents). */
  model: string
  /** Backend base URL; empty = same origin (vite proxy / go:embed host). */
  apiEndpoint: string
  /** Bearer token for /v1/* when the server runs with -api-key. */
  apiKey: string
  autoScroll: boolean
  /** Chat area zoom (real effect: applied to the conversation container). */
  fontSize: 'small' | 'medium' | 'large'
  /** Session mode placeholder — the chip is a mount point for a future
   * mode config list; empty string until that list is defined. */
  mode: string
  /** Access level for the session (DSH levels). UI-facing for now: the
   * backend tool-gate mapping is a planned follow-up. */
  permission: 'readonly' | 'workspace-write' | 'permissive' | 'full-access'
  /** Context management mode (context.Mode: passthrough/three_zone/
   * summary/hybrid/assembly). UI-facing; per-run engine switch is planned. */
  contextMode: string
}

/** One registered server workspace (GET /v1/workspaces). */
export interface WorkspaceInfo {
  name: string
  root: string
}

export interface AppState {
  /* ── Workspaces ── */
  /** True when a bootstrap call hit 401 — the key is missing/invalid. */
  authRequired: boolean
  setAuthRequired: (v: boolean) => void
  workspaces: WorkspaceInfo[]
  activeWorkspace: string

  /* ── Sessions ── */
  sessions: Session[]
  activeSessionId: string | null
  sidebarCollapsed: boolean
  searchQuery: string
  settingsOpen: boolean
  
  /* ── Settings ── */
  settings: Settings
  
  /* ── Streaming state ── */
  isStreaming: boolean
  streamingMessageId: string | null
  streamingStartTime: number | null

  /* ── Backend connectivity (null = probing) ── */
  backendOnline: boolean | null
  
  /* ── Composer: whether the user has begun typing (sinks the input to the bottom) ── */
  composerTouched: boolean
  
  /* ── Actions: Workspaces ── */
  setWorkspaces: (list: WorkspaceInfo[]) => void
  setActiveWorkspace: (name: string) => void

  /* ── Actions: Sessions ── */
  createSession: () => void
  selectSession: (id: string) => void
  /** Rename a session locally; the caller fires the backend PATCH. */
  renameSession: (id: string, title: string) => void
  deleteSession: (id: string) => void
  updateSessionTitle: (id: string, title: string) => void
  /** Bridge: swap in the backend session list (maps to DSH sessions). */
  replaceAllSessions: (sessions: Session[]) => void
  /** Bridge: backfill one session's history without re-triggering auto-title. */
  replaceMessages: (sessionId: string, messages: Message[]) => void
  
  /* ── Actions: Messages ── */
  addMessage: (sessionId: string, message: Message) => void
  updateMessage: (sessionId: string, messageId: string, updates: Partial<Message>) => void
  addToolCall: (sessionId: string, messageId: string, toolCall: ToolCall) => void
  
  /* ── Actions: Sidebar ── */
  toggleSidebar: () => void
  setSearchQuery: (query: string) => void
  
  /* ── Actions: Settings ── */
  openSettings: () => void
  closeSettings: () => void
  updateSetting: <K extends keyof Settings>(key: K, value: Settings[K]) => void
  
  /* ── Actions: Streaming ── */
  setStreaming: (active: boolean, messageId?: string) => void

  /* ── Actions: Connectivity ── */
  setBackendOnline: (online: boolean | null) => void
  
  /* ── Actions: Composer ── */
  setComposerTouched: (active: boolean) => void
}

/* ── Helpers ── */
let _sessionCounter = 0

function genSessionId(): string { return `sess-${Date.now()}-${++_sessionCounter}` }

function genTitle(content: string): string {
  const text = content.replace(/[#*`]/g, '').trim()
  return text.slice(0, 40) + (text.length > 40 ? '...' : '')
}

/* ── Default settings ── */
const defaultSettings: Settings = {
  theme: 'dark',
  model: '',
  apiEndpoint: '',
  apiKey: '',
  autoScroll: true,
  fontSize: 'medium',
  mode: '',
  permission: 'workspace-write',
  contextMode: 'hybrid',
}

/* ── Theme application ── */
const SETTINGS_KEY = 'inferglow.webui-dsh.settings.v1'
const systemDark = window.matchMedia?.('(prefers-color-scheme: dark)')

function applyTheme(theme: ThemeSetting): void {
  const dark = theme === 'system' ? (systemDark?.matches ?? true) : theme === 'dark'
  if (dark) document.body.setAttribute('data-ds-dark-theme', '')
  else document.body.removeAttribute('data-ds-dark-theme')
}

/** Persisted-settings loader with defaults-merge and darkMode→theme migration. */
function loadPersistedSettings(): Settings {
  try {
    const raw = localStorage.getItem(SETTINGS_KEY)
    if (!raw) return { ...defaultSettings }
    const parsed = JSON.parse(raw) as Partial<Settings> & { darkMode?: boolean }
    // Migration: pre-third-round settings stored a darkMode boolean.
    if (parsed.theme === undefined && typeof parsed.darkMode === 'boolean') {
      parsed.theme = parsed.darkMode ? 'dark' : 'light'
    }
    delete (parsed as Record<string, unknown>).darkMode
    return { ...defaultSettings, ...parsed }
  } catch {
    return { ...defaultSettings }
  }
}

function persistSettings(s: Settings): void {
  try {
    localStorage.setItem(SETTINGS_KEY, JSON.stringify(s))
  } catch {
    // localStorage unavailable — keep in-memory state only.
  }
}

/** Clear persisted settings (设置 → 会话管理). */
export function clearPersistedSettings(): void {
  try {
    localStorage.removeItem(SETTINGS_KEY)
  } catch { /* ignore */ }
}

/* ── Store implementation ── */
const listeners = new Set<() => void>()
const state: Omit<Partial<AppState>, 'actions'> = {
  authRequired: false,
  workspaces: [],
  activeWorkspace: '',
  sessions: [],
  activeSessionId: null,
  sidebarCollapsed: false,
  searchQuery: '',
  settingsOpen: false,
  settings: loadPersistedSettings(),
  isStreaming: false,
  streamingMessageId: null,
  streamingStartTime: null,
  composerTouched: false,
  backendOnline: null,
}

/* ── Actions ── */
const actions: AppState = {
  /* Workspaces */
  get workspaces() { return state.workspaces ?? [] },
  get authRequired() { return state.authRequired ?? false },
  setAuthRequired(v: boolean) {
    if (state.authRequired !== v) {
      state.authRequired = v
      notify()
    }
  },
  get activeWorkspace() { return state.activeWorkspace ?? '' },
  setWorkspaces(list) {
    state.workspaces = list
    // Keep the selection valid; default to the first (server-resolved) root.
    if (!list.some(w => w.name === state.activeWorkspace)) {
      state.activeWorkspace = list[0]?.name ?? ''
    }
    notify()
  },
  setActiveWorkspace(name) {
    state.activeWorkspace = name
    notify()
  },

  /* Sessions */
  get sessions() { return state.sessions ?? [] },
  get activeSessionId() { return state.activeSessionId ?? null },
  get isStreaming() { return state.isStreaming ?? false },
  get streamingMessageId() { return state.streamingMessageId ?? null },
  get streamingStartTime() { return state.streamingStartTime ?? null },
  get composerTouched() { return state.composerTouched ?? false },
  
  createSession() {
    const id = genSessionId()
    const session: Session = {
      id, title: '新对话', messages: [],
      createdAt: Date.now(), updatedAt: Date.now(),
      localOnly: true,
    }
    state.sessions = [session, ...(state.sessions ?? [])]
    state.activeSessionId = id
    state.composerTouched = false
    notify()
  },

  replaceAllSessions(sessions) {
    state.sessions = sessions
    notify()
  },

  replaceMessages(sessionId, messages) {
    const sessions = state.sessions ?? []
    const session = sessions.find(s => s.id === sessionId)
    if (session) {
      session.messages = messages
      session.messagesLoaded = true
      session.localOnly = false
      notify()
    }
  },
  
  selectSession(id) {
    state.activeSessionId = id
    state.composerTouched = false
    notify()
  },

  renameSession(id, title) {
    const sess = state.sessions?.find(x => x.id === id)
    if (!sess) return
    const updated = { ...sess, title }
    state.sessions = (state.sessions ?? []).map(x => (x.id === id ? updated : x))
    notify()
  },
  
  deleteSession(id) {
    state.sessions = (state.sessions ?? []).filter(s => s.id !== id)
    if (state.activeSessionId === id) {
      state.activeSessionId = state.sessions?.[0]?.id ?? null
    }
    notify()
  },
  
  updateSessionTitle(id, title) {
    const sessions = state.sessions ?? []
    const idx = sessions.findIndex(s => s.id === id)
    if (idx !== -1) { sessions[idx].title = title; notify() }
  },
  
  /* Messages */
  addMessage(sessionId, message) {
    const sessions = state.sessions ?? []
    const session = sessions.find(s => s.id === sessionId)
    if (session) {
      session.messages = [...session.messages, message]
      session.updatedAt = Date.now()
      // Auto-update title on first message
      if (session.messages.length === 1) {
        session.title = genTitle(message.content)
      }
      notify()
    }
  },
  
  updateMessage(sessionId, messageId, updates) {
    const sessions = state.sessions ?? []
    const session = sessions.find(s => s.id === sessionId)
    if (session) {
      const idx = session.messages.findIndex(m => m.id === messageId)
      if (idx !== -1) {
        // Rebuild the array (and the message object) so React sees a new
        // reference — in-place mutation bails out of re-renders, which would
        // freeze streaming deltas on screen.
        session.messages = session.messages.map((m, i) =>
          i === idx ? { ...m, ...updates } : m)
        session.updatedAt = Date.now()
        notify()
      }
    }
  },

  addToolCall(sessionId, messageId, toolCall) {
    const sessions = state.sessions ?? []
    const session = sessions.find(s => s.id === sessionId)
    if (session) {
      session.messages = session.messages.map(m =>
        m.id === messageId
          ? { ...m, toolCalls: [...(m.toolCalls ?? []), toolCall] }
          : m)
      session.updatedAt = Date.now()
      notify()
    }
  },
  
  /* Sidebar */
  get sidebarCollapsed() { return state.sidebarCollapsed ?? false },
  get searchQuery() { return state.searchQuery ?? '' },
  
  toggleSidebar() {
    state.sidebarCollapsed = !state.sidebarCollapsed
    notify()
  },
  
  setSearchQuery(query) {
    state.searchQuery = query
    notify()
  },
  
  /* Settings */
  get settingsOpen() { return state.settingsOpen ?? false },
  get settings() { return state.settings ?? defaultSettings },
  
  openSettings() { state.settingsOpen = true; notify() },
  closeSettings() { state.settingsOpen = false; notify() },
  
  updateSetting(key, value) {
    const s = { ...(state.settings ?? defaultSettings) }
    s[key] = value
    state.settings = s
    persistSettings(s)
    // Apply theme change immediately
    if (key === 'theme') {
      applyTheme(s.theme)
    }
    notify()
  },
  
  /* Streaming */
  setStreaming(active, messageId) {
    state.isStreaming = active
    state.streamingMessageId = active ? messageId ?? null : null
    state.streamingStartTime = active ? Date.now() : null
    notify()
  },

  /* Connectivity */
  get backendOnline() { return state.backendOnline ?? null },
  setBackendOnline(online) {
    state.backendOnline = online
    notify()
  },

  /* Composer */
  setComposerTouched(active) {
    state.composerTouched = active
    notify()
  },
}

/* ── Subscribe ── */
function notify() { listeners.forEach(fn => fn()) }

function subscribe(fn: () => void): () => void {
  listeners.add(fn)
  return () => { listeners.delete(fn) }
}

export { actions as store, subscribe, defaultSettings, applyTheme }
