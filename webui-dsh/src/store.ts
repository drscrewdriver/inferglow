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
  /** True once history has been fetched from the backend (InferGlow bridge). */
  messagesLoaded?: boolean
  /** True for sessions that only exist locally and are not yet persisted. */
  localOnly?: boolean
}

export interface Settings {
  darkMode: boolean
  /** InferGlow agent id backing new sessions (resolved from GET /v1/agents). */
  model: string
  /** Backend base URL; empty = same origin (vite proxy / go:embed host). */
  apiEndpoint: string
  autoScroll: boolean
  fontSize: 'small' | 'medium' | 'large'
}

export interface AppState {
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
  
  /* ── Actions: Sessions ── */
  createSession: () => void
  selectSession: (id: string) => void
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
  darkMode: true,
  model: '',
  apiEndpoint: '',
  autoScroll: true,
  fontSize: 'medium',
}

/* ── Store implementation ── */
const listeners = new Set<() => void>()
const state: Omit<Partial<AppState>, 'actions'> = {
  sessions: [],
  activeSessionId: null,
  sidebarCollapsed: false,
  searchQuery: '',
  settingsOpen: false,
  settings: { ...defaultSettings },
  isStreaming: false,
  streamingMessageId: null,
  streamingStartTime: null,
  composerTouched: false,
  backendOnline: null,
}

/* ── Actions ── */
const actions: AppState = {
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
      const msg = session.messages.find(m => m.id === messageId)
      if (msg) {
        Object.assign(msg, updates)
        notify()
      }
    }
  },
  
  addToolCall(sessionId, messageId, toolCall) {
    const sessions = state.sessions ?? []
    const session = sessions.find(s => s.id === sessionId)
    if (session) {
      const msg = session.messages.find(m => m.id === messageId)
      if (msg) {
        msg.toolCalls = [...(msg.toolCalls ?? []), toolCall]
        notify()
      }
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
    // Apply theme change immediately
    if (key === 'darkMode') {
      if (value) {
        document.body.setAttribute('data-ds-dark-theme', '')
      } else {
        document.body.removeAttribute('data-ds-dark-theme')
      }
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

export { actions as store, subscribe, defaultSettings }
