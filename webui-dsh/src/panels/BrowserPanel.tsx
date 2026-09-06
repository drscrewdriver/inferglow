/**
 * BrowserPanel — v1 real embedding: an iframe with a locked-down sandbox
 * (no allow-same-origin). Honest boundaries: many sites send
 * X-Frame-Options/CSP frame-ancestors and will refuse to render inside any
 * embed; there is no server-side proxy and no shared login state. The
 * "新窗口打开" fallback always works.
 */

import { useEffect, useRef, useState } from 'react'

function normalizeUrl(raw: string): string {
  const t = raw.trim()
  if (!t) return ''
  if (/^https?:\/\//i.test(t)) return t
  return `https://${t}`
}

export function BrowserPanel() {
  const [input, setInput] = useState('')
  const [url, setUrl] = useState('')
  const [loaded, setLoaded] = useState(false)
  const frameRef = useRef<HTMLIFrameElement>(null)

  useEffect(() => {
    setLoaded(false)
  }, [url])

  function go() {
    const u = normalizeUrl(input)
    if (u) setUrl(u)
  }

  return (
    <div className="dsh-pane dsh-pane-browser">
      <div className="dsh-pane-browser-bar">
        <button type="button" className="dsh-pane-iconbtn" title="后退（浏览器内）" onClick={() => frameRef.current?.contentWindow?.history.back()}>◀</button>
        <button type="button" className="dsh-pane-iconbtn" title="前进（浏览器内）" onClick={() => frameRef.current?.contentWindow?.history.forward()}>▶</button>
        <button type="button" className="dsh-pane-iconbtn" title="重新加载" onClick={() => setUrl(u => u)}>⟳</button>
        <input
          className="dsh-pane-browser-url"
          placeholder="输入网址，例如 example.com"
          value={input}
          onChange={e => setInput(e.target.value)}
          onKeyDown={e => { if (e.key === 'Enter') go() }}
        />
        <button type="button" className="dsh-pane-linkbtn" onClick={go}>前往</button>
        {url && (
          <a className="dsh-pane-linkbtn" href={url} target="_blank" rel="noreferrer noopener">新窗口打开</a>
        )}
      </div>
      <div className="dsh-pane-browser-sandbox">
        边界说明：多数站点通过 X-Frame-Options/CSP 禁止内嵌（白屏即被拒，请用"新窗口打开"）；
        无服务端代理、无登录态/Cookie 共享；iframe 运行在无同源权限的沙箱中
      </div>
      {url ? (
        <iframe
          ref={frameRef}
          src={url}
          title="browser-panel"
          className="dsh-pane-browser-start"
          style={{ padding: 0, border: 'none', width: '100%', height: '100%', background: '#fff' }}
          sandbox="allow-scripts allow-forms allow-popups"
          onLoad={() => setLoaded(true)}
        />
      ) : (
        <div className="dsh-pane-browser-start">输入网址开始浏览（沙箱模式）</div>
      )}
      {url && !loaded && <div className="dsh-pane-browser-start">加载中…（若长时间白屏，该站点禁止内嵌）</div>}
    </div>
  )
}
