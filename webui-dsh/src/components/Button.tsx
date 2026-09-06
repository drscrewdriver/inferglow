/**
 * Button component — extracted from DSH ui-primitives.
 * Uses CSS custom properties for theming.
 */

import type { ButtonHTMLAttributes, ReactNode } from 'react'

export type ButtonVariant = 'primary' | 'ghost' | 'outline'

export function Button({
  variant = 'ghost',
  size = 'md',
  icon,
  className = '',
  children,
  disabled,
  ...rest
}: ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: ButtonVariant
  size?: 'md' | 'sm'
  icon?: ReactNode
}) {
  return (
    <button
      type="button"
      className={`dsh-btn dsh-btn-${variant} dsh-btn-${size}${disabled ? ' dsh-btn-disabled' : ''}${className ? ' ' + className : ''}`}
      disabled={disabled}
      {...rest}
    >
      {icon != null && <span className="dsh-btn-icon">{icon}</span>}
      {children != null && <span className="dsh-btn-label">{children}</span>}
    </button>
  )
}
