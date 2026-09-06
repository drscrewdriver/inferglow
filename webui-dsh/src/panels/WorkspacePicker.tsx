/**
 * WorkspacePicker — hero workspace-selector popover. Lists the server's
 * full workspace registry (GET /v1/workspaces — flags + shared config +
 * runtime registrations) on every open; picking one switches the global
 * activeWorkspace, which the hero label and every fs/git/exec/pty panel
 * already follow.
 */

import { useEffect, useState } from 'react'
import { store } from '../store.ts'
import { refreshWorkspaces } from '../bridge/inferglow.ts'
import type { ConfigPopoverProps } from './ConfigPopover.tsx'
import { ConfigPopover, type PopoverItem } from './ConfigPopover.tsx'

export function WorkspacePicker({ anchor, onClose }: { anchor: HTMLElement | null; onClose: () => void }) {
  const [items, setItems] = useState<PopoverItem[] | null>(null)

  // 每次打开重拉 — 与 server 注册表保持同步(-workspace/共享配置/运行时注册)。
  useEffect(() => {
    let alive = true
    void refreshWorkspaces().then(() => {
      if (alive) {
        setItems(store.workspaces.map(w => ({
          id: w.name,
          label: w.name,
          detail: w.root || '(默认链)',
          selected: w.name === store.activeWorkspace,
        })))
      }
    })
    return () => { alive = false }
  }, [])

  const pick = (id: string) => {
    store.setActiveWorkspace(id)
    onClose()
  }

  const props: ConfigPopoverProps = {
    anchor, items, onPick: pick, onClose,
    footer: 'GET /v1/workspaces · 旗标+共享配置+运行时注册',
  }
  return <ConfigPopover {...props} />
}
