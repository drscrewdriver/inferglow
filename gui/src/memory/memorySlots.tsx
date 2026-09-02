import { registerSlot } from '../framework'
import { MemoryPanel } from './MemoryPanel'

/** Registered by importing this module (side-effect, mirrors traffic/slots). */
function registerMemorySlots(): void {
  // details.panel.items — memory facts card in the right panel strip.
  registerSlot('details.panel.items', () => <MemoryPanel />, { order: 2 })
}

registerMemorySlots()
