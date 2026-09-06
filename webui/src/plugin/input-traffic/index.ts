/* input-traffic 插件入口 — 注册槽位并导出组件

 * 槽位约定（Task7 据此集成）：
 *   - conversation.input.dock  → SteerQueueDock（渲染于 Composer 上方）
 *   - conversation.input.right → FreezeButton（渲染于 Composer 右侧）
 */

import { createElement } from 'react'
import { registerSlot, setCardinality } from '../registry'
import { SteerQueueDock } from './SteerQueueDock'
import { FreezeButton } from './FreezeButton'

export { SteerQueueDock } from './SteerQueueDock'
export { FreezeButton } from './FreezeButton'
export * from './store'
export type { SteerQueueDockProps } from './SteerQueueDock'
export type { FreezeButtonProps } from './FreezeButton'
export type { TrafficQueueItem, FreezeRecord, FreezeItem } from './store'

export function registerInputTrafficPlugin(): () => void {
  setCardinality('conversation.input.dock', 'list')
  setCardinality('conversation.input.right', 'list')

  const unDock = registerSlot<{ onPullBack?: (text: string) => void }>(
    'conversation.input.dock',
    (props) => createElement(SteerQueueDock, props),
    { id: 'input-traffic:dock', order: 0 },
  )
  const unFreeze = registerSlot<{ disabled?: boolean }>(
    'conversation.input.right',
    (props) => createElement(FreezeButton, props),
    { id: 'input-traffic:freeze', order: 0 },
  )

  return () => {
    unDock()
    unFreeze()
  }
}