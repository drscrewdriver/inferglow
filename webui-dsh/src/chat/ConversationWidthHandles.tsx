/**
 * ConversationWidthHandles — centered transcript content-width handles,
 * mirroring the dsh v0.1.2-alpha.3 WidthHandle (ui-conversation).
 *
 * Renders the conversation body band (positioning context for the two
 * col-resize strips), publishes the column's live width and the persisted
 * user width as CSS variables on it, and wires a symmetric resize gesture:
 * both sides write the one centered width, so dragging outward widens by 2×
 * the pointer distance. The strips expose the pointer's Y as
 * --dsh-width-handle-pointer-y for a pointer-following glow indicator.
 */

import { useCallback, useRef, useState, type ReactNode } from 'react'
import {
  readWidthPreference,
  resolveContentWidth,
  WIDTH_PREF_KEY,
} from './conversationWidth.ts'

/** Default content width before the first measurement. */
const FALLBACK_WIDTH = 680

/**
 * One drag handle: pointer capture + rAF-throttled symmetric resize.
 * Mirrors ui-layout AppFrame's DragHandle capture model.
 */
function WidthHandle(props: {
  side: 'left' | 'right'
  onStart: () => number
  onDrag: (width: number) => void
  onCommit: (width: number) => void
  onEnd: () => void
}) {
  const [dragging, setDragging] = useState(false)
  const base = useRef(0)
  const origin = useRef(0)
  const latest = useRef(0)
  const frame = useRef<number | null>(null)
  const callbacks = useRef(props)
  callbacks.current = props

  const outwardWidth = () => {
    const dx = latest.current - origin.current
    const outward = callbacks.current.side === 'right' ? dx : -dx
    return base.current + outward * 2
  }
  const cancelFrame = () => {
    if (frame.current !== null) { cancelAnimationFrame(frame.current); frame.current = null }
  }

  const onPointerDown = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    e.preventDefault()
    e.currentTarget.setPointerCapture(e.pointerId)
    origin.current = e.clientX
    latest.current = e.clientX
    base.current = callbacks.current.onStart()
    setDragging(true)
  }, [])

  const onPointerMove = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    const box = e.currentTarget.getBoundingClientRect()
    e.currentTarget.style.setProperty('--dsh-width-handle-pointer-y', `${e.clientY - box.top}px`)
    if (!e.currentTarget.hasPointerCapture(e.pointerId)) return
    latest.current = e.clientX
    frame.current ??= requestAnimationFrame(() => {
      frame.current = null
      callbacks.current.onDrag(outwardWidth())
    })
  }, [])

  const onPointerUp = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    if (!e.currentTarget.hasPointerCapture(e.pointerId)) return
    e.currentTarget.releasePointerCapture(e.pointerId)
    cancelFrame()
    latest.current = e.clientX
    // Only a gesture with actual travel commits: a press-and-release on a
    // window-clamped width must not overwrite the wider stored preference.
    if (latest.current !== origin.current) callbacks.current.onCommit(outwardWidth())
    setDragging(false)
    callbacks.current.onEnd()
  }, [])

  // Releasing outside the window delivers pointercancel (or drops capture)
  // instead of pointerup; abandon the gesture uncommitted and republish.
  const onPointerCancel = useCallback(() => {
    cancelFrame()
    setDragging(false)
    callbacks.current.onEnd()
  }, [])

  return (
    <div
      className="dsh-width-handle"
      data-side={props.side}
      data-width-handle={props.side}
      data-dragging={dragging || undefined}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={onPointerUp}
      onPointerCancel={onPointerCancel}
      onLostPointerCapture={onPointerCancel}
    />
  )
}

/**
 * Body band that owns the content-width axis. Shadow the children (the
 * transcript scroller) with the two draggable strips in the column gutters.
 */
export function ConversationWidthHandles({ children }: { children: ReactNode }) {
  const rootEl = useRef<HTMLDivElement | null>(null)
  const observer = useRef<ResizeObserver | null>(null)

  const publishWidths = useCallback((root: HTMLDivElement) => {
    const column = root.offsetWidth
    root.style.setProperty('--dsh-conversation-column-width', `${column}px`)
    const preference = readWidthPreference()
    if (preference === null) {
      root.style.removeProperty('--dsh-chat-user-width')
    } else {
      root.style.setProperty('--dsh-chat-user-width', `${resolveContentWidth(column, preference)}px`)
    }
  }, [])

  const rootResizeRef = useCallback((root: HTMLDivElement | null) => {
    observer.current?.disconnect()
    observer.current = null
    rootEl.current = root
    if (root === null) return
    observer.current = new ResizeObserver(() => { publishWidths(root) })
    observer.current.observe(root)
    publishWidths(root)
  }, [publishWidths])

  // Drag plumbing: onStart snapshots the resolved width (grabbing a
  // clamped column must not jump back to the raw stored preference), onDrag
  // publishes only the live clamped style, onCommit persists a gesture that
  // actually travelled, and onEnd republishes from storage.
  const onHandleStart = useCallback((): number => {
    const root = rootEl.current
    if (root === null) return FALLBACK_WIDTH
    return resolveContentWidth(root.offsetWidth, readWidthPreference())
  }, [])
  const onHandleDrag = useCallback((width: number): void => {
    const root = rootEl.current
    if (root === null) return
    root.style.setProperty('--dsh-chat-user-width', `${resolveContentWidth(root.offsetWidth, width)}px`)
  }, [])
  const onHandleCommit = useCallback((width: number): void => {
    const root = rootEl.current
    if (root === null) return
    localStorage.setItem(WIDTH_PREF_KEY, `${resolveContentWidth(root.offsetWidth, width)}`)
  }, [])
  const onHandleEnd = useCallback((): void => {
    const root = rootEl.current
    if (root !== null) publishWidths(root)
  }, [publishWidths])

  return (
    <div ref={rootResizeRef} className="dsh-chat-body">
      {children}
      {(['left', 'right'] as const).map(side => (
        <WidthHandle
          key={side}
          side={side}
          onStart={onHandleStart}
          onDrag={onHandleDrag}
          onCommit={onHandleCommit}
          onEnd={onHandleEnd}
        />
      ))}
    </div>
  )
}