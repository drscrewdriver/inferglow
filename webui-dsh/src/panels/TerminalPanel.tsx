/**
 * TerminalPanel — v2 persistent PTY terminal.
 *
 * One long-lived interactive shell per workspace, bridged over WebSocket to
 * GET /v1/pty (go-pty ConPTY on Windows). Output streams live; the process
 * survives page refreshes and panel switches (the server keeps a transcript
 * ring and replays it on reconnect), mirroring DSH-better-sidebar's
 * pty-manager lifecycle. Presentation follows better-sidebar's TerminalView:
 * xterm.js, theme-token surface colors, one-dark/one-light ANSI, fit-to-pane
 * resizing.
 *
 * Gates surface as friendly errors: 404 = server not started with -pty
 * (+ -api-key), 401 = missing/wrong token.
 */

import { useEffect, useRef, useState } from 'react'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import { store, subscribe } from '../store.ts'
import { getActiveWorkspace } from '../bridge/inferglow.ts'
import { terminalFontStack, xtermTheme } from './terminalTheme.ts'

type ConnState = 'connecting' | 'live' | 'reconnecting' | 'exited' | 'error'

/** The reconnection ladder (ms) — quick retries first, then give up. */
const RECONNECT_DELAYS = [400, 800, 1600, 3200]

function b64ToBytes(b64: string): Uint8Array {
  const bin = atob(b64)
  const out = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i)
  return out
}

/** Build the PTY WebSocket URL from the API endpoint (origin) + token. */
export function ptyUrl(endpoint: string, apiKey: string, workspace: string): string {
  let base = endpoint.trim() || (typeof location !== 'undefined' ? location.origin : '')
  if (!/^https?:\/\//i.test(base)) base = `http://${base}`
  const url = new URL('/v1/pty', base.endsWith('/') ? base : base + '/')
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  if (apiKey) url.searchParams.set('token', apiKey)
  if (workspace) url.searchParams.set('workspace', workspace)
  return url.toString()
}

/** Human-readable failure for a refused/failed dial. */
export function friendlyDialError(err: unknown): string {
  const msg = String((err as Error)?.message ?? err)
  if (msg.includes('404')) return '服务端未启用 PTY 终端（需 -api-key 且 -pty 启动）'
  if (msg.includes('401')) return '鉴权失败 — 请在 设置 → API 配置 填写正确的 API Key'
  if (msg.includes('1006')) return '连接被拒绝 — 会话数已满或服务端不可达'
  return msg
}

const STATUS_LABEL: Record<ConnState, string> = {
  connecting: '连接中…',
  live: '● 已连接',
  reconnecting: '重连中…',
  exited: '进程退出',
  error: '未连接',
}

export function TerminalPanel() {
  const hostRef = useRef<HTMLDivElement | null>(null)
  const [connState, setConnState] = useState<ConnState>('connecting')
  const [errMsg, setErrMsg] = useState<string | null>(null)
  const [wsLabel, setWsLabel] = useState(getActiveWorkspace() || '(默认)')
  const [epoch, setEpoch] = useState(0) // bump ⇒ full re-dial (manual 重连 / workspace switch)

  // Re-dial when the active workspace changes: each workspace owns its own
  // persistent shell, so switching selectors switches terminals.
  useEffect(() => subscribe(() => setWsLabel(getActiveWorkspace() || '(默认)')), [])
  useEffect(() => {
    setEpoch(e => e + 1)
  }, [wsLabel])

  useEffect(() => {
    const host = hostRef.current
    if (host === null) return
    let disposed = false
    let ws: WebSocket | null = null
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null
    let reconnectAttempt = 0

    // The terminal must not open in a zero-size container (pane may still be
    // animating in): buffer writes — xterm's WriteBuffer replays once open.
    const term = new Terminal({
      cursorBlink: true,
      fontFamily: terminalFontStack(),
      fontSize: 12.5,
      allowTransparency: true,
      convertEol: false,
      scrollback: 4000,
      theme: xtermTheme({
        dark: document.body.hasAttribute('data-ds-dark-theme'),
        background: getComputedStyle(document.body).getPropertyValue('--dsw-alias-bg-base').trim(),
        foreground: getComputedStyle(document.body).getPropertyValue('--dsw-alias-label-primary').trim(),
      }),
    })
    const fit = new FitAddon()
    try {
      term.loadAddon(fit)
      term.open(host)
      try { fit.fit() } catch { /* zero-size at mount is fine */ }
    } catch (e) {
      setConnState('error')
      setErrMsg(`终端初始化失败: ${String(e)}`)
      return
    }

    // Re-theme in place when the app's scheme flips.
    const unsubTheme = subscribe(() => {
      term.options.theme = xtermTheme({
        dark: document.body.hasAttribute('data-ds-dark-theme'),
        background: getComputedStyle(document.body).getPropertyValue('--dsw-alias-bg-base').trim(),
        foreground: getComputedStyle(document.body).getPropertyValue('--dsw-alias-label-primary').trim(),
      })
    })

    // Fit to the pane and forward the new grid to the pty.
    const sendResize = () => {
      if (ws && ws.readyState === WebSocket.OPEN && term.cols > 0) {
        ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }))
      }
    }
    const ro = new ResizeObserver(() => {
      try { fit.fit() } catch { /* hidden pane */ }
      sendResize()
    })
    ro.observe(host)

    const dial = () => {
      if (disposed) return
      setConnState(reconnectAttempt === 0 ? 'connecting' : 'reconnecting')
      ws = new WebSocket(ptyUrl(store.settings.apiEndpoint, store.settings.apiKey, getActiveWorkspace()))
      ws.onopen = () => {
        reconnectAttempt = 0
        setErrMsg(null)
        setConnState('live')
        sendResize()
      }
      ws.onmessage = ev => {
        let f: { type?: string; d?: string; code?: number }
        try { f = JSON.parse(ev.data as string) } catch { return }
        if ((f.type === 'o' || f.type === 'replay') && f.d) {
          term.write(b64ToBytes(f.d))
        } else if (f.type === 'exit') {
          term.write(`\r\n\x1b[2m[shell 进程已退出，code ${f.code ?? 0} — 输入或点击“重启”开始新会话]\x1b[0m\r\n`)
          setConnState('exited')
        }
      }
      ws.onclose = ev => {
        if (disposed) return
        if (ev.code === 1006 || ev.wasClean === false) {
          // Refused dial / dropped socket: ladder up to the last delay.
          if (reconnectAttempt < RECONNECT_DELAYS.length) {
            setConnState('reconnecting')
            setErrMsg(null)
            reconnectTimer = setTimeout(dial, RECONNECT_DELAYS[reconnectAttempt++])
            return
          }
          setConnState('error')
          setErrMsg('多次重连失败 — 请确认服务端以 -pty 启动且 API Key 正确')
          return
        }
        if (ev.code === 1008 || ev.code === 1002) {
          setConnState('error')
          setErrMsg('鉴权失败 — 请在 设置 → API 配置 填写正确的 API Key')
          return
        }
        // Server-initiated close with a reason (unknown workspace, quota).
        setConnState('error')
        setErrMsg(ev.reason || `连接关闭 (${ev.code})`)
      }
      ws.onerror = () => { /* close handler reports */ }
    }
    term.onData(d => {
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'input', text: d }))
      }
    })
    dial()

    return () => {
      disposed = true
      if (reconnectTimer) clearTimeout(reconnectTimer)
      ro.disconnect()
      unsubTheme()
      ws?.close()
      term.dispose()
    }
  }, [epoch])

  // ⌘/Ctrl+Shift+K kills the shell; the restart button re-dials fresh.
  const restart = () => setEpoch(e => e + 1)

  return (
    <div className="dsh-pane dsh-pane-terminal" style={{ display: 'flex', flexDirection: 'column' }}>
      <div className="dsh-pane-files-toolbar" style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'nowrap' }}>
        <span style={{ opacity: 0.85, whiteSpace: 'nowrap' }}>PTY · {wsLabel}</span>
        <span
          style={{
            opacity: connState === 'live' ? 0.9 : 0.55,
            color: connState === 'live' ? 'var(--dsw-alias-success, #98c379)' : connState === 'error' ? '#d4544a' : 'inherit',
            whiteSpace: 'nowrap',
          }}
          title={errMsg ?? undefined}
        >
          {STATUS_LABEL[connState]}
        </span>
        <span style={{ flex: 1 }} />
        {errMsg && <span style={{ color: '#d4544a', opacity: 0.9, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{errMsg}</span>}
        <button
          type="button"
          className="dsh-pane-iconbtn"
          title="重启终端会话（结束当前 shell 进程并新开一个）"
          onClick={restart}
        >
          ↻
        </button>
      </div>
      <div
        ref={hostRef}
        style={{ flex: 1, minHeight: 120, padding: '4px 8px' }}
        aria-label="交互式终端"
      />
    </div>
  )
}
