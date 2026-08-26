import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { useChatStore, type ChatMessage } from '../stores/chatStore'
import { useSessionStore } from '../stores/sessionStore'
import { useSession, UNGROUPED_LABEL, SlotOutlet } from '../framework'
import { useTrafficStore, selectFrozen } from '../traffic/trafficStore'
import '../traffic/slots' // registers conversation.* / details.panel.items slots (side effect)
import '../approval/slots' // registers details.panel.items (approval cards)
import styles from './conversation.module.css'
import { useTidychatStore, selectTurnFolded } from '../tidychat/tidychatStore'
import { turnGrouping, decideFold, type TzTurn } from '../tidychat/logic'
import { FoldTurn } from '../tidychat/FoldTurn'
import { TurnNavigator } from '../tidychat/TurnNavigator'
import { AutoLoad } from '../tidychat/AutoLoad'
import { ContextView } from '../context/ContextView'
import { MentionInput, type MentionInputHandle } from '../filetag/MentionInput'
import { ProducedFiles } from '../filetag/ProducedFiles'

/** Flow-node types for the message stream (spec Task 9). */
export type FlowNode =
  | { kind: 'user' | 'assistant' | 'tool'; message: ChatMessage }
  | { kind: 'command'; message: ChatMessage }
  | { kind: 'compaction'; spans: number }
  | { kind: 'turn-error'; text: string }
  | { kind: 'turn-tail' }

// ─── Tool card (from the former dock inline) ───────────────────────────────
function ToolCardView({ m }: { m: ChatMessage }) {
  const status = m.toolStatus ?? 'run'
  const statusIcon = status === 'run' ? '⟳' : status === 'ok' ? '✓' : '✕'
  return (
    <div className={`${styles.toolCard}${m.toolStatus === 'run' ? ` ${styles.toolCardOpen}` : ''}`}>
      <div className={styles.toolHead}>
        <span className={`${styles.toolStatus} ${styles[`toolStatus__${status}`]}`}>{statusIcon}</span>
        <span className={styles.toolName}>{m.toolName ?? 'tool'}</span>
        <span className={styles.toolTail}>
          <span className={styles.toolDur}>
            {status === 'run' ? '运行中…' : status === 'ok' ? '完成' : '失败'}
          </span>
          <span className={styles.toolChev}>▾</span>
        </span>
      </div>
      {m.content && (
        <div className={styles.toolOut}>
          <pre className={styles.toolPre}>{m.content}</pre>
        </div>
      )}
    </div>
  )
}

// ─── Per-message actions: copy / evaluate / branch / restore ───────────────
function MessageActions({ content }: { content: string }) {
  const [reacted, setReacted] = useState<'up' | 'down' | null>(null)
  const copy = () => {
    if (content) void navigator.clipboard?.writeText(content)
  }
  return (
    <div className={styles.msgActions}>
      <button onClick={copy} title="复制">
        ⧉
      </button>
      <button
        className={reacted === 'up' ? styles.actionOn : undefined}
        onClick={() => setReacted((v) => (v === 'up' ? null : 'up'))}
        title="点赞"
      >
        👍
      </button>
      <button
        className={reacted === 'down' ? styles.actionOn : undefined}
        onClick={() => setReacted((v) => (v === 'down' ? null : 'down'))}
        title="点踩"
      >
        👎
      </button>
      <button title="分支（stub）" disabled>
        ⑂
      </button>
      <button title="恢复（stub）" disabled>
        ↺
      </button>
    </div>
  )
}

function MessageNode({
  node,
  streaming,
  anchorKey,
  flowKind,
  showThink,
}: {
  node: Extract<FlowNode, { message: ChatMessage }>
  streaming: boolean
  anchorKey?: string
  flowKind: string
  showThink: boolean
}) {
  const { kind, message } = node
  if (kind === 'tool') {
    return (
      <div className={`${styles.msg} ${styles.msgTool}`} data-chat-flow-kind={flowKind} data-chat-anchor-key={anchorKey}>
        <span className={styles.msgAvatar}>⚙</span>
        <div className={styles.msgBody}>
          <ToolCardView m={message} />
        </div>
      </div>
    )
  }
  const isUser = kind === 'command' || kind === 'user'
  const isStreamTail = streaming && kind === 'assistant' && message.content === ''
  const think = message.think
  const thinkVisible = think !== undefined && think !== '' && showThink
  return (
    <div className={`${styles.msg} ${isUser ? styles.msgUser : styles.msgAssistant}`} data-chat-flow-kind={flowKind} data-chat-anchor-key={anchorKey}>
      <span className={styles.msgAvatar}>{isUser ? (kind === 'command' ? '/' : '我') : '◈'}</span>
      <div className={styles.msgBody}>
        {kind === 'command' && <span className={styles.commandTag}>命令</span>}
        {thinkVisible && (
          <div data-variant="think" data-tidychat-think className={styles.thinkBlock}>
            {think}
          </div>
        )}
        {thinkVisible && <hr data-tidychat-divider className={styles.thinkDivider} />}
        <div className={styles.msgText}>{message.content || (isStreamTail ? <span className={styles.caret} /> : '')}</div>
        <MessageActions content={message.content} />
      </div>
    </div>
  )
}

// ─── Flow node renderer for all node kinds ────────────────────────────────
function FlowList({
  nodes,
  streaming,
  onScrollTop,
  sessionId,
  hasMore,
  loadOlder,
}: {
  nodes: FlowNode[]
  streaming: boolean
  onScrollTop?: () => void
  sessionId: string
  hasMore: boolean
  loadOlder: (sessionId: string) => Promise<void>
}) {
  const ref = useRef<HTMLDivElement>(null)
  const foldEnabled = useTidychatStore((s) => s.config.fold)
  const foldStates = useTidychatStore((s) => s.foldStates)
  const toggleFold = useTidychatStore((s) => s.toggleFold)
  useEffect(() => {
    if (ref.current) ref.current.scrollTop = ref.current.scrollHeight
  }, [nodes.length, streaming])

  // Group the message flow into turns (pure logic) and derive the current
  // fold / hide / think expectations from the store — all derived, no DOM scan.
  const turns = useMemo(() => turnGrouping(nodes, streaming), [nodes, streaming])

  const nodeToTurn = useMemo(() => {
    const m = new Map<number, number>()
    for (const t of turns) {
      for (const s of t.steps) m.set(s, t.turn)
      for (const ti of t.toolIndexes) m.set(ti, t.turn)
    }
    return m
  }, [turns])

  const foldPlan = useMemo(() => {
    const hidden = new Set<number>()
    const thinkHidden = new Set<number>()
    const bars: { index: number; turn: TzTurn; folded: boolean }[] = []
    if (!foldEnabled) return { hidden, thinkHidden, bars }
    for (const t of turns) {
      if (!t.hasTail) continue // only completed rounds fold
      const folded = selectTurnFolded(foldStates, sessionId, t.turn)
      bars.push({ index: t.firstIndex, turn: t, folded })
      const d = decideFold(t, nodes, folded, true)
      for (const h of d.hiddenIndexes) hidden.add(h)
      if (d.finalThinkHidden && t.finalIndex !== -1) thinkHidden.add(t.finalIndex)
    }
    return { hidden, thinkHidden, bars }
  }, [turns, foldStates, sessionId, foldEnabled, nodes])

  // Per-kind sequence counters keep mini-map jump targets unique.
  let userSeq = 0
  let toolSeq = 0

  const render: ReactNode[] = []
  for (let i = 0; i < nodes.length; i++) {
    const n = nodes[i]
    const bar = foldPlan.bars.find((b) => b.index === i)
    if (bar) {
      render.push(
        <FoldTurn
          key={`bar-${bar.turn.turn}`}
          turn={bar.turn}
          folded={bar.folded}
          enabled={foldEnabled}
          onToggle={() => toggleFold(sessionId, bar.turn.turn)}
        />,
      )
    }
    if (foldPlan.hidden.has(i)) continue

    const flowKind = n.kind
    const withMsg = 'message' in n ? (n as Extract<FlowNode, { message: ChatMessage }>) : null
    let anchorKey: string | undefined
    if (withMsg) {
      const block = nodeToTurn.get(i) ?? 0
      if (flowKind === 'user' || flowKind === 'command') anchorKey = `${userSeq++}:user`
      else if (flowKind === 'tool') anchorKey = `${toolSeq++}:tool-call{${block}}`
      else if (flowKind === 'assistant') anchorKey = `s${i}:assistant-step{${block}}`
    }

    switch (flowKind) {
      case 'user':
      case 'assistant':
      case 'tool':
        render.push(
          <MessageNode
            key={withMsg!.message.id}
            node={withMsg!}
            streaming={streaming}
            anchorKey={anchorKey}
            flowKind={flowKind}
            showThink={!foldPlan.thinkHidden.has(i)}
          />,
        )
        break
      case 'command':
        render.push(
          <MessageNode
            key={`cmd-${withMsg!.message.id}`}
            node={withMsg!}
            streaming={streaming}
            anchorKey={anchorKey}
            flowKind={flowKind}
            showThink={false}
          />,
        )
        break
      case 'compaction':
        render.push(
          <div key={`compaction-${i}`} className={styles.compaction} data-chat-flow-kind="compaction">
            <span>⤢ 上下文已压缩 · 约 {n.spans} 段</span>
          </div>,
        )
        break
      case 'turn-error':
        render.push(
          <div key={`err-${i}`} className={styles.turnError} data-chat-flow-kind="turn-error">
            <span>⚠</span>
            <span>{n.text}</span>
          </div>,
        )
        break
      case 'turn-tail':
        render.push(
          <div key={`tail-${i}`} className={styles.turnTail} data-chat-flow-kind="turn-tail">
            <span>─ 本次对话结束 ─</span>
          </div>,
        )
        break
      default:
        break
    }
  }

  return (
    <div
      className={styles.messages}
      ref={ref}
      data-conversation-scroll
      onScroll={(e) => {
        if (e.currentTarget.scrollTop < 80) onScrollTop?.()
      }}
    >
      <AutoLoad sessionId={sessionId} hasMore={hasMore} loadOlder={loadOlder} />
      {render}
      {nodes.length === 0 && (
        <div className={styles.empty}>
          <div className={styles.emptyTitle}>开始对话</div>
          <div>选择左侧会话开始对话，或新建一个会话。</div>
        </div>
      )}
    </div>
  )
}

// ─── Composer: command menu + model select + context usage + send ─────────
function Composer({
  disabled,
  text,
  setText,
}: {
  disabled: boolean
  text: string
  setText: (v: string) => void
}) {
  const [cmdOpen, setCmdOpen] = useState(false)
  const [model, setModel] = useState('deepseek-chat')
  const [mentionOpen, setMentionOpen] = useState(false)
  const mentionRef = useRef<MentionInputHandle>(null)
  const sendMessage = useChatStore((s) => s.sendMessage)
  const streaming = useChatStore((s) => s.streaming)
  const running = useChatStore((s) => s.running)
  const stop = useChatStore((s) => s.stop)
  const active = useSession()
  const agentId = active?.agent_id ?? 'a1'

  const COMMANDS = ['/plan', '/think', '/tool', '/memory', '/agent']

  const send = useCallback(() => {
    if (!active || streaming || mentionOpen) return
    // Serialize chips (@file refs) into the message before sending.
    const raw = mentionRef.current?.serializeValue() ?? text
    const msg = raw.trim()
    if (!msg) return
    setText('')
    setCmdOpen(false)
    void sendMessage(active.id, agentId, msg)
  }, [text, active, agentId, streaming, sendMessage, setText, mentionOpen])

  const pickCommand = (c: string) => {
    setText(`${c} `)
    setCmdOpen(false)
  }

  const openFile = useCallback((path: string) => {
    // Demo stub: surface the produced file. Real impl would open in a viewer.
    window.alert(`打开文件：${path}`)
  }, [])

  return (
    <div className={styles.composerWrap}>
      <SlotOutlet name="conversation.input.dock" props={{ onPullBack: setText }} />
      <div className={styles.composer}>
        <div className={styles.composerToolbar}>
          <button className={styles.cmdBtn} onClick={() => setCmdOpen((v) => !v)} title="命令菜单">
            /
          </button>
          <select className={styles.modelSelect} value={model} onChange={(e) => setModel(e.target.value)}>
            <option value="deepseek-chat">deepseek-chat</option>
            <option value="deepseek-reasoner">deepseek-reasoner</option>
            <option value="gpt-5">gpt-5</option>
          </select>
          {running && <span className={styles.runningTag}>运行中…</span>}
        </div>
        {cmdOpen && (
          <div className={styles.cmdMenu}>
            {COMMANDS.map((c) => (
              <button key={c} onClick={() => pickCommand(c)}>
                <span className={styles.cmdName}>{c}</span>
                <span className={styles.cmdHint}>插入命令</span>
              </button>
            ))}
          </div>
        )}
        <MentionInput
          ref={mentionRef}
          value={text}
          onChange={(v) => {
            setText(v)
            if (v.startsWith('/')) setCmdOpen(true)
          }}
          onMenuChange={setMentionOpen}
          onSubmit={send}
          disabled={disabled}
        />
        <ProducedFiles onOpen={openFile} />
        <div className={styles.composerBar}>
          <span className={styles.ctxChip}>上下文 128k</span>
          <span className={styles.composerSpacer} />
          <SlotOutlet name="conversation.input.right" props={{ disabled }} />
          {streaming && (
            <button className={styles.stopBtn} onClick={stop}>
              ■ 停止
            </button>
          )}
          <button className={styles.sendBtn} onClick={send} disabled={disabled || !text.trim() || mentionOpen}>
            ➤
          </button>
        </div>
      </div>
    </div>
  )
}

// ─── Session header: breadcrumb + status + tabs ───────────────────────────
type ChatTab = 'chat' | 'summary' | 'context'

function ChatHeader({ live, tab, onTab }: { live: boolean; tab: ChatTab; onTab: (t: ChatTab) => void }) {
  const active = useSession()
  return (
    <div className={styles.chatHead}>
      <div className={styles.breadcrumb}>
        <span className={styles.crumb}>{active?.group || UNGROUPED_LABEL}</span>
        <span className={styles.crumbSep}>/</span>
        <span className={styles.crumbCur}>{active?.title ?? '未选择会话'}</span>
      </div>
      <span className={styles.model}>{active?.agent_id ?? 'agent'} · {live ? 'live' : 'demo'}</span>
      <span className={styles.spacer} />
      <div className={styles.tabs}>
        <button className={tab === 'chat' ? styles.tabOn : styles.tab} onClick={() => onTab('chat')}>
          聊天
        </button>
        <button className={tab === 'summary' ? styles.tabOn : styles.tab} onClick={() => onTab('summary')}>
          概要
        </button>
        <button className={tab === 'context' ? styles.tabOn : styles.tab} onClick={() => onTab('context')}>
          上下文
        </button>
      </div>
    </div>
  )
}

// ─── Conversation pane ────────────────────────────────────────────────────
export function Conversation({ live }: { live: boolean }) {
  const activeId = useSessionStore((s) => s.activeId)
  const messages = useChatStore((s) => s.messages)
  const error = useChatStore((s) => s.error)
  const streaming = useChatStore((s) => s.streaming)
  const loadHistory = useChatStore((s) => s.loadHistory)
  const appendHistory = useChatStore((s) => s.appendHistory)
  const loadOlder = useChatStore((s) => s.loadOlder)
  const hasMore = useChatStore((s) => s.hasMore)
  const clear = useChatStore((s) => s.clear)
  const frozen = useTrafficStore(selectFrozen)
  const [composeText, setComposeText] = useState('')
  const [tab, setTab] = useState<ChatTab>('chat')

  // Keep the traffic store focused on the active session (isolated freeze /
  // queues per session) whenever the selection changes.
  useEffect(() => {
    useTrafficStore.getState().setSession(activeId)
  }, [activeId])

  const active = useSession()
  useEffect(() => {
    useTrafficStore.getState().setAgent(active?.agent_id ?? 'a1')
  }, [active?.agent_id])

  // Load history on session switch; clear otherwise.
  useEffect(() => {
    if (!activeId) {
      clear()
      return
    }
    clear()
    void loadHistory(activeId).then((page) => {
      if (page) appendHistory(page)
    })
  }, [activeId, clear, loadHistory, appendHistory])

  const nodes = useMemo<FlowNode[]>(() => {
    const list: FlowNode[] = []
    for (const m of messages) {
      if (m.role === 'tool') list.push({ kind: 'tool', message: m })
      else if (m.role === 'user' && m.content.startsWith('/')) list.push({ kind: 'command', message: m })
      else list.push({ kind: m.role, message: m })
    }
    if (error) list.push({ kind: 'turn-error', text: error })
    if (list.some((n) => n.kind === 'user' || n.kind === 'assistant' || n.kind === 'tool')) {
      if (!streaming) list.push({ kind: 'turn-tail' })
    }
    return list
  }, [messages, error, streaming])

  return (
    <>
      <ChatHeader live={live} tab={tab} onTab={setTab} />
      {tab === 'context' ? (
        <ContextView sessionId={activeId ?? ''} />
      ) : (
        <>
          <FlowList
            nodes={nodes}
            streaming={streaming}
            sessionId={activeId ?? ''}
            hasMore={hasMore}
            loadOlder={loadOlder}
            onScrollTop={activeId ? () => void loadOlder(activeId) : undefined}
          />
          <TurnNavigator sessionId={activeId ?? ''} />
          <Composer disabled={!live || frozen} text={composeText} setText={setComposeText} />
        </>
      )}
    </>
  )
}