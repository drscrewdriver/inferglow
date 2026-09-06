/**
 * Icon components — extracted from DSH ui-primitives icons.
 * Minimal SVG icons using CSS variables for coloring.
 */

import type { CSSProperties, ReactNode } from 'react'

export interface IconProps {
  size?: number
  className?: string
  style?: CSSProperties
  children?: ReactNode
}

function IconBase({ size = 16, className = '', style, children }: IconProps): ReactNode {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 16 16"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      className={`dsh-icon ${className}`}
      style={{ width: size, height: size, verticalAlign: 'middle', ...style }}
    >
      <title>icon</title>
      {children}
    </svg>
  )
}

/** Sidebar/menu icon */
export function IconMenu({ size = 16, className }: IconProps): ReactNode {
  return (
    <IconBase size={size} className={className}>
      <path d="M3 4h10M3 8h10M3 12h10" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
    </IconBase>
  )
}

/** Chat/message icon */
export function IconChat({ size = 16, className }: IconProps): ReactNode {
  return (
    <IconBase size={size} className={className}>
      <path d="M2 3a2 2 0 012-2h8a2 2 0 012 2v7a2 2 0 01-2 2H5l-3 3V3z" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round" />
    </IconBase>
  )
}

/** Settings/gear icon */
export function IconSettings({ size = 16, className }: IconProps): ReactNode {
  return (
    <IconBase size={size} className={className}>
      <circle cx="8" cy="8" r="2" stroke="currentColor" strokeWidth="1.5" />
      <path d="M8 1v2M8 13v2M1 8h2M13 8h2M3.05 3.05l1.41 1.41M11.54 11.54l1.41 1.41M3.05 12.95l1.41-1.41M11.54 4.46l1.41-1.41" stroke="currentColor" strokeWidth="1.2" strokeLinecap="round" />
    </IconBase>
  )
}

/** New session icon */
export function IconPlus({ size = 16, className }: IconProps): ReactNode {
  return (
    <IconBase size={size} className={className}>
      <path d="M8 3v10M3 8h10" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
    </IconBase>
  )
}

/** Send arrow icon */
export function IconSend({ size = 16, className }: IconProps): ReactNode {
  return (
    <IconBase size={size} className={className}>
      <path d="M2 8l12-5-5 12-2-5-5-2z" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round" />
    </IconBase>
  )
}

/** Moon icon for dark mode toggle */
export function IconMoon({ size = 16, className }: IconProps): ReactNode {
  return (
    <IconBase size={size} className={className}>
      <path d="M13.5 8.5a5.5 5.5 0 11-5-5A4 4 0 0013.5 8.5z" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round" />
    </IconBase>
  )
}

/** Sun icon for light mode toggle */
export function IconSun({ size = 16, className }: IconProps): ReactNode {
  return (
    <IconBase size={size} className={className}>
      <circle cx="8" cy="8" r="3" stroke="currentColor" strokeWidth="1.5" />
      <path d="M8 1v2M8 13v2M1 8h2M13 8h2M3.05 3.05l1.41 1.41M11.54 11.54l1.41 1.41M3.05 12.95l1.41-1.41M11.54 4.46l1.41-1.41" stroke="currentColor" strokeWidth="1.2" strokeLinecap="round" />
    </IconBase>
  )
}

/** Stop/cancel icon */
export function IconStop({ size = 16, className }: IconProps): ReactNode {
  return (
    <IconBase size={size} className={className}>
      <rect x="4" y="4" width="8" height="8" rx="1" stroke="currentColor" strokeWidth="1.5" />
    </IconBase>
  )
}

/** Trash icon */
export function IconTrash({ size = 16, className }: IconProps): ReactNode {
  return (
    <IconBase size={size} className={className}>
      <path d="M3 4h10M6 4V3a1 1 0 011-1h2a1 1 0 011 1v1M5 4v8a1 1 0 001 1h4a1 1 0 001-1V4" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
    </IconBase>
  )
}

/** Search icon */
export function IconSearch({ size = 16, className }: IconProps): ReactNode {
  return (
    <IconBase size={size} className={className}>
      <circle cx="7" cy="7" r="4" stroke="currentColor" strokeWidth="1.5" />
      <path d="M10 10l4 4" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
    </IconBase>
  )
}

/** Panel left icon (sidebar toggle) */
export function IconPanelLeft({ size = 16, className }: IconProps): ReactNode {
  return (
    <IconBase size={size} className={className}>
      <rect x="2" y="3" width="12" height="10" rx="1" stroke="currentColor" strokeWidth="1.5" />
      <path d="M6 3v10" stroke="currentColor" strokeWidth="1.5" />
    </IconBase>
  )
}
