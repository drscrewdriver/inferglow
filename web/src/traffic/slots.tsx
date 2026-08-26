import { registerSlot } from '../framework'
import { SteerQueueDock } from './SteerQueueDock'
import type { SteerQueueDockProps } from './SteerQueueDock'
import { FreezeButton } from './FreezeButton'
import { JobList } from './JobList'

export interface ConversationRightProps {
  disabled?: boolean
}

/** Registered by importing this module (side-effect, mirrors sidebarSlots). */
function registerTrafficSlots(): void {
  // conversation.input.dock — three-level steer queue above the composer.
  registerSlot<SteerQueueDockProps>(
    'conversation.input.dock',
    (props) => <SteerQueueDock onPullBack={props?.onPullBack} />,
    { order: 0 },
  )

  // conversation.input.right — freeze toggle in the composer bar.
  registerSlot<ConversationRightProps>(
    'conversation.input.right',
    (props) => <FreezeButton disabled={props?.disabled} />,
    { order: 0 },
  )

  // details.panel.items — background job list inside the details panel.
  registerSlot('details.panel.items', () => <JobList />, { order: 0 })
}

registerTrafficSlots()