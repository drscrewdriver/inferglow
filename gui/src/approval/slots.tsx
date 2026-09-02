import { registerSlot } from '../framework'
import { ApprovalPanel } from './ApprovalPanel'

/** approval slot registrations (side-effect module). */
function registerApprovalSlots(): void {
  // details.panel.items — approval cards beneath the background job list.
  registerSlot('details.panel.items', () => <ApprovalPanel />, { order: 1 })
}

registerApprovalSlots()