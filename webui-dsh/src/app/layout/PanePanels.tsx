/**
 * PanePanels — the three pane content components shown under the bottom tab
 * bar, mirroring the reference layouts for 文件 / 源代码管理 / 任务管理.
 */

import type { TabKind } from './PaneEmptyCards.tsx'

/* ── 1) 文件树 (Files) ── */
const FILES_TREE = [
  { name: 'src', folder: true, indent: 0 },
  { name: 'app', folder: true, indent: 1 },
  { name: 'App.tsx', folder: false, indent: 2 },
  { name: 'main.tsx', folder: false, indent: 1 },
  { name: 'styles', folder: true, indent: 0 },
  { name: 'components.css', folder: false, indent: 1 },
  { name: 'vite.config.ts', folder: false, indent: 0 },
]

function FilesTree() {
  return (
    <div className="dsh-pane dsh-pane-files">
      <div className="dsh-pane-files-toolbar">
        <input className="dsh-pane-files-search" placeholder="按文件名搜索…" />
        <button type="button" className="dsh-pane-iconbtn" title="刷新">↻</button>
        <button type="button" className="dsh-pane-iconbtn" title="上传文件">↑</button>
        <button type="button" className="dsh-pane-iconbtn" title="上传文件夹">📁</button>
      </div>
      <div className="dsh-pane-files-tree">
        {FILES_TREE.map(n => (
          <div key={n.name} className={`dsh-pane-tree-row${n.folder ? ' is-folder' : ''}`} style={{ paddingLeft: 8 + n.indent * 16 }}>
            <span className="dsh-pane-tree-caret">{n.folder ? '▸' : ''}</span>
            <span className="dsh-pane-tree-name">{n.folder ? '📁' : '📄'}<span className="dsh-pane-tree-label">{n.name}</span></span>
            <button type="button" className="dsh-pane-tree-act" aria-label="操作">…</button>
          </div>
        ))}
      </div>
    </div>
  )
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

/* ── 3) 子代理树 (任务管理 — 后台运行的子代理) ── */
function SubagentTree() {
  return (
    <div className="dsh-pane dsh-pane-agent">
      <div className="dsh-pane-agent-header">任务管理 · rewrite-agently</div>
      <div className="dsh-pane-agent-body" role="tree" aria-label="任务管理">
        <div className="dsh-pane-agent-row is-active" role="treeitem" aria-selected="true">
          <span className="dsh-pane-agent-dot" data-state="done" aria-hidden="true" />
          <span className="dsh-pane-agent-content">
            <span className="dsh-pane-agent-label">rewrite-agently</span>
            <span className="dsh-pane-agent-secondary">主代理 · 空闲</span>
          </span>
        </div>
        <div className="dsh-pane-agent-empty">
          <div>暂无子代理</div>
          <div className="dsh-pane-agent-empty-hint">当前主代理派生的子代理将显示在这里</div>
        </div>
      </div>
    </div>
  )
}

/* ── 4) 终端 (modal terminal session) ── */
function TerminalPanel() {
  return (
    <div className="dsh-pane dsh-pane-terminal">
      <div className="dsh-pane-terminal-output">
        <div className="dsh-pane-terminal-line">PS C:\workspace&gt; </div>
        <div className="dsh-pane-terminal-caret">▌</div>
      </div>
    </div>
  )
}

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

/* ── 6) 浏览器 (空态) ── */
function BrowserPanel() {
  return (
    <div className="dsh-pane dsh-pane-browser">
      <div className="dsh-pane-browser-bar">
        <button type="button" className="dsh-pane-iconbtn" disabled>◀</button>
        <button type="button" className="dsh-pane-iconbtn" disabled>▶</button>
        <button type="button" className="dsh-pane-iconbtn">⟳</button>
        <input className="dsh-pane-browser-url" placeholder="输入网址，例如 example.com" />
        <button type="button" className="dsh-pane-linkbtn">前往</button>
      </div>
      <div className="dsh-pane-browser-sandbox">
        沙箱模式：已启用 · 页面无法访问界面数据与本地文件，登录态与第三方 Cookie 可能不可用
      </div>
      <div className="dsh-pane-browser-start">输入网址开始浏览（沙箱模式）</div>
    </div>
  )
}

/* ── Dispatcher ── */
export function PaneContent({ kind }: { kind: TabKind }) {
  switch (kind) {
    case 'files':
      return <FilesTree />
    case 'scm':
      return <ScmPanel />
    case 'tasks':
      return <SubagentTree />
    case 'terminal':
      return <TerminalPanel />
    case 'sidechat':
      return <SidechatPanel />
    case 'browser':
      return <BrowserPanel />
    default:
      return <SubagentTree />
  }
}