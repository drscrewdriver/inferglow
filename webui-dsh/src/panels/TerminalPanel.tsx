/**
 * TerminalPanel — v1 gated command execution against POST /v1/exec.
 * One command per request (no PTY/interactive shell). Errors map to their
 * gates: 404 = route not enabled on the server, 401 = missing API key,
 * 403 = command not allowlisted / workdir rejected.
 */

import { useState } from 'react'
import { api, getActiveWorkspace } from '../bridge/inferglow.ts'

interface HistEntry {
  cmd: string
  exit: number
  output: string
  durationMs: number
  truncated: boolean
}

/** v1 splits on whitespace; quoted args are not supported yet (documented). */
function splitCommand(line: string): string[] {
  return line.trim().split(/\s+/).filter(Boolean)
}

function friendlyError(e: unknown): string {
  const msg = String((e as Error)?.message ?? e)
  if (msg.startsWith('404')) return '404 — server 未启用执行通道（需 -api-key 且 -exec 启动）'
  if (msg.startsWith('401')) return '401 — 缺少 API Key，请在设置 → API 配置中填写'
  if (msg.startsWith('403')) return `403 — ${msg}`
  return msg
}

export function TerminalPanel() {
  const [line, setLine] = useState('')
  const [running, setRunning] = useState(false)
  const [err, setErr] = useState<string | null>(null)
  const [hist, setHist] = useState<HistEntry[]>([])

  async function run() {
    const argv = splitCommand(line)
    if (argv.length === 0 || running) return
    setRunning(true)
    setErr(null)
    try {
      const r = await api.execRun({ argv, workspace: getActiveWorkspace() })
      setHist(h => [{
        cmd: line.trim(),
        exit: r.exit_code,
        output: (r.stdout + (r.stderr ? `\n[stderr]\n${r.stderr}` : '')).trim(),
        durationMs: r.duration_ms,
        truncated: r.truncated,
      }, ...h].slice(0, 20))
      setLine('')
    } catch (e) {
      setErr(friendlyError(e))
    } finally {
      setRunning(false)
    }
  }

  return (
    <div className="dsh-pane dsh-pane-terminal">
      <div className="dsh-pane-terminal-output">
        <div className="dsh-pane-terminal-line" style={{ opacity: 0.65 }}>
          非交互执行模式 · 白名单命令(git/ls/dir/pwd/go)· 每次 {`<`}60s · 全程审计
        </div>
        {hist.map((h, i) => (
          <div key={i} style={{ marginBottom: 8 }}>
            <div className="dsh-pane-terminal-line">
              $ {h.cmd}
              <span style={{ opacity: 0.6 }}> · exit {h.exit} · {h.durationMs}ms{h.truncated ? ' · 已截断' : ''}</span>
            </div>
            <pre style={{ whiteSpace: 'pre-wrap', margin: '2px 0 6px 12px', maxHeight: 160, overflow: 'auto' }}>
              {h.output || '(无输出)'}
            </pre>
          </div>
        ))}
        {hist.length === 0 && !running && <div className="dsh-pane-terminal-caret">▌</div>}
        {running && <div className="dsh-pane-terminal-line">执行中…</div>}
        {err && <div className="dsh-pane-terminal-line" style={{ color: '#d4544a' }}>⚠ {err}</div>}
      </div>
      <div className="dsh-pane-files-toolbar" style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
        <input
          className="dsh-pane-files-search"
          style={{ flex: 1 }}
          placeholder="输入命令，如 git status（空格分参，不支持引号）"
          value={line}
          onChange={e => setLine(e.target.value)}
          onKeyDown={e => { if (e.key === 'Enter') void run() }}
          disabled={running}
        />
        <button type="button" className="dsh-pane-iconbtn" title="执行" onClick={() => void run()} disabled={running || !line.trim()}>
          ▶
        </button>
      </div>
    </div>
  )
}
