/**
 * SidebarEnhance —— 侧栏增强容器。
 *
 * 「折叠 / 展开开关 + crossfade/slide 动画」：展开时宽度为 `width`，
 * 折叠时收窄为 `railWidth` 的 rail（可再展开）。宽度变化由
 * `transition: width .25s ease` 承担 slide，内容以 opacity crossfade + 轻微
 * translateX 滑入（`.wideIn` / `.railIn`）。
 *
 * 滚动区域（`.scrollbar-rail`）的原生滚动条仅在 hover 时显示、移开后由
 * `useLingerScroll` 续留 2s 再隐藏（pointer-follow linger）。
 *
 * `header` / `rail` 为纯 ReactNode 槽位，host 可注入 <BrandRow /> 等增强内容。
 */
import { type ReactNode, useState } from 'react'
import s from './sidebar-enhance.module.css'
import { SCROLLBAR_LINGER_MS, useLingerScroll } from './hooks'

export interface SidebarEnhanceProps {
  /** 展开态宽度（px），默认 280 */
  width?: number
  /** 折叠 rail 宽度（px），默认 56 */
  railWidth?: number
  /** 顶部品牌区内容（如 <BrandRow />） */
  header?: ReactNode
  /** 折叠时 rail 内展示的内容；缺省为展开按钮 */
  rail?: ReactNode
  children?: ReactNode
}

export function SidebarEnhance({
  width = 280,
  railWidth = 56,
  header,
  rail,
  children,
}: SidebarEnhanceProps) {
  const [collapsed, setCollapsed] = useState(false)
  const linger = useLingerScroll(SCROLLBAR_LINGER_MS)

  const toggle = () => setCollapsed((v) => !v)

  return (
    <aside
      className={`${s.container}${collapsed ? ` ${s.collapsed}` : ''}`}
      style={{ width: collapsed ? railWidth : width }}
    >
      {collapsed ? (
        <div className={`${s.inner} ${s.railIn}`}>
          <div className={s.rail}>
            {rail ?? (
              <button
                className={s.toggleBtn}
                title="展开侧边栏"
                onClick={toggle}
              >
                »
              </button>
            )}
          </div>
        </div>
      ) : (
        <div className={`${s.inner} ${s.wideIn}`}>
          <div className={s.header}>
            <div className={s.brandArea}>{header}</div>
            <button className={s.toggleBtn} title="折叠侧边栏" onClick={toggle}>
              «
            </button>
          </div>
          <div
            className={`${s['scrollbar-rail']}${linger.visible ? ` ${s.revealed}` : ''}`}
            onMouseEnter={linger.onMouseEnter}
            onMouseLeave={linger.onMouseLeave}
            onScroll={linger.onScroll}
          >
            <div className={s.body}>{children}</div>
          </div>
        </div>
      )}
    </aside>
  )
}