import { registerSlot } from '../framework'
import { TodoPanel } from './TodoPanel'

/** Registered by importing this module (side-effect, mirrors traffic/slots). */
function registerTodoSlots(): void {
  // details.panel.items — todo list card in the right panel strip.
  registerSlot('details.panel.items', () => <TodoPanel />, { order: 4 })
}

registerTodoSlots()
