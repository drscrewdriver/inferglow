import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Button } from './Button'

describe('Button', () => {
  it('renders children as label', () => {
    render(<Button>点我</Button>)
    expect(screen.getByRole('button', { name: '点我' })).toBeInTheDocument()
  })

  it('renders an icon alongside the label', () => {
    render(<Button icon={<span aria-hidden>★</span>}>Run</Button>)
    const btn = screen.getByRole('button', { name: 'Run' })
    expect(btn.querySelector('.dsh-btn-icon')).toBeInTheDocument()
  })

  it('applies variant and size classes', () => {
    render(<Button variant="primary" size="sm">OK</Button>)
    expect(screen.getByRole('button', { name: 'OK' })).toHaveClass(
      'dsh-btn-primary',
      'dsh-btn-sm',
    )
  })

  it('adds disabled class and blocks clicks when disabled', async () => {
    const onClick = vi.fn()
    const user = userEvent.setup()
    render(
      <Button disabled onClick={onClick}>禁用</Button>,
    )
    const btn = screen.getByRole('button', { name: '禁用' })
    expect(btn).toBeDisabled()
    expect(btn).toHaveClass('dsh-btn-disabled')
    await user.click(btn)
    expect(onClick).not.toHaveBeenCalled()
  })

  it('triggers onClick when enabled', async () => {
    const onClick = vi.fn()
    const user = userEvent.setup()
    render(<Button onClick={onClick}>提交</Button>)
    await user.click(screen.getByRole('button', { name: '提交' }))
    expect(onClick).toHaveBeenCalledTimes(1)
  })
})