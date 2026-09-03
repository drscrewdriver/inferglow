/**
 * useLingerScroll —— 滚动条 linger 控制逻辑（默认 2s）。
 *
 * 浏览器自带的原生滚动条仅在指针悬停时显示（:hover），当指针移开后由
 * `visible` 状态续留 linger ms 再隐藏，配合 `.scrollbar-rail.revealed`
 * 类将滚动条 thumb 重新露出。onScroll 在滚动过程中也会续留计时。
 */
import { useCallback, useEffect, useRef, useState } from 'react'

export const SCROLLBAR_LINGER_MS = 2000

export interface LingerScrollHandlers {
  /** 滚动条是否当前可见（供 CSS 类切换） */
  visible: boolean
  onMouseEnter: () => void
  onMouseLeave: () => void
  onScroll: () => void
}

export function useLingerScroll(lingerMs: number = SCROLLBAR_LINGER_MS): LingerScrollHandlers {
  const [visible, setVisible] = useState(false)
  const hovering = useRef(false)
  const timer = useRef<number | undefined>(undefined)

  const clearTimer = useCallback(() => {
    if (timer.current !== undefined) {
      window.clearTimeout(timer.current)
      timer.current = undefined
    }
  }, [])

  const scheduleHide = useCallback(() => {
    clearTimer()
    timer.current = window.setTimeout(() => {
      // 指针仍在区域内时不该隐藏（hover 已保证可见）。
      if (!hovering.current) setVisible(false)
      timer.current = undefined
    }, lingerMs)
  }, [clearTimer, lingerMs])

  const show = useCallback(() => {
    setVisible(true)
    clearTimer()
  }, [clearTimer])

  const onMouseEnter = useCallback(() => {
    hovering.current = true
    show()
  }, [show])

  // 移开 → 开始 2s linger 计时，到期隐藏。
  const onMouseLeave = useCallback(() => {
    hovering.current = false
    scheduleHide()
  }, [scheduleHide])

  // 滚动时立即露出并重置计时，滚动停止 linger ms 后隐藏。
  const onScroll = useCallback(() => {
    show()
    scheduleHide()
  }, [show, scheduleHide])

  // 卸载时清理计时器，避免泄漏。
  useEffect(() => clearTimer, [clearTimer])

  return { visible, onMouseEnter, onMouseLeave, onScroll }
}