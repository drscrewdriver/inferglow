import { registerSlot } from '../framework'
import { SubAgentPanel } from './SubAgentPanel'

/** Registered by importing this module (side-effect, mirrors traffic/slots). */
function registerSubAgentSlots(): void {
  // details.panel.items — sub-agent profile cards in the right panel strip.
  registerSlot('details.panel.items', () => <SubAgentPanel />, { order: 3 })
}

registerSubAgentSlots()
