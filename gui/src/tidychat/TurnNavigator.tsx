import { useEffect, useRef, useState } from 'react'
import styles from './tidychat.module.css'
import { useTidychatStore } from './tidychatStore'
import {
  HEADER_OFFSET,
  NAV_RAIL_BAR_H,
  NAV_RAIL_BAR_LEN,
  NAV_RAIL_BAR_LEN_CURRENT,
  NAV_RAIL_BAR_LEN_NEAR,
  NAV_RAIL_WIDTH,
  hueColor,
} from './config'
import { indexFromY, layoutPositions, railHeight } from './minimap'

const scrollSel = '[data-conversation-scroll]'
const userSel = '[data-chat-anchor-key][data-chat-flow-kind="user"]'

interface NavPos {
  left: number
  top: number
  gutter: number
}

/** Parse an rgb(a)/hex string into [r,g,b], or null when transparent/unknown. */
function parseRgb(s: string): [number, number, number] | null {
  const m = /rgba?\((\d+)[,\s]+(\d+)[,\s]+(\d+)(?:[,\s]+([\d.]+))?\)/.exec(s)
  if (m !== null) {
    const alpha = m[4] !== undefined ? Number(m[4]) : 1
    if (alpha === 0) return null
    return [Number(m[1]), Number(m[2]), Number(m[3])]
  }
  const h = /^#([0-9a-f]{6})$/i.exec(s.trim())
  if (h !== null) {
    const n = parseInt(h[1], 16)
    return [(n >> 16) & 255, (n >> 8) & 255, n & 255]
  }
  return null
}

/** Return a fixed-rail position hugging the conversation pane left edge. */
function measurePos(): NavPos | null {
  const host = document.querySelector<HTMLElement>(scrollSel)
  if (!host) return null
  const r = host.getBoundingClientRect()
  if (r.width < 10 || r.height < 10) return null
  const content = document.querySelector<HTMLElement>(userSel)
  const gutter = content ? Math.max(0, content.getBoundingClientRect().left - r.left) : r.width
  return { left: r.left, top: r.top + r.height * 0.5, gutter }
}

/** Ambient background luminance → whether to pick contrast light-grey bars. */
function isDarkBackground(): boolean {
  const candidates: HTMLElement[] = []
  const c = document.querySelector<HTMLElement>(scrollSel)
  if (c) candidates.push(c)
  candidates.push(document.body, document.documentElement)
  for (const el of candidates) {
    const rgb = parseRgb(getComputedStyle(el).backgroundColor)
    if (rgb !== null) return 0.2126 * rgb[0] + 0.7152 * rgb[1] + 0.0722 * rgb[2] < 128
  }
  return false
}

function hhmm(ms: number): string {
  const d = new Date(ms)
  const pad = (n: number) => (n < 10 ? '0' + n : String(n))
  return d.getMonth() + 1 + '月' + d.getDate() + '日 ' + pad(d.getHours()) + ':' + pad(d.getMinutes())
}

interface UserRow {
  anchor: string
  time: number
  summary: string
}

/** Left-edge Canvas minimap over the conversation's user turns. */
export function TurnNavigator({ sessionId }: { sessionId: string }) {
  const enabled = useTidychatStore((s) => s.config.navigator)
  const config = useTidychatStore((s) => s.config)
  const canvasRef = useRef<HTMLCanvasElement | null>(null)
  const [pos, setPos] = useState<NavPos | null>(null)
  const [hover, setHover] = useState<number | null>(null)
  const [current, setCurrent] = useState<number | null>(null)
  const [tip, setTip] = useState<{ x: number; y: number; num: number; time: string; text: string } | null>(null)

  // Content-coordinate cache of user rows for binary current-detection + jump.
  const rowCache = useRef<HTMLElement[]>([])
  const topsRef = useRef<number[]>([])
  const scrollInfo = useRef<{ count: number; scrollH: number }>({ count: -1, scrollH: -1 })

  const users: UserRow[] = Array.from(document.querySelectorAll<HTMLElement>(userSel)).map((r) => {
    const anchor = r.getAttribute('data-chat-anchor-key') ?? ''
    const raw = (r.textContent ?? '').replace(/\s+/g, ' ').trim().slice(0, 120)
    return { anchor, time: 0, summary: raw }
  })

  useEffect(() => {
    if (!sessionId) return
    const container = document.querySelector<HTMLElement>(scrollSel)
    if (!container) return
    let raf = 0
    const detect = () => {
      const tops = topsRef.current
      const target = container.scrollTop + HEADER_OFFSET
      let lo = 0
      let hi = tops.length - 1
      let ans = -1
      while (lo <= hi) {
        const mid = (lo + hi) >> 1
        if (tops[mid] <= target) {
          ans = mid
          lo = mid + 1
        } else hi = mid - 1
      }
      const cur = ans === -1 ? 0 : ans
      setCurrent((p) => (p === cur ? p : cur))
    }
    const onScroll = () => {
      if (raf !== 0) return
      raf = requestAnimationFrame(() => {
        raf = 0
        detect()
      })
    }
    container.addEventListener('scroll', onScroll, { passive: true })
    return () => {
      container.removeEventListener('scroll', onScroll)
      if (raf !== 0) cancelAnimationFrame(raf)
    }
  }, [sessionId])

  function rebuildCache(): void {
    const container = document.querySelector<HTMLElement>(scrollSel)
    const rows = Array.from(document.querySelectorAll<HTMLElement>(userSel))
    rowCache.current = rows
    if (!container) return
    const cRect = container.getBoundingClientRect()
    topsRef.current = rows.map((r) => r.getBoundingClientRect().top - cRect.top + container.scrollTop)
  }

  // Refresh layout / cache / canvas after every render where content changed.
  useEffect(() => {
    const container = document.querySelector<HTMLElement>(scrollSel)
    const scrollH = container ? container.scrollHeight : 0
    if (scrollInfo.current.count !== users.length || scrollInfo.current.scrollH !== scrollH) {
      scrollInfo.current = { count: users.length, scrollH }
      rebuildCache()
    }
    setPos(measurePos())
    // Recompute current turn from the cached tops.
    if (container && topsRef.current.length > 0) {
      const target = container.scrollTop + HEADER_OFFSET
      let lo = 0
      let hi = topsRef.current.length - 1
      let ans = -1
      while (lo <= hi) {
        const mid = (lo + hi) >> 1
        if (topsRef.current[mid] <= target) {
          ans = mid
          lo = mid + 1
        } else hi = mid - 1
      }
      setCurrent((p) => (p === (ans === -1 ? 0 : ans) ? p : ans === -1 ? 0 : ans))
    }
    redraw()
  })

  function redraw(): void {
    const canvas = canvasRef.current
    if (!canvas) return
    const n = users.length
    if (n === 0) return
    const H = railHeight(n, window.innerHeight)
    const W = NAV_RAIL_WIDTH - 8
    const dpr = window.devicePixelRatio || 1
    if (canvas.width !== Math.round(W * dpr) || canvas.height !== Math.round(H * dpr)) {
      canvas.width = Math.round(W * dpr)
      canvas.height = Math.round(H * dpr)
      canvas.style.width = W + 'px'
      canvas.style.height = H + 'px'
    }
    const ctx = canvas.getContext('2d')
    if (!ctx) return
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
    ctx.clearRect(0, 0, W, H)

    let bar: string
    if ((config.navColor ?? 'auto') === 'auto') {
      bar = isDarkBackground() ? 'rgba(226,226,226,0.85)' : 'rgba(80,80,80,0.78)'
    } else {
      bar = hueColor(config.navColor, config.navColorLight, 'rgba(128,128,128,0.8)')
    }
    const hot = hueColor(config.navAccent, config.navAccentLight, 'var(--igw-accent, #3b82f6)')
    const positions = layoutPositions(n, hover, H)
    const nearest = (i: number) => hover !== null && Math.abs(i - hover) <= 2
    for (let i = 0; i < n; i++) {
      const y = positions[i]
      const isCurrent = current === i
      const isHover = hover === i
      const len = isHover ? NAV_RAIL_BAR_LEN_NEAR : isCurrent ? NAV_RAIL_BAR_LEN_CURRENT : nearest(i) ? NAV_RAIL_BAR_LEN + 4 : NAV_RAIL_BAR_LEN
      ctx.fillStyle = isCurrent || isHover ? hot : bar
      ctx.fillRect(0, y - NAV_RAIL_BAR_H / 2, len, NAV_RAIL_BAR_H)
      if (isCurrent) {
        ctx.fillStyle = hot
        ctx.beginPath()
        ctx.moveTo(len + 2, y)
        ctx.lineTo(len + 6, y - 3)
        ctx.lineTo(len + 6, y + 3)
        ctx.closePath()
        ctx.fill()
      }
    }
  }

  function jumpTo(index: number): void {
    const target = rowCache.current[index]
    const container = document.querySelector<HTMLElement>(scrollSel)
    if (!target || !container) return
    const cRect = container.getBoundingClientRect()
    const tRect = target.getBoundingClientRect()
    container.scrollTo({ top: tRect.top - cRect.top + container.scrollTop - HEADER_OFFSET, behavior: 'smooth' })
  }

  const positions = layoutPositions(users.length, hover, railHeight(users.length, window.innerHeight))

  const handleMove = (ev: React.PointerEvent<HTMLCanvasElement>) => {
    const canvas = canvasRef.current
    if (!canvas) return
    const rect = canvas.getBoundingClientRect()
    const idx = indexFromY(ev.clientY - rect.top, positions)
    if (idx !== hover) setHover(idx)
    const u = users[idx]
    if (u) setTip({ x: ev.clientX + 18, y: ev.clientY - 8, num: idx + 1, time: u.time ? hhmm(u.time) : '', text: u.summary })
  }

  if (!enabled) return null
  if (pos && pos.gutter < NAV_RAIL_WIDTH) return null
  if (users.length === 0) return null

  const style = pos ? { left: pos.left, top: pos.top, transform: 'translateY(-50%)' } : { left: 280, top: '50vh' }
  return (
    <>
      <div className={styles.rail} style={style} aria-label="用户消息定位">
        <canvas
          ref={canvasRef}
          className={styles.canvas}
          onPointerMove={handleMove}
          onPointerLeave={() => {
            setHover(null)
            setTip(null)
          }}
          onPointerUp={(ev) => {
            const rect = canvasRef.current?.getBoundingClientRect()
            if (rect) jumpTo(indexFromY(ev.clientY - rect.top, positions))
            setHover(null)
            setTip(null)
          }}
        />
      </div>
      {tip && (
        <div className={styles.tip} style={{ left: tip.x, top: tip.y }}>
          <div className={styles.tipHead}>#{tip.num}{tip.time !== '' ? ' · ' + tip.time : ''}</div>
          <div>{tip.text || `用户 ${tip.num}`}</div>
        </div>
      )}
    </>
  )
}

export type TurnNavigatorProps = { sessionId: string }