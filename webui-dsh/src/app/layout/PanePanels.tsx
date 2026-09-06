/**
 * PanePanels — the three pane content components shown under the bottom tab
 * bar, mirroring the reference layouts for 文件 / 源代码管理 / 任务管理.
 * The files pane is wired to the real backend (/v1/fs/*); the rest are the
 * upstream static demos.
 */

import type { TabKind } from './PaneEmptyCards.tsx'
import { FilesPanel } from '../../panels/FilesPanel.tsx'
import { BrowserPanel } from '../../panels/BrowserPanel.tsx'
import { TerminalPanel } from '../../panels/TerminalPanel.tsx'
import { TodoPanel } from '../../panels/TodoPanel.tsx'
import { SubagentPanel } from '../../panels/SubagentPanel.tsx'

/* ── 1) 文件树 (Files) — real workspace tree (panels/FilesPanel) ── */
function FilesTree() {
  return <FilesPanel />
}

/* ── 2) 源代码管理 (SCM) ── */
const GIT_ROWS = [
  { status: 'D', path: 'src/old.ts' },
  { status: '?', path: 'src/new.ts' },
  { status: 'M', path: 'src/app/App.tsx' },
  { status: 'M', path: 'package.json' },
]

function ScmPanel() {
  return (
    <div className="dsh-pane dsh-pane-git">
      <div className="dsh-pane-git-header">
        <select className="dsh-pane-git-select" defaultValue="0">
          <option value="0">awesome-dsh-plugin/src</option>
        </select>
        <select className="dsh-pane-git-select" defaultValue="0">
          <option value="0">add/dsh-thinking-levels</option>
        </select>
      </div>

      <div className="dsh-pane-git-section">
        <div className="dsh-pane-git-section-header">
          <span>未暂存 (4)</span>
          <button type="button" className="dsh-pane-linkbtn">全部暂存</button>
        </div>
        {GIT_ROWS.map(r => (
          <div key={r.path} className="dsh-pane-git-row">
            <button type="button" className="dsh-pane-git-row-main">
              <span className="dsh-pane-git-status">{r.status}</span>
              <span className="dsh-pane-git-path">{r.path}</span>
            </button>
            <button type="button" className="dsh-pane-linkbtn">暂存</button>
          </div>
        ))}
      </div>

      <div className="dsh-pane-git-section">
        <div className="dsh-pane-git-section-header"><span>已暂存 (0)</span></div>
        <div className="dsh-pane-git-empty">没有变更</div>
      </div>

      <div className="dsh-pane-git-commit">
        <input className="dsh-pane-git-commit-input" placeholder="提交信息 (Ctrl+Enter)" />
        <button type="button" className="dsh-pane-git-commit-btn" disabled>提交</button>
      </div>
    </div>
  )
}

/* ── 3) 子代理树 (任务管理 — R9: real spawn registry + run summaries) ──
 * The static demo tree moved to panels/SubagentPanel.tsx. */

/* ── 4) 终端 — gated real exec (panels/TerminalPanel) ── */

/* ── 5) 侧边对话 (新对话态) ── */
function SidechatPanel() {
  return (
    <div className="dsh-pane dsh-pane-sidechat">
      <div className="dsh-pane-sidechat-header">
        <span className="dsh-pane-sidechat-context">standard-win · Qwen3.6-35B-A3B</span>
        <div className="dsh-pane-sidechat-actions">
          <button type="button" className="dsh-pane-linkbtn">切换线程</button>
          <button type="button" className="dsh-pane-linkbtn">新建</button>
          <button type="button" className="dsh-pane-linkbtn" disabled>保存为新会话</button>
        </div>
      </div>
      <div className="dsh-pane-sidechat-scroll" />
      <div className="dsh-pane-sidechat-composer">
        <input className="dsh-pane-sidechat-input" placeholder="输入第一个问题，已继承当前会话上下文…" />
        <div className="dsh-pane-sidechat-bar">
          <button type="button" className="dsh-pane-primary-btn" disabled>发送</button>
        </div>
      </div>
    </div>
  )
}

/* ── 6) 浏览器 — real sandboxed iframe (panels/BrowserPanel) ── */

/* ── Dispatcher ── */
export function PaneContent({ kind }: { kind: TabKind }) {
  switch (kind) {
    case 'files':
      return <FilesTree />
    case 'scm':
      return <ScmPanel />
    case 'tasks':
      return <TodoPanel />
    case 'subagent':
      return <SubagentPanel />
    case 'terminal':
      return <TerminalPanel />
    case 'sidechat':
      return <SidechatPanel />
    case 'browser':
      return <BrowserPanel />
    default:
      return <SubagentPanel />
  }
}