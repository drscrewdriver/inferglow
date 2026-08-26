import { registerSlot } from '../framework'
import { ThinkingSettingsCard } from './ThinkingSettings'

/** thinking slot registrations (side-effect module, mirrors tidychat/slots). */
function registerThinkingSlots(): void {
  registerSlot('settings.plugin.item', () => <ThinkingSettingsCard />, { order: 1 })
}

registerThinkingSlots()