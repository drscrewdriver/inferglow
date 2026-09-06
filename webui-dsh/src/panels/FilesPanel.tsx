/**
 * FilesPanel — real workspace file tree backed by /v1/fs/*.
 *
 * Read-only v1 (fs write/delete/upload have no server-side audit surface).
 * Lazy per-directory loading (the API is single-level by design), 200-row
 * paging for huge directories, filename search, and a guarded preview
 * (extension blacklist + size truncation + NUL-byte binary detection).
 */

import { useEffect, useRef, useState } from 'react'
import { api, getActiveWorkspace } from '../bridge/inferglow.ts'
import { fileIcon } from './fileIcons.tsx'
import type { FsEntry } from '../api/client.ts'

const PAGE = 200
const PREVIEW_MAX = 256 * 1024

const BINARY_EXT = new Set([
  'png', 'jpg', 'jpeg', 'gif', 'webp', 'ico', 'bmp', 'pdf', 'zip', 'gz', 'tgz',
  'tar', 'rar', '7z', 'exe', 'dll', 'so', 'dylib', 'woff', 'woff2', 'ttf',
  'eot', 'mp3', 'mp4', 'mov', 'avi', 'mkv', 'wav', 'flac', 'sqlite', 'db',
  'bin', 'class', 'jar', 'pyc', 'wasm', 'woff',
])

function extOf(name: string): string {
  const i = name.lastIndexOf('.')
  return i >= 0 ? name.slice(i + 1).toLowerCase() : ''
}

interface Preview {
  path: string
  content: string
  bytes: number
  truncated: boolean
}

function sortEntries(entries: FsEntry[]): FsEntry[] {
  return [...entries].sort((a, b) => {
    if (a.is_dir !== b.is_dir) return a.is_dir ? -1 : 1
    return a.name.localeCompare(b.name)
  })
}

export function FilesPanel() {
  const [root, setRoot] = useState('')
  const [children, setChildren] = useState<Map<string, FsEntry[]>>(new Map())
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const [limits, setLimits] = useState<Map<string, number>>(new Map())
  const [pending, setPending] = useState<Set<string>>(new Set())
  const [loading, setLoading] = useState(true)
  const [err, setErr] = useState<string | null>(null)
  const [preview, setPreview] = useState<Preview | null>(null)
  const [previewLoading, setPreviewLoading] = useState(false)
  const [searchQ, setSearchQ] = useState('')
  const [searchHits, setSearchHits] = useState<{ matches: string[]; truncated: boolean } | null>(null)
  const [recent, setRecent] = useState<{ path: string; bytes: number }[] | null>(null)
  const mounted = useRef(true)

  const wsRef = useRef('')
  useEffect(() => {
    mounted.current = true
    wsRef.current = getActiveWorkspace()
    void reload()
    void api.producedFiles(8, wsRef.current).then(r => {
      if (mounted.current) setRecent(r.files)
    }).catch(() => { /* produced list is best-effort */ })
    return () => { mounted.current = false }
  }, [])

  async function reload() {
    setLoading(true)
    setErr(null)
    setPreview(null)
    setSearchHits(null)
    try {
      const t = await api.fsTree('', wsRef.current)
      if (!mounted.current) return
      setRoot(t.root)
      setChildren(new Map([['', sortEntries(t.entries)]]))
      setExpanded(new Set())
    } catch (e) {
      if (mounted.current) setErr(String((e as Error)?.message ?? e))
    } finally {
      if (mounted.current) setLoading(false)
    }
  }

  async function toggleDir(path: string) {
    if (expanded.has(path)) {
      const next = new Set(expanded)
      next.delete(path)
      setExpanded(next)
      return
    }
    const next = new Set(expanded)
    next.add(path)
    setExpanded(next)
    if (!children.has(path)) {
      setPending(prev => new Set(prev).add(path))
      try {
        const t = await api.fsTree(path, wsRef.current)
        if (!mounted.current) return
        setChildren(prev => new Map(prev).set(path, sortEntries(t.entries)))
      } catch (e) {
        if (mounted.current) setErr(String((e as Error)?.message ?? e))
      } finally {
        if (mounted.current) setPending(prev => {
          const n = new Set(prev)
          n.delete(path)
          return n
        })
      }
    }
  }

  async function openPreview(path: string) {
    if (BINARY_EXT.has(extOf(path))) {
      setPreview({ path, content: `⚠ ${extOf(path)} 为二进制类型，只读预览不支持。`, bytes: 0, truncated: false })
      return
    }
    setPreviewLoading(true)
    setPreview(null)
    try {
      const r = await api.fsRead(path, wsRef.current)
      if (!mounted.current) return
      if (r.content.includes('\u0000')) {
        setPreview({ path, content: '⚠ 检测到 NUL 字节（二进制内容），只读预览不支持。', bytes: r.bytes, truncated: false })
        return
      }
      const truncated = r.content.length > PREVIEW_MAX
      setPreview({
        path,
        content: truncated ? r.content.slice(0, PREVIEW_MAX) : r.content,
        bytes: r.bytes,
        truncated,
      })
    } catch (e) {
      if (mounted.current) setPreview({ path, content: `⚠ 读取失败：${String((e as Error)?.message ?? e)}`, bytes: 0, truncated: false })
    } finally {
      if (mounted.current) setPreviewLoading(false)
    }
  }

  async function runSearch() {
    const q = searchQ.trim()
    if (!q) { setSearchHits(null); return }
    setLoading(true)
    try {
      const r = await api.fsSearch(q, 200, wsRef.current)
      if (!mounted.current) return
      setSearchHits({ matches: r.matches, truncated: r.truncated })
    } catch (e) {
      if (mounted.current) setErr(String((e as Error)?.message ?? e))
    } finally {
      if (mounted.current) setLoading(false)
    }
  }

  function renderEntries(entries: FsEntry[], depth: number, pathKey: string) {
    const limit = limits.get(pathKey) ?? PAGE
    const visible = entries.slice(0, limit)
    const rows: ReturnType<typeof renderRow>[] = []
    for (const e of visible) {
      if (e.hidden) continue
      rows.push(renderRow(e, depth))
      if (e.is_dir && expanded.has(e.path)) {
        const kids = children.get(e.path)
        if (kids) rows.push(...renderEntries(kids, depth + 1, e.path))
        else if (pending.has(e.path)) {
          rows.push(
            <div key={`${e.path}::loading`} className="dsh-pane-tree-row" style={{ paddingLeft: 8 + (depth + 1) * 16 }}>
              <span className="dsh-pane-tree-caret" />
              <span className="dsh-pane-tree-name"><span className="dsh-pane-tree-label">加载中…</span></span>
            </div>,
          )
        }
      }
    }
    if (entries.length > limit) {
      rows.push(
        <button
          key={`${pathKey}::more`}
          type="button"
          className="dsh-pane-linkbtn"
          style={{ margin: '2px 0 2px 8px', display: 'block' }}
          onClick={() => setLimits(prev => new Map(prev).set(pathKey, limit + PAGE))}
        >
          加载更多（剩余 {entries.length - limit}）
        </button>,
      )
    }
    return rows
  }

  function renderRow(e: FsEntry, depth: number) {
    const isOpen = e.is_dir && expanded.has(e.path)
    return (
      <div
        key={e.path}
        className={`dsh-pane-tree-row${e.is_dir ? ' is-folder' : ''}`}
        style={{ paddingLeft: 8 + depth * 16 }}
        onClick={() => { if (e.is_dir) void toggleDir(e.path) }}
        role={e.is_dir ? 'treeitem' : undefined}
        aria-expanded={e.is_dir ? isOpen : undefined}
      >
        <span className="dsh-pane-tree-caret">{e.is_dir ? (isOpen ? '▾' : '▸') : ''}</span>
        <span className="dsh-pane-tree-name">
          {fileIcon(e.name, e.is_dir, isOpen)}
          <span className="dsh-pane-tree-label" title={e.path}>{e.name}</span>
        </span>
        {!e.is_dir && (
          <button
            type="button"
            className="dsh-pane-tree-act"
            aria-label={`预览 ${e.name}`}
            onClick={ev => { ev.stopPropagation(); void openPreview(e.path) }}
          >…</button>
        )}
      </div>
    )
  }

  return (
    <div className="dsh-pane dsh-pane-files">
      <div className="dsh-pane-files-toolbar">
        <input
          className="dsh-pane-files-search"
          placeholder="按文件名搜索…"
          value={searchQ}
          onChange={e => setSearchQ(e.target.value)}
          onKeyDown={e => { if (e.key === 'Enter') void runSearch() }}
        />
        <button type="button" className="dsh-pane-iconbtn" title="刷新" onClick={() => void reload()}>↻</button>
        <button type="button" className="dsh-pane-iconbtn" title="只读模式（写操作二期开放）" disabled>↑</button>
      </div>
      <div className="dsh-pane-files-toolbar" style={{ opacity: 0.75 }}>
        <span className="dsh-pane-tree-label" title={root}>workspace: {root || '…'}</span>
      </div>

      {err && <div className="dsh-pane-git-empty">⚠ {err}</div>}
      {previewLoading && <div className="dsh-pane-git-empty">读取中…</div>}
      {loading && !preview && <div className="dsh-pane-git-empty">加载中…</div>}

      {preview ? (
        <div className="dsh-pane-files-tree">
          <div className="dsh-pane-git-section-header">
            <span className="dsh-pane-tree-label" title={preview.path}>{preview.path}（{preview.bytes} B）</span>
            <button type="button" className="dsh-pane-linkbtn" onClick={() => setPreview(null)}>关闭预览</button>
          </div>
          {preview.truncated && (
            <div className="dsh-pane-git-empty">⚠ 内容超过 256KB，已截断展示</div>
          )}
          <pre className="dsh-pane-terminal-output" style={{ whiteSpace: 'pre-wrap', maxHeight: 320, overflow: 'auto' }}>
            {preview.content}
          </pre>
        </div>
      ) : searchHits ? (
        <div className="dsh-pane-files-tree">
          <div className="dsh-pane-git-section-header">
            <span>搜索 “{searchQ}”：{searchHits.matches.length} 个结果{searchHits.truncated ? '（已截断）' : ''}</span>
            <button type="button" className="dsh-pane-linkbtn" onClick={() => setSearchHits(null)}>返回目录树</button>
          </div>
          {searchHits.matches.map(m => (
            <div key={m} className="dsh-pane-tree-row" onClick={() => void openPreview(m)}>
              <span className="dsh-pane-tree-caret" />
              <span className="dsh-pane-tree-name">{fileIcon(m, false)}<span className="dsh-pane-tree-label" title={m}>{m}</span></span>
              <button type="button" className="dsh-pane-tree-act" aria-label={`预览 ${m}`}
                onClick={ev => { ev.stopPropagation(); void openPreview(m) }}>…</button>
            </div>
          ))}
          {searchHits.matches.length === 0 && <div className="dsh-pane-git-empty">无匹配</div>}
        </div>
      ) : (
        <div className="dsh-pane-files-tree" role="tree" aria-label="workspace 文件树">
          {!loading && renderEntries(children.get('') ?? [], 0, '')}
          {recent && recent.length > 0 && (
            <>
              <div className="dsh-pane-git-section-header" style={{ marginTop: 8 }}>
                <span>最近产出（produced-files）</span>
              </div>
              {recent.map(f => (
                <div key={f.path} className="dsh-pane-tree-row" onClick={() => void openPreview(f.path)}>
                  <span className="dsh-pane-tree-caret" />
                  <span className="dsh-pane-tree-name">{fileIcon(f.path, false)}<span className="dsh-pane-tree-label" title={f.path}>{f.path}</span></span>
                </div>
              ))}
            </>
          )}
        </div>
      )}
    </div>
  )
}
