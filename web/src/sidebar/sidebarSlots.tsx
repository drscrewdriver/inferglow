import { registerSlot } from '../framework'
import { WorkspaceList } from './WorkspaceList'
import { SessionTree } from './SessionTree'
import styles from './sidebar.module.css'

/** Context props forwarded to sidebar plugin slots at render time. */
export interface SidebarSlotProps {
  onOpenSettings?: () => void
  onNewSession?: () => void
  onToggle?: () => void
  collapsed?: boolean
  sessionCount?: number
}

/** Registered by importing SidebarRoot; side-effectful slot wiring. */
function registerSidebarSlots(): void {
  // sidebar.workspaces — workspace management pane (hidden in flat view).
  registerSlot<{ groupMode: 'group' | 'flat' }>(
    'sidebar.workspaces',
    (props) => <WorkspaceList groupMode={props?.groupMode} />,
    { order: 0 },
  )

  // sidebar.settings — entry point into the settings panel.
  registerSlot<SidebarSlotProps>(
    'sidebar.settings',
    (props) => (
      <button className={styles.settingsEntry} onClick={props?.onOpenSettings} title="打开设置">
        <span>⚙</span>
        <span>设置</span>
      </button>
    ),
    { order: 0 },
  )

  // sidebar.footer.action — auxiliary footer actions.
  registerSlot<SidebarSlotProps>(
    'sidebar.footer.action',
    (props) => (
      <div className={styles.footerActions}>
        <button className={styles.footerAction} onClick={props?.onNewSession} title="新建会话">
          新建
        </button>
        <button
          className={styles.footerAction}
          onClick={props?.onToggle}
          title={props?.collapsed ? '展开侧边栏' : '折叠侧边栏'}
        >
          {props?.collapsed ? '»' : '«'}
        </button>
        <span className={styles.footerCount}>{props?.sessionCount ?? 0}</span>
      </div>
    ),
    { order: 1 },
  )
}

registerSidebarSlots()

// Re-exported so the whole slot section stays together; SessionTree is used
// directly by SidebarRoot but exported here for plugin authors who want to
// re-render the tree inside their own slot.
export { SessionTree }